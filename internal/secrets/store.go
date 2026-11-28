// Package secrets keeps the assistant's credentials (OpenAI API key, Google
// service account, ChatGPT sign-in) in an owner-only file beside config.json.
// Nothing here is ever part of the config that is broadcast to browsers.
package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// EnvOpenAIKey, when non-empty, overrides the stored OpenAI API key.
	EnvOpenAIKey = "OPENAI_API_KEY"
	// EnvGoogleServiceAccount, when non-empty, overrides the stored Google
	// service account (the JSON itself or a path to it).
	EnvGoogleServiceAccount = "GOOGLE_SERVICE_ACCOUNT_JSON"
	// EnvHomeAssistantURL and EnvHomeAssistantToken, when the token is
	// non-empty, override the stored Home Assistant connection.
	EnvHomeAssistantURL   = "HOME_ASSISTANT_URL"
	EnvHomeAssistantToken = "HOME_ASSISTANT_TOKEN"
)

// ErrFromEnv reports an attempt to change a value that an environment
// variable sets.
var ErrFromEnv = errors.New("secrets: value is set by an environment variable")

// PathFor returns the secrets file path for a config file path: secrets.json
// in the same directory.
func PathFor(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "secrets.json")
}

// Value is a secret and whether it came from the environment.
type Value struct {
	Value   string
	FromEnv bool
}

// ChatGPT is the saved "Sign in with ChatGPT" registration for this host.
type ChatGPT struct {
	// ClientID is the issued per-user client id (oaiapp_...); empty before
	// the first registration.
	ClientID string `json:"clientId,omitempty"`
	// HostID is this host's stable ext_agent_host_id (urn:uuid:...).
	HostID       string    `json:"hostId"`
	Subject      string    `json:"subject,omitempty"`
	Email        string    `json:"email,omitempty"`
	IDToken      string    `json:"idToken,omitempty"`
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitzero"`
}

// HomeAssistant is the connection Ziggy uses to control home devices: the
// instance's base URL and a long-lived access token.
type HomeAssistant struct {
	URL     string `json:"url,omitempty"`
	Token   string `json:"token,omitempty"`
	FromEnv bool   `json:"-"`
}

type fileData struct {
	OpenAIKey            string        `json:"openaiApiKey,omitempty"`
	GoogleServiceAccount string        `json:"googleServiceAccount,omitempty"`
	HomeAssistant        HomeAssistant `json:"homeAssistant,omitzero"`
	ChatGPT              ChatGPT       `json:"chatgpt"`
}

// Store is the secrets file. Environment overrides are read once, at Open.
type Store struct {
	path      string
	envKey    string
	envGoogle string
	envHA     HomeAssistant
	mu        sync.Mutex
	data      fileData
}

// Open loads the secrets file at path, creating it (with a new host id) when
// missing. A file that cannot be parsed is an error: replacing it would lose
// the host id and the ChatGPT refresh token.
func Open(path string) (*Store, error) {
	s := &Store{
		path:      path,
		envKey:    os.Getenv(EnvOpenAIKey),
		envGoogle: os.Getenv(EnvGoogleServiceAccount),
	}
	if tok := os.Getenv(EnvHomeAssistantToken); tok != "" {
		s.envHA = HomeAssistant{URL: os.Getenv(EnvHomeAssistantURL), Token: tok, FromEnv: true}
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("read secrets: %w", err)
	default:
		if err := json.Unmarshal(raw, &s.data); err != nil {
			return nil, fmt.Errorf("parse secrets %s: %w", path, err)
		}
	}
	if s.data.ChatGPT.HostID == "" {
		s.data.ChatGPT.HostID = "urn:uuid:" + uuid.NewString()
		if err := s.save(s.data); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) OpenAIKey() Value {
	if s.envKey != "" {
		return Value{Value: s.envKey, FromEnv: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return Value{Value: s.data.OpenAIKey}
}

func (s *Store) SetOpenAIKey(key string) error {
	if s.envKey != "" {
		return ErrFromEnv
	}
	return s.update(func(d *fileData) { d.OpenAIKey = key })
}

func (s *Store) ClearOpenAIKey() error { return s.SetOpenAIKey("") }

func (s *Store) GoogleServiceAccount() Value {
	if s.envGoogle != "" {
		return Value{Value: s.envGoogle, FromEnv: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return Value{Value: s.data.GoogleServiceAccount}
}

func (s *Store) SetGoogleServiceAccount(json string) error {
	if s.envGoogle != "" {
		return ErrFromEnv
	}
	return s.update(func(d *fileData) { d.GoogleServiceAccount = json })
}

func (s *Store) ClearGoogleServiceAccount() error { return s.SetGoogleServiceAccount("") }

func (s *Store) HomeAssistant() HomeAssistant {
	if s.envHA.FromEnv {
		return s.envHA
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.HomeAssistant
}

func (s *Store) SetHomeAssistant(url, token string) error {
	if s.envHA.FromEnv {
		return ErrFromEnv
	}
	return s.update(func(d *fileData) { d.HomeAssistant = HomeAssistant{URL: url, Token: token} })
}

func (s *Store) ClearHomeAssistant() error {
	if s.envHA.FromEnv {
		return ErrFromEnv
	}
	return s.update(func(d *fileData) { d.HomeAssistant = HomeAssistant{} })
}

func (s *Store) ChatGPT() ChatGPT {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.data.ChatGPT
	c.Scopes = slices.Clone(c.Scopes)
	return c
}

// UpdateChatGPT applies fn to the saved ChatGPT registration and persists the
// result atomically.
func (s *Store) UpdateChatGPT(fn func(*ChatGPT)) error {
	return s.update(func(d *fileData) {
		d.ChatGPT.Scopes = slices.Clone(d.ChatGPT.Scopes)
		fn(&d.ChatGPT)
	})
}

// update applies fn to a copy of the data and keeps it only once saved.
func (s *Store) update(fn func(*fileData)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	fn(&next)
	if err := s.save(next); err != nil {
		return err
	}
	s.data = next
	return nil
}

// save writes d to a new owner-only temp file and renames it over the
// secrets file, so a crash never leaves a partial file.
func (s *Store) save(d fileData) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".secrets-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
