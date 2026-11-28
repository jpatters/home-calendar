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

	if _, err := openStores(filepath.Join(dir, "config.json")); err == nil {
		t.Errorf("openStores err = nil, want startup to stop on a corrupt secrets file")
	}
}

func TestStartupOpensConfigSecretsAndMemoriesTogether(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", "")
	dir := t.TempDir()

	st, err := openStores(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("openStores: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets.json")); err != nil {
		t.Errorf("secrets.json not created beside config.json: %v", err)
	}
	if _, err := st.memories.Add("A fact."); err != nil {
		t.Fatalf("Add memory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "memories.json")); err != nil {
		t.Errorf("memories.json not saved beside config.json: %v", err)
	}
}

func TestStartupFailsOnACorruptMemoriesFile(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "memories.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := openStores(filepath.Join(dir, "config.json")); err == nil {
		t.Errorf("openStores err = nil, want startup to stop on a corrupt memories file")
	}
}

// os.Getenv reports an unset TZ as "", and compose passes TZ="" when .env
// has none; Go treats both as UTC.
func TestZoneFallsBackToTheWeatherLocationWhenTZIsEmpty(t *testing.T) {
	if loc := zoneFallback("", "America/Halifax"); loc == nil || loc.String() != "America/Halifax" {
		t.Errorf("zoneFallback = %v, want America/Halifax", loc)
	}
}

func TestZoneKeepsAnExplicitTZ(t *testing.T) {
	if loc := zoneFallback("America/Toronto", "America/Halifax"); loc != nil {
		t.Errorf("zoneFallback = %v, want nil so TZ is honoured", loc)
	}
}

func TestZoneKeepsTheDefaultWithoutAUsableWeatherZone(t *testing.T) {
	for _, weatherTZ := range []string{"", "auto", "Not/AZone"} {
		if loc := zoneFallback("", weatherTZ); loc != nil {
			t.Errorf("zoneFallback(weather %q) = %v, want nil", weatherTZ, loc)
		}
	}
}
