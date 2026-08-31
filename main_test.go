package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePublicPath_PrefersEnvThenConfig(t *testing.T) {
	got, err := resolvePublicPath("/from/env", "/from/cfg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/from/env" {
		t.Errorf("got %q, want env path", got)
	}

	got, err = resolvePublicPath("", "/from/cfg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/from/cfg" {
		t.Errorf("got %q, want config path", got)
	}
}

func TestResolvePublicPath_FallbackNextToBinary(t *testing.T) {
	got, err := resolvePublicPath("", "")
	if err != nil {
		t.Fatalf("executable fallback failed: %v", err)
	}
	if filepath.Base(got) != "public" {
		t.Errorf("got %q, want a path ending in public", got)
	}
}

func TestResolvePublicPath_ExecutableError(t *testing.T) {
	orig := lookUpExecutable
	lookUpExecutable = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { lookUpExecutable = orig })

	_, err := resolvePublicPath("", "")
	if err == nil {
		t.Fatal("expected error when executable lookup fails")
	}
	if !strings.Contains(err.Error(), "cannot resolve static files") {
		t.Errorf("error should name the static-path failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected wrapped cause, got %v", err)
	}
}
