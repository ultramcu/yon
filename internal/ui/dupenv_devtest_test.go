package ui

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestDuplicateEnvironmentVariablesIndependence checks that mutating the copy's
// Variables (the slice backing array and an element's fields) never reaches back
// into the source environment.
func TestDuplicateEnvironmentVariablesIndependence(t *testing.T) {
	src := model.Environment{
		Name: "Staging",
		Variables: []model.Variable{
			{Key: "host", Value: "stage.example.com", Enabled: true},
			{Key: "token", Value: "abc", Enabled: true, Secret: true},
		},
	}

	dup := duplicateEnvironment(src, "Staging copy")

	if dup.Name != "Staging copy" {
		t.Fatalf("Name = %q, want %q", dup.Name, "Staging copy")
	}
	if len(dup.Variables) != len(src.Variables) {
		t.Fatalf("len(Variables) = %d, want %d", len(dup.Variables), len(src.Variables))
	}

	// Mutating a copied element must not touch the source element.
	dup.Variables[0].Value = "MUTATED"
	if src.Variables[0].Value != "stage.example.com" {
		t.Errorf("mutating dup.Variables[0].Value changed src: got %q", src.Variables[0].Value)
	}

	// The slices must not share a backing array: appending to dup must not be
	// observable through src.
	dup.Variables = append(dup.Variables, model.Variable{Key: "extra"})
	if len(src.Variables) != 2 {
		t.Errorf("appending to dup.Variables changed src length: got %d", len(src.Variables))
	}
}

// TestDuplicateEnvironmentJumpHostIndependence checks that the copy gets its own
// *JumpHost whose fields can be mutated without affecting the source.
func TestDuplicateEnvironmentJumpHostIndependence(t *testing.T) {
	src := model.Environment{
		Name: "Prod",
		JumpHost: &model.JumpHost{
			Host:       "bastion.example.com",
			Port:       2222,
			User:       "deploy",
			Auth:       model.JumpAuthPassword,
			Password:   "s3cret",
			Passphrase: "pp",
			KeyPath:    "/k",
			Insecure:   true,
		},
	}

	dup := duplicateEnvironment(src, "Prod copy")

	if dup.JumpHost == nil {
		t.Fatal("dup.JumpHost is nil, want a copy")
	}
	if dup.JumpHost == src.JumpHost {
		t.Fatal("dup.JumpHost shares the same pointer as src.JumpHost")
	}
	if *dup.JumpHost != *src.JumpHost {
		t.Fatalf("dup.JumpHost value = %+v, want equal to src %+v", *dup.JumpHost, *src.JumpHost)
	}

	dup.JumpHost.Host = "MUTATED"
	dup.JumpHost.Password = "MUTATED"
	if src.JumpHost.Host != "bastion.example.com" {
		t.Errorf("mutating dup.JumpHost.Host changed src: got %q", src.JumpHost.Host)
	}
	if src.JumpHost.Password != "s3cret" {
		t.Errorf("mutating dup.JumpHost.Password changed src: got %q", src.JumpHost.Password)
	}
}

// TestDuplicateEnvironmentNilJumpHost checks a source without a jump host yields
// a copy that also has none (and doesn't panic).
func TestDuplicateEnvironmentNilJumpHost(t *testing.T) {
	src := model.Environment{Name: "Local"}
	dup := duplicateEnvironment(src, "Local copy")
	if dup.JumpHost != nil {
		t.Errorf("dup.JumpHost = %+v, want nil", dup.JumpHost)
	}
}

// TestUniqueEnvName covers the collision sequence, including case-insensitive
// matching against existing names.
func TestUniqueEnvName(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		existing []string
		want     string
	}{
		{
			name:     "no collision",
			base:     "Staging",
			existing: []string{"Staging", "Prod"},
			want:     "Staging copy",
		},
		{
			name:     "first copy taken",
			base:     "Staging",
			existing: []string{"Staging", "Staging copy"},
			want:     "Staging copy 2",
		},
		{
			name:     "copy and copy 2 taken",
			base:     "Staging",
			existing: []string{"Staging", "Staging copy", "Staging copy 2"},
			want:     "Staging copy 3",
		},
		{
			name:     "case-insensitive collision",
			base:     "Staging",
			existing: []string{"staging copy", "STAGING COPY 2"},
			want:     "Staging copy 3",
		},
		{
			name:     "base already ends in copy",
			base:     "Dev copy",
			existing: []string{"Dev copy"},
			want:     "Dev copy copy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uniqueEnvName(tt.base, tt.existing); got != tt.want {
				t.Errorf("uniqueEnvName(%q, %v) = %q, want %q", tt.base, tt.existing, got, tt.want)
			}
		})
	}
}
