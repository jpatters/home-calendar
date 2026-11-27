package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/agent/tools"
	"github.com/jpatters/home-calendar/internal/chatgpt"
	"github.com/jpatters/home-calendar/internal/gcal"
	"github.com/jpatters/home-calendar/internal/types"
)

// maxConverseBody bounds a converse request; a spoken query is far smaller.
const maxConverseBody = 64 << 10

const agentDisabledReply = "I'm switched off right now. You can turn me on in the calendar's settings."

// newAgent builds Ziggy and the Sign in with ChatGPT client. Credentials
// come from the secrets store (where env vars override stored values) and are
// re-read by refreshAgent whenever they change, so admin edits take effect
// without a restart.
func (s *Server) newAgent(issuer string) {
	s.chatgpt = chatgpt.New(chatgpt.Options{
		Issuer:   issuer,
		Secrets:  s.secrets,
		OnChange: s.refreshAgent,
	})
	s.agent = agent.New(agent.Options{
		Token:  s.agentToken,
		Config: s.cfg.Get,
		OnState: func(snap *types.AgentSnapshot) {
			s.hub.Broadcast(Frame{Type: "agent", Agent: snap})
		},
	})
	s.refreshAgent()
}

// refreshAgent rebuilds the agent's capabilities from the current
// credentials: whether it has one, the Google Calendar client and the tools
// that depend on it. It publishes an agent frame.
func (s *Server) refreshAgent() {
	s.credMu.Lock()
	defer s.credMu.Unlock()
	deps := tools.Deps{
		Config:   s.cfg.Get,
		Events:   s.ical,
		Hockey:   s.hockey,
		Baseball: s.baseball,
		Weather:  s.weather,
		HotTub:   s.hottub,
		Pool:     s.pool,
	}
	if client := s.googleCalendarClient(); client != nil {
		deps.Calendar = client
	}
	s.agent.SetCapabilities(agent.Capabilities{
		Ready:            s.chatgpt.Status().SignedIn || s.secrets.OpenAIKey().Value != "",
		CalendarWritable: deps.Calendar != nil,
		Tools:            agent.NewRegistry(tools.All(deps)...),
	})
}

// agentToken is the bearer credential for a model request: the ChatGPT
// access token when signed in, else the OpenAI API key.
func (s *Server) agentToken(ctx context.Context) (string, error) {
	if s.chatgpt.Status().SignedIn {
		tok, err := s.chatgpt.AccessToken(ctx)
		if errors.Is(err, chatgpt.ErrSignedOut) {
			return "", fmt.Errorf("%w: %w", agent.ErrSignInAgain, err)
		}
		if err != nil {
			return "", err
		}
		if tok != "" {
			return tok, nil
		}
	}
	return s.secrets.OpenAIKey().Value, nil
}

func (s *Server) googleCalendarClient() *gcal.Client {
	value := s.secrets.GoogleServiceAccount().Value
	if strings.TrimSpace(value) == "" {
		return nil
	}
	creds, err := gcal.LoadCredentials(value)
	if err != nil {
		log.Printf("agent: calendar writes disabled: %v", err)
		return nil
	}
	client, err := gcal.New(s.rootCtx, creds)
	if err != nil {
		log.Printf("agent: calendar writes disabled: %v", err)
		return nil
	}
	return client
}

// handleGetAgent reports the agent's latest state, including whether it has
// a credential and calendar write access, without revealing any secret.
func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.agent.Snapshot())
}

// handleAgentConverse implements Home Assistant's Webhook Conversation
// contract: it reads {conversation_id, query} and answers {"output": text},
// which Home Assistant speaks. That integration treats any non-200 as a
// failure and speaks nothing, so every answerable request (including "the
// agent is off" and model failures) is a 200 whose output explains itself.
func (s *Server) handleAgentConverse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationID string `json:"conversation_id"`
		Query          string `json:"query"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConverseBody)).Decode(&body); err != nil || strings.TrimSpace(body.Query) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"output": "I didn't catch that."})
		return
	}
	if !s.cfg.Get().Agent.Enabled {
		writeJSON(w, http.StatusOK, map[string]string{"output": agentDisabledReply})
		return
	}
	reply, err := s.agent.Converse(r.Context(), body.ConversationID, strings.TrimSpace(body.Query))
	if err != nil {
		log.Printf("agent: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": reply})
}
