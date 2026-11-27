package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/agent/tools"
	"github.com/jpatters/home-calendar/internal/gcal"
	"github.com/jpatters/home-calendar/internal/types"
)

// maxConverseBody bounds a converse request; a spoken query is far smaller.
const maxConverseBody = 64 << 10

const agentDisabledReply = "I'm switched off right now. You can turn me on in the calendar's settings."

// newAgent builds Ziggy from the environment: OPENAI_API_KEY (and the SDK's
// other OPENAI_* variables) for the model, and GOOGLE_SERVICE_ACCOUNT_JSON
// (the JSON or a path to it) for calendar writes. Either may be absent; the
// agent then reports itself not ready or read-only.
func (s *Server) newAgent(ctx context.Context) *agent.Agent {
	deps := tools.Deps{
		Config:   s.cfg.Get,
		Events:   s.ical,
		Hockey:   s.hockey,
		Baseball: s.baseball,
		Weather:  s.weather,
		HotTub:   s.hottub,
		Pool:     s.pool,
	}
	if client := googleCalendarClient(ctx); client != nil {
		deps.Calendar = client
	}
	return agent.New(agent.Options{
		APIKey:           os.Getenv("OPENAI_API_KEY"),
		Config:           s.cfg.Get,
		Tools:            agent.NewRegistry(tools.All(deps)...),
		CalendarWritable: deps.Calendar != nil,
		OnState: func(snap *types.AgentSnapshot) {
			s.hub.Broadcast(Frame{Type: "agent", Agent: snap})
		},
	})
}

func googleCalendarClient(ctx context.Context) *gcal.Client {
	value := os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON")
	if strings.TrimSpace(value) == "" {
		return nil
	}
	creds, err := gcal.LoadCredentials(value)
	if err != nil {
		log.Printf("agent: calendar writes disabled: %v", err)
		return nil
	}
	client, err := gcal.New(ctx, creds)
	if err != nil {
		log.Printf("agent: calendar writes disabled: %v", err)
		return nil
	}
	return client
}

// handleGetAgent reports the agent's latest state, including whether it has
// an API key and calendar write access, without revealing either secret.
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
