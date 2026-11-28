package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jpatters/home-calendar/internal/gcal"
	"github.com/jpatters/home-calendar/internal/secrets"
)

// AuthStatus is GET /api/agent/auth: which credential the assistant uses and
// what is configured, without any secret values.
type AuthStatus struct {
	Using   string        `json:"using"` // "chatgpt", "apiKey" or "none"
	ChatGPT ChatGPTStatus `json:"chatgpt"`
	APIKey  SecretStatus  `json:"apiKey"`
	Google  GoogleStatus  `json:"google"`
	// Warning is set only on a sign-out whose remote revocation could not
	// be confirmed.
	Warning string `json:"warning,omitempty"`
}

type ChatGPTStatus struct {
	SignedIn bool   `json:"signedIn"`
	Email    string `json:"email"`
}

type SecretStatus struct {
	Set     bool `json:"set"`
	FromEnv bool `json:"fromEnv"`
}

type GoogleStatus struct {
	Set                 bool   `json:"set"`
	FromEnv             bool   `json:"fromEnv"`
	ServiceAccountEmail string `json:"serviceAccountEmail"`
}

// maxAuthBody bounds credential bodies; a service account key is a few KiB.
const maxAuthBody = 64 << 10

const revocationWarning = "Signed out here, but OpenAI didn't confirm the sign-in was revoked. You can disconnect Home Calendar in ChatGPT's settings."

func (s *Server) authStatus() AuthStatus {
	cg := s.chatgpt.Status()
	key := s.secrets.OpenAIKey()
	google := s.secrets.GoogleServiceAccount()
	st := AuthStatus{
		Using:   "none",
		ChatGPT: ChatGPTStatus{SignedIn: cg.SignedIn, Email: cg.Email},
		APIKey:  SecretStatus{Set: key.Value != "", FromEnv: key.FromEnv},
		Google:  GoogleStatus{Set: google.Value != "", FromEnv: google.FromEnv},
	}
	switch {
	case cg.SignedIn:
		st.Using = "chatgpt"
	case st.APIKey.Set:
		st.Using = "apiKey"
	}
	if st.Google.Set {
		if creds, err := gcal.LoadCredentials(google.Value); err == nil {
			st.Google.ServiceAccountEmail, _ = gcal.ServiceAccountEmail(creds)
		}
	}
	return st
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeAuthBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBody)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "The request body isn't valid JSON.")
		return false
	}
	return true
}

// credentialChanged applies a saved credential change and answers with the
// new status.
func (s *Server) credentialChanged(w http.ResponseWriter, err error, envVar string) {
	switch {
	case errors.Is(err, secrets.ErrFromEnv):
		writeError(w, http.StatusConflict, fmt.Sprintf("This is set by the %s environment variable, so it can't be changed here.", envVar))
		return
	case err != nil:
		log.Printf("agent: save credentials: %v", err)
		writeError(w, http.StatusInternalServerError, "The credentials couldn't be saved.")
		return
	}
	s.refreshAgent()
	writeJSON(w, http.StatusOK, s.authStatus())
}

func (s *Server) handleGetAgentAuth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.authStatus())
}

func (s *Server) handleChatGPTStart(w http.ResponseWriter, r *http.Request) {
	u, err := s.chatgpt.Start()
	if err != nil {
		log.Printf("agent: start ChatGPT sign-in: %v", err)
		writeError(w, http.StatusInternalServerError, "ChatGPT sign-in couldn't be started.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorizeUrl": u})
}

func (s *Server) handleChatGPTComplete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.RedirectURL) == "" {
		writeError(w, http.StatusBadRequest, "Paste the address the sign-in page sent you to.")
		return
	}
	if err := s.chatgpt.Complete(r.Context(), body.RedirectURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.authStatus())
}

func (s *Server) handleChatGPTSignOut(w http.ResponseWriter, r *http.Request) {
	err := s.chatgpt.SignOut(r.Context())
	st := s.authStatus()
	if err != nil {
		log.Printf("agent: ChatGPT sign-out: %v", err)
		if st.ChatGPT.SignedIn {
			writeError(w, http.StatusInternalServerError, "Signing out of ChatGPT failed.")
			return
		}
		st.Warning = revocationWarning
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handlePutAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	key := strings.TrimSpace(body.Key)
	if key == "" {
		writeError(w, http.StatusBadRequest, "Enter an OpenAI API key.")
		return
	}
	s.credentialChanged(w, s.secrets.SetOpenAIKey(key), secrets.EnvOpenAIKey)
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	s.credentialChanged(w, s.secrets.ClearOpenAIKey(), secrets.EnvOpenAIKey)
}

func (s *Server) handlePutGoogle(w http.ResponseWriter, r *http.Request) {
	if s.secrets.GoogleServiceAccount().FromEnv {
		s.credentialChanged(w, secrets.ErrFromEnv, secrets.EnvGoogleServiceAccount)
		return
	}
	var body struct {
		JSON string `json:"json"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	creds := strings.TrimSpace(body.JSON)
	if _, err := gcal.ServiceAccountEmail([]byte(creds)); err != nil {
		msg := err.Error()
		writeError(w, http.StatusBadRequest, strings.ToUpper(msg[:1])+msg[1:]+".")
		return
	}
	s.credentialChanged(w, s.secrets.SetGoogleServiceAccount(creds), secrets.EnvGoogleServiceAccount)
}

func (s *Server) handleDeleteGoogle(w http.ResponseWriter, r *http.Request) {
	s.credentialChanged(w, s.secrets.ClearGoogleServiceAccount(), secrets.EnvGoogleServiceAccount)
}
