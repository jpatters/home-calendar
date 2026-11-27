package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartupFailsOnACorruptSecretsFile(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := openStores(filepath.Join(dir, "config.json")); err == nil {
		t.Errorf("openStores err = nil, want startup to stop on a corrupt secrets file")
	}
}

func TestStartupOpensConfigAndSecretsTogether(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", "")
	dir := t.TempDir()

	if _, _, err := openStores(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("openStores: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets.json")); err != nil {
		t.Errorf("secrets.json not created beside config.json: %v", err)
	}
}
