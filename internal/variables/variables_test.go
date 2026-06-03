package variables

import (
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/ultramcu/yon/internal/model"
)

// helpers ---------------------------------------------------------------------

func env(vars ...model.Variable) model.Environment {
	return model.Environment{Name: "test", Variables: vars}
}

func v(key, value string, enabled bool) model.Variable {
	return model.Variable{Key: key, Value: value, Enabled: enabled}
}

// 1. Lookup precedence --------------------------------------------------------

func TestLookup_EnvBeatsCollection(t *testing.T) {
	sc := Scope{
		Env:        env(v("host", "prod", true)),
		Collection: []model.Variable{v("host", "local", true)},
	}
	got, ok := sc.Lookup("host")
	if !ok || got != "prod" {
		t.Fatalf("Lookup(host) = %q,%v; want \"prod\",true (active env wins)", got, ok)
	}
}

func TestLookup_CollectionWhenEnvLacks(t *testing.T) {
	sc := Scope{
		Env:        env(v("other", "x", true)),
		Collection: []model.Variable{v("host", "local", true)},
	}
	got, ok := sc.Lookup("host")
	if !ok || got != "local" {
		t.Fatalf("Lookup(host) = %q,%v; want \"local\",true (collection fallback)", got, ok)
	}
}

func TestLookup_DisabledEnvFallsThrough(t *testing.T) {
	sc := Scope{
		Env:        env(v("host", "prod", false)), // disabled
		Collection: []model.Variable{v("host", "local", true)},
	}
	got, ok := sc.Lookup("host")
	if !ok || got != "local" {
		t.Fatalf("Lookup(host) = %q,%v; want \"local\",true (disabled env skipped)", got, ok)
	}
}

func TestLookup_UnknownKey(t *testing.T) {
	sc := Scope{Env: env(v("host", "prod", true))}
	got, ok := sc.Lookup("missing")
	if ok || got != "" {
		t.Fatalf("Lookup(missing) = %q,%v; want \"\",false", got, ok)
	}
}

// 2. Secret -------------------------------------------------------------------

func TestLookup_SecretValueFromSecretsMap(t *testing.T) {
	sc := Scope{
		Env:     env(model.Variable{Key: "token", Value: "", Enabled: true, Secret: true}),
		Secrets: map[string]string{"token": "s3cret"},
	}
	got, ok := sc.Lookup("token")
	if !ok || got != "s3cret" {
		t.Fatalf("Lookup(token) = %q,%v; want \"s3cret\",true (secret from Secrets map)", got, ok)
	}
}

// 3. Resolve embedded ---------------------------------------------------------

func TestResolve_Embedded(t *testing.T) {
	sc := Scope{
		Env: env(v("base", "http://h", true), v("id", "42", true)),
	}
	got := sc.Resolve("{{base}}/u?i={{id}}")
	if want := "http://h/u?i=42"; got != want {
		t.Fatalf("Resolve = %q; want %q", got, want)
	}
}

// 4. Unknown left literal -----------------------------------------------------

func TestResolve_UnknownLiteral(t *testing.T) {
	sc := Scope{Env: env(v("known", "y", true))}
	got := sc.Resolve("{{nope}}/x")
	if want := "{{nope}}/x"; got != want {
		t.Fatalf("Resolve = %q; want %q (unknown stays literal)", got, want)
	}
}

// 5. Trim ---------------------------------------------------------------------

func TestResolve_TrimsSpaces(t *testing.T) {
	sc := Scope{Env: env(v("base", "http://h", true))}
	spaced := sc.Resolve("{{ base }}")
	tight := sc.Resolve("{{base}}")
	if spaced != tight {
		t.Fatalf("Resolve(\"{{ base }}\") = %q; want == Resolve(\"{{base}}\") = %q", spaced, tight)
	}
	if spaced != "http://h" {
		t.Fatalf("Resolve(\"{{ base }}\") = %q; want \"http://h\"", spaced)
	}
}

// 6. Multi-pass ---------------------------------------------------------------

func TestResolve_MultiPass(t *testing.T) {
	sc := Scope{
		Env: env(v("a", "{{b}}", true), v("b", "end", true)),
	}
	got := sc.Resolve("{{a}}")
	if want := "end"; got != want {
		t.Fatalf("Resolve(\"{{a}}\") = %q; want %q (value contains another ref)", got, want)
	}
}

// 7. Cycle guard --------------------------------------------------------------

func TestResolve_CycleGuardTerminates(t *testing.T) {
	sc := Scope{
		Env: env(v("a", "{{b}}", true), v("b", "{{a}}", true)),
	}
	done := make(chan string, 1)
	go func() {
		done <- sc.Resolve("{{a}}")
	}()
	select {
	case <-done:
		// completed without hanging; exact output intentionally not asserted
	case <-time.After(2 * time.Second):
		t.Fatal("Resolve hung on a->b->a cycle; cycle guard missing")
	}
}

// 8. Dynamics -----------------------------------------------------------------

var (
	uuidV4Re = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	digitsRe = regexp.MustCompile(`^[0-9]+$`)
)

func TestResolve_DynamicUUIDv4(t *testing.T) {
	sc := Scope{}
	got := sc.Resolve("{{$uuid}}")
	if !uuidV4Re.MatchString(got) {
		t.Fatalf("Resolve({{$uuid}}) = %q; want a UUIDv4", got)
	}
}

func TestResolve_DynamicTwoUUIDsDiffer(t *testing.T) {
	sc := Scope{}
	got := sc.Resolve("{{$uuid}} {{$uuid}}")
	re := regexp.MustCompile(`^(\S+) (\S+)$`)
	m := re.FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("Resolve(two uuids) = %q; unexpected shape", got)
	}
	if !uuidV4Re.MatchString(m[1]) || !uuidV4Re.MatchString(m[2]) {
		t.Fatalf("Resolve(two uuids) parts not both UUIDv4: %q", got)
	}
	if m[1] == m[2] {
		t.Fatalf("two {{$uuid}} produced identical values %q; want different", m[1])
	}
}

func TestResolve_DynamicTimestampDigits(t *testing.T) {
	sc := Scope{}
	got := sc.Resolve("{{$timestamp}}")
	if !digitsRe.MatchString(got) {
		t.Fatalf("Resolve({{$timestamp}}) = %q; want all digits", got)
	}
}

func TestResolve_DynamicRandomIntRange(t *testing.T) {
	sc := Scope{}
	for i := 0; i < 50; i++ {
		got := sc.Resolve("{{$randomInt}}")
		n, err := strconv.Atoi(got)
		if err != nil {
			t.Fatalf("Resolve({{$randomInt}}) = %q; not an int: %v", got, err)
		}
		if n < 0 || n > 1000 {
			t.Fatalf("Resolve({{$randomInt}}) = %d; want in [0,1000]", n)
		}
	}
}

func TestResolve_DynamicIsoTimestamp(t *testing.T) {
	sc := Scope{}
	got := sc.Resolve("{{$isoTimestamp}}")
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("Resolve({{$isoTimestamp}}) = %q; not RFC3339: %v", got, err)
	}
}

// 9. Disabled collection var skipped ------------------------------------------

func TestLookup_DisabledCollectionSkipped(t *testing.T) {
	sc := Scope{
		Collection: []model.Variable{
			v("host", "disabled-val", false),
			v("host", "enabled-val", true),
		},
	}
	got, ok := sc.Lookup("host")
	if !ok || got != "enabled-val" {
		t.Fatalf("Lookup(host) = %q,%v; want \"enabled-val\",true (disabled collection var skipped)", got, ok)
	}
}
