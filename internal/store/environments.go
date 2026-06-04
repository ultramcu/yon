package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ultramcu/yon/internal/model"
)

// environmentsSuffix is appended to a collection's base name (in place of the
// `.yon` extension) to form the sibling directory that holds its environment
// files. For ".../api.yon" the directory is ".../api.environments".
const environmentsSuffix = ".environments"

// envFileExt is the file extension for a single environment file.
const envFileExt = ".json"

// dotenvName is the basename of the gitignored secrets file written next to the
// collection (in the collection's own directory, not the environments dir).
const dotenvName = ".env"

// Reserved .env key scheme for a jump host's secret fields. A jump host's
// Passphrase/Password live in the same .env as Secret variables but under keys
// NAMESPACED with the "__jumphost." prefix so they can never collide with a
// user Variable key (a Variable named "password" is stored under "password",
// the jump host's under "__jumphost.<env>.password"). The "<env>" segment is
// the environment's file base (see envFileBase), so distinct environments each
// get their own reserved keys rather than overwriting one shared slot. The
// double-underscore prefix keeps these store-owned keys visually distinct in
// the committed-out .env and lets the load/prune code recognise them.
const jumpHostKeyPrefix = "__jumphost."

// jumpHostSecretKeys returns the reserved .env keys for the named environment's
// jump-host Passphrase and Password. Keying on envFileBase(name) — the same
// collision-resistant base used for the environment file — keeps each
// environment's jump-host secrets in their own slots.
func jumpHostSecretKeys(name string) (passphraseKey, passwordKey string) {
	base := envFileBase(name)
	return jumpHostKeyPrefix + base + ".passphrase",
		jumpHostKeyPrefix + base + ".password"
}

// Reserved .env key scheme for a Secret Variable's value. A secret Variable's
// value lives in the same .env as the jump-host secrets, but under a key
// NAMESPACED with the "__var." prefix and the owning environment's file base so
// two environments that each declare a Secret with the SAME Variable.Key keep
// their values in separate slots rather than sharing one (which would let a
// later save clobber an earlier env's secret, losing data). This mirrors
// jumpHostSecretKeys: the "<env>" segment is envFileBase(envName), giving each
// environment its own reserved keys. The "__var." prefix is store-owned and
// visually distinct from "__jumphost." in the committed-out .env, and the two
// can never be confused (different leading segment) even though both are ours.
const varKeyPrefix = "__var."

// varSecretKey returns the reserved .env key for a Secret variable named varKey
// in the environment named envName. Namespacing on envFileBase keeps each
// environment's secrets in their own slot (like jumpHostSecretKeys), so a Secret
// named "token" in env "A" and one in env "B" never share a single .env slot.
func varSecretKey(envName, varKey string) string {
	return varKeyPrefix + envFileBase(envName) + "." + varKey
}

// gitignoreName is the basename of the .gitignore maintained in the collection
// directory so the .env secrets file is never committed.
const gitignoreName = ".gitignore"

// EnvironmentsDir returns the sibling directory that holds the environment files
// for the collection at collPath, e.g. ".../api.yon" -> ".../api.environments".
// It returns "" when collPath is "" (an unsaved collection has no on-disk home).
func EnvironmentsDir(collPath string) string {
	if collPath == "" {
		return ""
	}
	ext := filepath.Ext(collPath)
	base := strings.TrimSuffix(collPath, ext)
	return base + environmentsSuffix
}

// dotenvPath returns the path to the collection's secrets .env file, which lives
// in the collection's own directory (the parent of EnvironmentsDir).
func dotenvPath(collPath string) string {
	return filepath.Join(filepath.Dir(collPath), dotenvName)
}

// sanitizeEnvName maps an environment Name to a filesystem-safe base filename.
// Path separators and other characters that are unsafe or awkward in filenames
// are replaced with '_'. The original Environment.Name is preserved verbatim in
// the file's JSON, so this is purely a filename-derivation step. An empty or
// all-unsafe name collapses to "_" so it still yields a valid filename.
//
// Sanitisation is lossy — distinct names like "a/b" and "a:b" (or "." and "..")
// can map to the same sanitised string — so this is NOT used alone for the
// filename. See envFileBase, which appends a hash of the original name to keep
// distinct environments in distinct files.
func sanitizeEnvName(name string) string {
	repl := func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		if r < 0x20 {
			return '_'
		}
		return r
	}
	s := strings.Map(repl, name)
	s = strings.TrimSpace(s)
	// Avoid names that are problematic on disk.
	if s == "" || s == "." || s == ".." {
		s = "_"
	}
	return s
}

// envFileBase derives the base filename (without extension) for an environment
// from its original Name. It joins the human-readable sanitised name with a
// short, stable hash of the ORIGINAL (unsanitised) name so that distinct names
// which sanitise to the same string — e.g. "a/b" vs "a:b", or "." vs ".." — can
// never collide on disk and silently overwrite one another. The hash is the
// first 8 hex chars of SHA-256(name), which is deterministic across runs so
// Save, Load and Delete all derive the same filename for a given Name.
func envFileBase(name string) string {
	sum := sha256.Sum256([]byte(name))
	return sanitizeEnvName(name) + "-" + hex.EncodeToString(sum[:])[:8]
}

// envFilePath returns the path of the file storing the environment named name
// for the collection at collPath.
func envFilePath(collPath, name string) string {
	return filepath.Join(EnvironmentsDir(collPath), envFileBase(name)+envFileExt)
}

// LoadEnvironments reads every environment file for the collection at collPath
// and returns the environments sorted by Name.
//
// For each environment, non-secret variable values come from the environment's
// JSON file; secret variable values (Variable.Secret == true) are filled in from
// the collection's .env file, keyed by the per-environment reserved key
// varSecretKey(env.Name, Variable.Key). For backward compatibility with .env
// files written before secret-variable keys were namespaced, a missing
// namespaced key falls back to the legacy bare Variable.Key. A secret absent
// under both keys is left with an empty Value.
//
// It returns (nil, nil) when collPath is "" (an unsaved collection) or when the
// environments directory does not exist. A file that fails to parse as JSON is
// skipped rather than failing the whole load, so one corrupt file cannot hide the
// rest of the environments.
func LoadEnvironments(collPath string) ([]model.Environment, error) {
	if collPath == "" {
		return nil, nil
	}

	dir := EnvironmentsDir(collPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: read environments dir %q: %w", dir, err)
	}

	secrets, err := readDotenv(dotenvPath(collPath))
	if err != nil {
		return nil, err
	}

	var envs []model.Environment
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), envFileExt) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("store: read %q: %w", path, err)
		}
		var env model.Environment
		if err := json.Unmarshal(data, &env); err != nil {
			// Skip a corrupt environment file rather than failing the load.
			continue
		}
		// Restore secret values from the .env file. Read the per-environment
		// namespaced key first; if it is ABSENT from the map (two-value read
		// distinguishes "namespaced present but empty" from "namespaced absent"),
		// fall back to the legacy bare key so .env files written before secret
		// keys were namespaced still resolve. The next SaveEnvironment migrates
		// the value to the namespaced key.
		for i := range env.Variables {
			if env.Variables[i].Secret {
				nsKey := varSecretKey(env.Name, env.Variables[i].Key)
				if v, ok := secrets[nsKey]; ok {
					env.Variables[i].Value = v
				} else {
					env.Variables[i].Value = secrets[env.Variables[i].Key]
				}
			}
		}
		// Restore the jump host's secret fields from the reserved .env keys.
		if env.JumpHost != nil {
			passphraseKey, passwordKey := jumpHostSecretKeys(env.Name)
			env.JumpHost.Passphrase = secrets[passphraseKey]
			env.JumpHost.Password = secrets[passwordKey]
		}
		envs = append(envs, env)
	}

	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	return envs, nil
}

// SaveEnvironment writes env to its file under EnvironmentsDir(collPath).
//
// Secret variables (Variable.Secret == true) have their Value written to the
// collection's .env file as `KEY=value` and BLANKED in the committed JSON, so
// credentials never enter the environment file. Non-secret variables are written
// normally. The environments directory is created as needed, and the collection
// directory's .gitignore is updated (idempotently) to ignore the .env file.
//
// Secret keys are merged into the existing .env: keys owned by other environments
// are preserved. It returns an error when collPath is "" — an unsaved collection
// has nowhere to persist environments.
func SaveEnvironment(collPath string, env model.Environment) error {
	if collPath == "" {
		return errors.New("store: cannot save environment for an unsaved collection (empty collPath)")
	}

	dir := EnvironmentsDir(collPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create environments dir %q: %w", dir, err)
	}

	// Split secrets out of the JSON and collect them for the .env merge.
	toWrite := model.Environment{Name: env.Name}
	if len(env.Variables) > 0 {
		toWrite.Variables = make([]model.Variable, len(env.Variables))
		copy(toWrite.Variables, env.Variables)
	}
	newSecrets := make(map[string]string)
	for i := range toWrite.Variables {
		if toWrite.Variables[i].Secret {
			// Store under the per-environment namespaced key so two environments
			// with a same-named Secret never share one .env slot. Any pre-fix
			// legacy bare key is left in place (a not-yet-re-saved sibling env may
			// still rely on it via Load's fallback); orphaned legacy bare keys are
			// harmless and gitignored.
			newSecrets[varSecretKey(env.Name, toWrite.Variables[i].Key)] = toWrite.Variables[i].Value
			toWrite.Variables[i].Value = "" // never persist secret values in JSON
		}
	}

	// Split the jump host's secret fields out the same way: copy the JumpHost,
	// move Passphrase/Password into the .env under the reserved keys, and blank
	// them in the committed JSON. Non-secret fields stay in the JSON. A nil
	// JumpHost is left nil so the environment serializes byte-identically to one
	// that never had a jump host.
	if env.JumpHost != nil {
		jh := *env.JumpHost // copy so we don't mutate the caller's value
		passphraseKey, passwordKey := jumpHostSecretKeys(env.Name)
		if jh.Passphrase != "" {
			newSecrets[passphraseKey] = jh.Passphrase
		}
		if jh.Password != "" {
			newSecrets[passwordKey] = jh.Password
		}
		jh.Passphrase = "" // never persist jump-host secrets in JSON
		jh.Password = ""
		toWrite.JumpHost = &jh
	}

	// Merge this environment's secret keys into the existing .env.
	envPath := dotenvPath(collPath)
	secrets, err := readDotenv(envPath)
	if err != nil {
		return err
	}
	for k, v := range newSecrets {
		secrets[k] = v
	}
	if err := writeDotenv(envPath, secrets); err != nil {
		return err
	}

	// Ensure .env is gitignored before (or regardless of) writing the JSON.
	if err := ensureGitignore(filepath.Dir(collPath), dotenvName); err != nil {
		return err
	}

	data, err := json.MarshalIndent(toWrite, "", "  ")
	if err != nil {
		return fmt.Errorf("store: marshal environment %q: %w", env.Name, err)
	}
	data = append(data, '\n')

	path := envFilePath(collPath, env.Name)
	if err := atomicWriteFile(path, data); err != nil {
		return err
	}
	return nil
}

// DeleteEnvironment removes the file for the named environment under
// EnvironmentsDir(collPath). It is not an error if the file does not exist.
//
// The deleted environment's secret keys are removed from the .env file, but only
// when no remaining environment still uses that key, so secrets shared by other
// environments are preserved. It returns an error when collPath is "".
func DeleteEnvironment(collPath, name string) error {
	if collPath == "" {
		return errors.New("store: cannot delete environment for an unsaved collection (empty collPath)")
	}

	// Identify the secret keys this environment declared (before removing it),
	// so we can prune the ones no other environment still needs.
	doomedKeys, err := secretKeysOf(collPath, name)
	if err != nil {
		return err
	}

	path := envFilePath(collPath, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: remove environment %q: %w", path, err)
	}

	if len(doomedKeys) == 0 {
		return nil
	}

	// Determine which keys are still referenced by the surviving environments.
	stillUsed, err := allSecretKeys(collPath)
	if err != nil {
		return err
	}

	envPath := dotenvPath(collPath)
	secrets, err := readDotenv(envPath)
	if err != nil {
		return err
	}
	changed := false
	for k := range doomedKeys {
		if !stillUsed[k] {
			if _, ok := secrets[k]; ok {
				delete(secrets, k)
				changed = true
			}
		}
	}
	if changed {
		if err := writeDotenv(envPath, secrets); err != nil {
			return err
		}
	}
	return nil
}

// secretKeysOf reads the on-disk file for the named environment and returns the
// set of Variable.Key values marked Secret. A missing or corrupt file yields an
// empty set with no error.
func secretKeysOf(collPath, name string) (map[string]struct{}, error) {
	keys := make(map[string]struct{})
	data, err := os.ReadFile(envFilePath(collPath, name))
	if err != nil {
		if os.IsNotExist(err) {
			return keys, nil
		}
		return nil, fmt.Errorf("store: read environment %q: %w", name, err)
	}
	var env model.Environment
	if err := json.Unmarshal(data, &env); err != nil {
		return keys, nil // corrupt: treat as owning no keys
	}
	for _, v := range env.Variables {
		if v.Secret {
			// Collect the per-environment namespaced key (the slot Save writes).
			// Legacy bare keys are intentionally NOT collected so they survive as
			// a Load fallback for any sibling env not yet re-saved; an orphaned
			// legacy bare key is harmless and gitignored.
			keys[varSecretKey(name, v.Key)] = struct{}{}
		}
	}
	// A jump host owns its reserved .env keys; include them so they are pruned
	// when this environment is deleted (they are env-specific, never shared).
	if env.JumpHost != nil {
		passphraseKey, passwordKey := jumpHostSecretKeys(name)
		keys[passphraseKey] = struct{}{}
		keys[passwordKey] = struct{}{}
	}
	return keys, nil
}

// allSecretKeys returns the set of secret Variable.Key values declared by every
// environment currently on disk for the collection.
func allSecretKeys(collPath string) (map[string]bool, error) {
	used := make(map[string]bool)
	dir := EnvironmentsDir(collPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return used, nil
		}
		return nil, fmt.Errorf("store: read environments dir %q: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), envFileExt) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("store: read %q: %w", e.Name(), err)
		}
		var env model.Environment
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		for _, v := range env.Variables {
			if v.Secret {
				// Namespaced var keys are env-specific (derived from this env's
				// Name), so they are never shared; collecting them keeps a
				// surviving env's secret from being pruned. Legacy bare keys are
				// not collected — they remain only as a Load fallback.
				used[varSecretKey(env.Name, v.Key)] = true
			}
		}
		// Reserved jump-host keys are env-specific; derive them from the parsed
		// env's Name so a surviving environment's keys are not pruned.
		if env.JumpHost != nil {
			passphraseKey, passwordKey := jumpHostSecretKeys(env.Name)
			used[passphraseKey] = true
			used[passwordKey] = true
		}
	}
	return used, nil
}

// ensureGitignore makes sure the .gitignore file in dir contains a line ignoring
// entry. It creates the file if absent and appends the line if missing, but never
// duplicates an existing matching line. Comparison ignores surrounding whitespace
// and a leading "/" so "/.env" and ".env" are treated as the same rule.
func ensureGitignore(dir, entry string) error {
	path := filepath.Join(dir, gitignoreName)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: read %q: %w", path, err)
	}

	want := strings.TrimPrefix(strings.TrimSpace(entry), "/")
	for _, line := range strings.Split(string(data), "\n") {
		got := strings.TrimPrefix(strings.TrimSpace(line), "/")
		if got == want {
			return nil // already ignored
		}
	}

	var b strings.Builder
	b.Write(data)
	// Ensure the existing content ends with a newline before appending.
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(entry)
	b.WriteByte('\n')

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create dir %q: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("store: write %q: %w", path, err)
	}
	return nil
}
