package agent

import (
	"sync"
	"time"
)

// conversationTTL is how long after its last turn a conversation can be
// continued. Home Assistant reuses a conversation_id for follow-ups said
// shortly after a reply; anything later is treated as a new request.
const conversationTTL = 5 * time.Minute

// memory maps a conversation id to the id of its last completed response.
// Turns are chained with the Responses API's previous_response_id, so OpenAI
// holds the history (messages, tool calls, reasoning) and nothing is replayed.
type memory struct {
	mu    sync.Mutex
	turns map[string]turn
}

type turn struct {
	responseID string
	at         time.Time
}

func newMemory() *memory {
	return &memory{turns: map[string]turn{}}
}

func (m *memory) last(conversationID string, now time.Time) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.turns[conversationID]
	if !ok {
		return "", false
	}
	if now.Sub(t.at) > conversationTTL {
		delete(m.turns, conversationID)
		return "", false
	}
	return t.responseID, true
}

func (m *memory) remember(conversationID, responseID string, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.turns {
		if now.Sub(t.at) > conversationTTL {
			delete(m.turns, id)
		}
	}
	m.turns[conversationID] = turn{responseID: responseID, at: now}
}

func (m *memory) forget(conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.turns, conversationID)
}
