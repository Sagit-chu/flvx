package config

import "testing"

func TestDefaultRuntimeEngine(t *testing.T) {
	t.Setenv("RUNTIME_ENGINE_DEFAULT", "dash")
	if got := DefaultRuntimeEngine(); got != "dash" {
		t.Fatalf("expected dash, got %q", got)
	}

	t.Setenv("RUNTIME_ENGINE_DEFAULT", "  GOST  ")
	if got := DefaultRuntimeEngine(); got != "gost" {
		t.Fatalf("expected gost, got %q", got)
	}

	t.Setenv("RUNTIME_ENGINE_DEFAULT", "invalid")
	if got := DefaultRuntimeEngine(); got != "gost" {
		t.Fatalf("expected invalid value to clamp to gost, got %q", got)
	}

	t.Setenv("RUNTIME_ENGINE_DEFAULT", "")
	if got := DefaultRuntimeEngine(); got != "gost" {
		t.Fatalf("expected empty value to default to gost, got %q", got)
	}
}
