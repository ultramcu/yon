package ui

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// srcEnv builds a representative source Environment: two Variables (one Secret)
// and a non-nil JumpHost whose secret fields (Passphrase/Password) are set.
func srcEnv() model.Environment {
	return model.Environment{
		Name: "Staging",
		Variables: []model.Variable{
			{Key: "base", Value: "https://api.example.com", Enabled: true},
			{Key: "token", Value: "s3cr3t", Enabled: true, Secret: true},
		},
		JumpHost: &model.JumpHost{
			Host:       "bastion.example.com",
			Port:       2222,
			User:       "deploy",
			Auth:       "key",
			KeyPath:    "/keys/id_ed25519",
			Insecure:   true,
			Passphrase: "openme",
			Password:   "hunter2",
		},
	}
}

func TestDuplicateEnvironment_DeepCopyIndependent_Blind(t *testing.T) {
	src := srcEnv()
	dup := duplicateEnvironment(src, "Staging copy")

	// Name set to the requested new name.
	if dup.Name != "Staging copy" {
		t.Fatalf("Name = %q, want %q", dup.Name, "Staging copy")
	}

	// Variables copied by value: same length and same field values.
	if len(dup.Variables) != len(src.Variables) {
		t.Fatalf("len(Variables) = %d, want %d", len(dup.Variables), len(src.Variables))
	}
	for i := range src.Variables {
		if dup.Variables[i] != src.Variables[i] {
			t.Errorf("Variables[%d] = %+v, want %+v", i, dup.Variables[i], src.Variables[i])
		}
	}

	// JumpHost copied by value into a non-nil, distinct pointer.
	if dup.JumpHost == nil {
		t.Fatalf("dup.JumpHost is nil, want a copy")
	}
	if dup.JumpHost == src.JumpHost {
		t.Errorf("dup.JumpHost shares the src pointer; want a distinct copy")
	}
	if *dup.JumpHost != *src.JumpHost {
		t.Errorf("*dup.JumpHost = %+v, want %+v", *dup.JumpHost, *src.JumpHost)
	}

	// --- Independence (the teeth) ---
	// Snapshot the src values we expect to remain untouched.
	wantVar0Value := src.Variables[0].Value
	wantVarLen := len(src.Variables)
	wantJHPassword := src.JumpHost.Password
	wantJHHost := src.JumpHost.Host

	// Mutate the copy: element field, the slice itself, and JumpHost fields.
	dup.Variables[0].Value = "MUTATED"
	dup.Variables = append(dup.Variables, model.Variable{Key: "extra", Value: "x", Enabled: true})
	dup.JumpHost.Password = "CHANGED"
	dup.JumpHost.Host = "evil.example.com"

	// src must be completely unaffected.
	if src.Variables[0].Value != wantVar0Value {
		t.Errorf("src.Variables[0].Value leaked: got %q, want %q", src.Variables[0].Value, wantVar0Value)
	}
	if len(src.Variables) != wantVarLen {
		t.Errorf("src.Variables length leaked: got %d, want %d", len(src.Variables), wantVarLen)
	}
	if src.JumpHost.Password != wantJHPassword {
		t.Errorf("src.JumpHost.Password leaked: got %q, want %q", src.JumpHost.Password, wantJHPassword)
	}
	if src.JumpHost.Host != wantJHHost {
		t.Errorf("src.JumpHost.Host leaked: got %q, want %q", src.JumpHost.Host, wantJHHost)
	}
}

func TestDuplicateEnvironment_NilJumpHost_Blind(t *testing.T) {
	src := model.Environment{
		Name: "NoHost",
		Variables: []model.Variable{
			{Key: "k", Value: "v", Enabled: true},
		},
		JumpHost: nil,
	}

	dup := duplicateEnvironment(src, "NoHost copy")

	if dup.JumpHost != nil {
		t.Errorf("dup.JumpHost = %+v, want nil", *dup.JumpHost)
	}
	if dup.Name != "NoHost copy" {
		t.Errorf("Name = %q, want %q", dup.Name, "NoHost copy")
	}
}

func TestUniqueEnvName_Blind(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		existing []string
		want     string
	}{
		{
			name:     "no existing names",
			base:     "Staging",
			existing: nil,
			want:     "Staging copy",
		},
		{
			name:     "first copy taken",
			base:     "Staging",
			existing: []string{"Staging copy"},
			want:     "Staging copy 2",
		},
		{
			name:     "first and second taken",
			base:     "Staging",
			existing: []string{"Staging copy", "Staging copy 2"},
			want:     "Staging copy 3",
		},
		{
			name:     "case-insensitive collision",
			base:     "Staging",
			existing: []string{"staging COPY"},
			want:     "Staging copy 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueEnvName(tt.base, tt.existing)
			if got != tt.want {
				t.Errorf("uniqueEnvName(%q, %v) = %q, want %q", tt.base, tt.existing, got, tt.want)
			}
		})
	}
}
