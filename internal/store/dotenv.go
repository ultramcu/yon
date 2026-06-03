package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// parseDotenv parses dotenv-format data into a key→value map.
//
// The format is one `KEY=value` assignment per line. Blank lines and lines whose
// first non-space character is `#` are ignored. Surrounding whitespace is trimmed
// from both the key and the value, and a single pair of matching surrounding
// quotes (either '"' or '\”) is stripped from the value. For a double-quoted
// value the escape sequences written by quoteDotenvValue are reversed in a
// single left-to-right pass (`\n`→newline, `\"`→'"', `\\`→'\'), so a value
// containing newlines, quotes or backslashes round-trips byte-identically.
// Single-quoted and unquoted values are taken literally. A line without an `=`,
// or with an empty key, is skipped rather than treated as an error so a slightly
// malformed .env never blocks loading a collection. On duplicate keys the last
// assignment wins.
func parseDotenv(data []byte) map[string]string {
	out := make(map[string]string)
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			continue
		}
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 {
			switch {
			case val[0] == '"' && val[len(val)-1] == '"':
				// Double-quoted: strip the quotes then reverse the writer's
				// escaping exactly (see quoteDotenvValue).
				val = unescapeDotenvValue(val[1 : len(val)-1])
			case val[0] == '\'' && val[len(val)-1] == '\'':
				// Single-quoted: literal, just strip the quotes.
				val = val[1 : len(val)-1]
			}
		}
		out[key] = val
	}
	return out
}

// readDotenv reads and parses the .env file at path. A missing file is not an
// error: it yields an empty (non-nil) map. Other read errors are returned.
func readDotenv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("store: read %q: %w", path, err)
	}
	return parseDotenv(data), nil
}

// writeDotenv writes the key→value pairs to the .env file at path in dotenv
// format, one `KEY=value` per line, sorted by key for stable, git-friendly
// output. The value is wrapped in double quotes only when it contains a
// character that would otherwise change its parsed meaning (leading/trailing
// space, quotes, '#', or newline); inner double quotes and backslashes are
// escaped. If pairs is empty the file is removed so no stray empty .env lingers.
// The write is atomic (temp file + rename).
func writeDotenv(path string, pairs map[string]string) error {
	if len(pairs) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: remove empty .env %q: %w", path, err)
		}
		return nil
	}

	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(quoteDotenvValue(pairs[k]))
		b.WriteByte('\n')
	}

	return atomicWriteFile(path, []byte(b.String()))
}

// quoteDotenvValue returns v formatted for the right-hand side of a dotenv
// assignment, adding double quotes and escaping only when needed so common
// values stay unquoted and readable.
func quoteDotenvValue(v string) string {
	needQuote := v != strings.TrimSpace(v) ||
		strings.ContainsAny(v, "\"'#\n")
	if !needQuote {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(v) + `"`
}

// unescapeDotenvValue reverses quoteDotenvValue's escaping for the contents of a
// double-quoted value (quotes already stripped). It scans left-to-right so each
// backslash consumes exactly one following character: `\n`→newline, `\"`→'"',
// `\\`→'\'. This makes `\\n` decode to a backslash followed by 'n' (not a
// newline). A trailing lone backslash, or any other `\x`, is passed through with
// the backslash preserved.
func unescapeDotenvValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case '"':
				b.WriteByte('"')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// atomicWriteFile writes data to path via a temporary file in the same directory
// followed by a rename, so a failed write never leaves a partial file at path.
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create dir %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".yon-*.tmp")
	if err != nil {
		return fmt.Errorf("store: create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store: write temp file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: close temp file %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("store: rename %q to %q: %w", tmpName, path, err)
	}
	return nil
}
