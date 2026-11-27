package secrets_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/secrets"
)

func openIn(t *testing.T, dir string) *secrets.Store {
	t.Helper()
	s, err := secrets.Open(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func noEnv(t *testing.T) {
	t.Helper()
	t.Setenv(secrets.EnvOpenAIKey, "")
	t.Setenv(secrets.EnvGoogleServiceAccount, "")
}

func TestPathForSitsBesideTheConfigFile(t *testing.T) {
	got := secrets.PathFor("/data/config.json")
	if got != filepath.Join("/data", "secrets.json") {
		t.Errorf("PathFor = %q, want /data/secrets.json", got)
	}
}

func TestSavedSecretsSurviveReopen(t *testing.T) {
	noEnv(t)
	dir := t.TempDir()
	s := openIn(t, dir)
	expires := time.Date(2026, 11, 27, 3, 30, 0, 0, time.UTC)

	if err := s.SetOpenAIKey("sk-stored"); err != nil {
		t.Fatalf("SetOpenAIKey: %v", err)
	}
	if err := s.SetGoogleServiceAccount(`{"type":"service_account"}`); err != nil {
		t.Fatalf("SetGoogleServiceAccount: %v", err)
	}
	if err := s.UpdateChatGPT(func(c *secrets.ChatGPT) {
		c.ClientID = "oaiapp_1"
		c.Subject = "user-1"
		c.Email = "me@example.com"
		c.IDToken = "id-1"
		c.AccessToken = "at-1"
		c.RefreshToken = "rt-1"
		c.Scopes = []string{"openid", "chatgpt.tokens.use.direct"}
		c.ExpiresAt = expires
	}); err != nil {
		t.Fatalf("UpdateChatGPT: %v", err)
	}
	before := s.ChatGPT()

	r := openIn(t, dir)

	if got := r.OpenAIKey(); got != (secrets.Value{Value: "sk-stored"}) {
		t.Errorf("OpenAIKey after reopen = %+v, want the stored key", got)
	}
	if got := r.GoogleServiceAccount(); got != (secrets.Value{Value: `{"type":"service_account"}`}) {
		t.Errorf("GoogleServiceAccount after reopen = %+v, want the stored JSON", got)
	}
	after := r.ChatGPT()
	if !after.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt after reopen = %v, want %v", after.ExpiresAt, expires)
	}
	after.ExpiresAt, before.ExpiresAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("ChatGPT after reopen = %+v, want %+v", after, before)
	}
}

func TestClearRemovesStoredValues(t *testing.T) {
	noEnv(t)
	dir := t.TempDir()
	s := openIn(t, dir)
	s.SetOpenAIKey("sk-stored")
	s.SetGoogleServiceAccount(`{"type":"service_account"}`)

	if err := s.ClearOpenAIKey(); err != nil {
		t.Fatalf("ClearOpenAIKey: %v", err)
	}
	if err := s.ClearGoogleServiceAccount(); err != nil {
		t.Fatalf("ClearGoogleServiceAccount: %v", err)
	}

	r := openIn(t, dir)
	if got := r.OpenAIKey(); got.Value != "" {
		t.Errorf("OpenAIKey = %+v, want cleared", got)
	}
	if got := r.GoogleServiceAccount(); got.Value != "" {
		t.Errorf("GoogleServiceAccount = %+v, want cleared", got)
	}
}

func TestSavingTightensFileToOwnerOnlyAndLeavesNoTempFile(t *testing.T) {
	noEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")
	// A file left world-readable (e.g. created by hand) is tightened on save.
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := secrets.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s.SetOpenAIKey("sk-stored"); err != nil {
		t.Fatalf("SetOpenAIKey: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secrets.json mode = %o, want 600", perm)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "secrets.json" {
			t.Errorf("leftover file %q in the secrets directory", e.Name())
		}
	}
}

func TestOpenCreatesOwnerOnlyFileWithStableHostID(t *testing.T) {
	noEnv(t)
	dir := t.TempDir()
	s := openIn(t, dir)

	host := s.ChatGPT().HostID
	uuidURN := regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidURN.MatchString(host) {
		t.Errorf("HostID = %q, want a urn:uuid: UUIDv4", host)
	}
	info, err := os.Stat(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatalf("Open did not persist the host id: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secrets.json mode = %o, want 600", perm)
	}
	if again := openIn(t, dir).ChatGPT().HostID; again != host {
		t.Errorf("HostID after reopen = %q, want the same %q", again, host)
	}
	if other := openIn(t, t.TempDir()).ChatGPT().HostID; other == host {
		t.Errorf("two hosts share HostID %q", host)
	}
}

func TestEnvironmentOverridesStoredValuesAndCannotBeChanged(t *testing.T) {
	noEnv(t)
	dir := t.TempDir()
	s := openIn(t, dir)
	s.SetOpenAIKey("sk-stored")
	s.SetGoogleServiceAccount(`{"stored":true}`)

	t.Setenv(secrets.EnvOpenAIKey, "sk-env")
	t.Setenv(secrets.EnvGoogleServiceAccount, "/run/secrets/google.json")
	s = openIn(t, dir)

	if got := s.OpenAIKey(); got != (secrets.Value{Value: "sk-env", FromEnv: true}) {
		t.Errorf("OpenAIKey = %+v, want the environment value", got)
	}
	if got := s.GoogleServiceAccount(); got != (secrets.Value{Value: "/run/secrets/google.json", FromEnv: true}) {
		t.Errorf("GoogleServiceAccount = %+v, want the environment value", got)
	}
	for name, err := range map[string]error{
		"SetOpenAIKey":              s.SetOpenAIKey("sk-new"),
		"ClearOpenAIKey":            s.ClearOpenAIKey(),
		"SetGoogleServiceAccount":   s.SetGoogleServiceAccount(`{"new":true}`),
		"ClearGoogleServiceAccount": s.ClearGoogleServiceAccount(),
	} {
		if !errors.Is(err, secrets.ErrFromEnv) {
			t.Errorf("%s err = %v, want ErrFromEnv", name, err)
		}
	}

	t.Setenv(secrets.EnvOpenAIKey, "")
	t.Setenv(secrets.EnvGoogleServiceAccount, "")
	s = openIn(t, dir)
	if got := s.OpenAIKey(); got != (secrets.Value{Value: "sk-stored"}) {
		t.Errorf("OpenAIKey without env = %+v, want the untouched stored key", got)
	}
	if got := s.GoogleServiceAccount(); got != (secrets.Value{Value: `{"stored":true}`}) {
		t.Errorf("GoogleServiceAccount without env = %+v, want the untouched stored value", got)
	}
}

func TestConcurrentUpdatesAreNotLost(t *testing.T) {
	noEnv(t)
	dir := t.TempDir()
	s := openIn(t, dir)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.UpdateChatGPT(func(c *secrets.ChatGPT) { c.Scopes = append(c.Scopes, "x") })
		}()
	}
	wg.Wait()

	if n := len(openIn(t, dir).ChatGPT().Scopes); n != 20 {
		t.Errorf("scopes after 20 concurrent appends = %d, want 20", n)
	}
}

func TestOpenRefusesACorruptFileInsteadOfReplacingIt(t *testing.T) {
	noEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")
	corrupt := []byte(`{"chatgpt":{"hostId":"urn:uuid:`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := secrets.Open(path); err == nil {
		t.Errorf("Open err = nil, want a corrupt secrets file reported")
	}
	if got, _ := os.ReadFile(path); string(got) != string(corrupt) {
		t.Errorf("corrupt file was rewritten to %q, want it left for the owner to repair", got)
	}
}
