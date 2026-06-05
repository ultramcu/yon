package ui

import (
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// Blind tests for the Variables inspector (issue #29): the pure collector
// collectVariableView and the secret-masking renderer renderVarLine. Written
// from the contract, against the public-to-package symbols only:
// varView{Key,Value,Secret,Scope}, collectVariableView and renderVarLine.

// TestCollectVariableView pins the projection contract: env ENABLED vars first
// (Scope "env"), then collection ENABLED vars (Scope "collection") whose Key did
// not already appear in the env (env wins on clash); disabled vars dropped;
// runtime entries become Scope "runtime" rows sorted by Key with Secret=false.
func TestCollectVariableView(t *testing.T) {
	env := model.Environment{
		Name: "Local",
		Variables: []model.Variable{
			{Key: "baseUrl", Value: "http://x", Enabled: true},
			{Key: "apiKey", Value: "topsecret", Enabled: true, Secret: true},
		},
	}
	collVars := []model.Variable{
		{Key: "bearerToken", Value: "tok", Enabled: true, Secret: true},
		// Dropped: env already defines baseUrl, so env precedence hides this one.
		{Key: "baseUrl", Value: "SHOULD_BE_HIDDEN", Enabled: true},
		// Dropped: disabled vars are excluded entirely.
		{Key: "skip", Value: "nope", Enabled: false},
	}
	runtime := map[string]string{"userId": "42", "appName": "Yon"}

	configured, runtimeRows := collectVariableView(env, collVars, runtime)

	wantConfigured := []varView{
		{Key: "baseUrl", Value: "http://x", Secret: false, Scope: "env"},
		{Key: "apiKey", Value: "topsecret", Secret: true, Scope: "env"},
		{Key: "bearerToken", Value: "tok", Secret: true, Scope: "collection"},
	}
	if len(configured) != len(wantConfigured) {
		t.Fatalf("configured length = %d (%+v), want %d (%+v)",
			len(configured), configured, len(wantConfigured), wantConfigured)
	}
	for i, want := range wantConfigured {
		if configured[i] != want {
			t.Errorf("configured[%d] = %+v, want %+v", i, configured[i], want)
		}
	}

	// The dropped collection baseUrl must never surface (env precedence), and
	// its value must not leak into the configured rows.
	for _, v := range configured {
		if v.Scope == "collection" && v.Key == "baseUrl" {
			t.Errorf("collection baseUrl should be dropped (env wins), got %+v", v)
		}
		if v.Value == "SHOULD_BE_HIDDEN" {
			t.Errorf("hidden collection baseUrl value leaked into %+v", v)
		}
		if v.Key == "skip" {
			t.Errorf("disabled var skip should be excluded, got %+v", v)
		}
	}

	wantRuntime := []varView{
		{Key: "appName", Value: "Yon", Secret: false, Scope: "runtime"},
		{Key: "userId", Value: "42", Secret: false, Scope: "runtime"},
	}
	if len(runtimeRows) != len(wantRuntime) {
		t.Fatalf("runtimeRows length = %d (%+v), want %d (%+v)",
			len(runtimeRows), runtimeRows, len(wantRuntime), wantRuntime)
	}
	for i, want := range wantRuntime {
		if runtimeRows[i] != want {
			t.Errorf("runtimeRows[%d] = %+v, want %+v", i, runtimeRows[i], want)
		}
	}

	// No-env case: an empty environment contributes no rows, so configured is
	// just the (enabled, non-clashing) collection vars. nil runtime → empty.
	cfgNoEnv, rtNil := collectVariableView(model.Environment{}, collVars, nil)
	for _, v := range cfgNoEnv {
		if v.Scope == "env" {
			t.Errorf("empty env must yield no env-scope rows, got %+v", v)
		}
	}
	wantNoEnv := []varView{
		{Key: "bearerToken", Value: "tok", Secret: true, Scope: "collection"},
		{Key: "baseUrl", Value: "SHOULD_BE_HIDDEN", Secret: false, Scope: "collection"},
	}
	if len(cfgNoEnv) != len(wantNoEnv) {
		t.Fatalf("no-env configured length = %d (%+v), want %d (%+v)",
			len(cfgNoEnv), cfgNoEnv, len(wantNoEnv), wantNoEnv)
	}
	for i, want := range wantNoEnv {
		if cfgNoEnv[i] != want {
			t.Errorf("no-env configured[%d] = %+v, want %+v", i, cfgNoEnv[i], want)
		}
	}
	if len(rtNil) != 0 {
		t.Errorf("nil runtime must yield empty runtimeRows, got %+v", rtNil)
	}
}

// TestRenderVarLine pins the renderer contract: plain rows render "key = value",
// secret rows mask the value with ••••, and the secret's real value must NEVER
// appear in the rendered output (the privacy guarantee).
func TestRenderVarLine(t *testing.T) {
	if got := renderVarLine(varView{Key: "baseUrl", Value: "http://x"}); got != "baseUrl = http://x" {
		t.Errorf("non-secret renderVarLine = %q, want %q", got, "baseUrl = http://x")
	}

	secret := renderVarLine(varView{Key: "apiKey", Value: "topsecret", Secret: true})
	if !strings.Contains(secret, "apiKey") {
		t.Errorf("secret line %q must contain the key %q", secret, "apiKey")
	}
	if !strings.Contains(secret, "••••") {
		t.Errorf("secret line %q must contain the mask %q", secret, "••••")
	}
	if strings.Contains(secret, "topsecret") {
		t.Errorf("PRIVACY LEAK: secret line %q must not contain the real value %q", secret, "topsecret")
	}

	if got := renderVarLine(varView{Key: "userId", Value: "42", Scope: "runtime"}); got != "userId = 42" {
		t.Errorf("runtime renderVarLine = %q, want %q", got, "userId = 42")
	}
}
