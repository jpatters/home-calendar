package agent

import (
	"slices"
	"sync"
	"time"

	"github.com/openai/openai-go/v3/responses"
)

const (
	// conversationTTL is how long after its last turn a conversation can be
	// continued. Home Assistant reuses a conversation_id for follow-ups said
	// shortly after a reply; anything later is treated as a new request.
	conversationTTL = 5 * time.Minute
	// maxTurns caps how many completed turns a conversation resends.
	maxTurns = 10
)

// memory holds each conversation's completed turns locally. Requests are not
// stored by OpenAI (store:false), so every request resends the history: the
// user's words, function calls with their outputs, and the spoken replies.
type memory struct {
	mu            sync.Mutex
	conversations map[string]conversation
}

type conversation struct {
	// turns are the input items of each completed turn, oldest first.
	turns [][]responses.ResponseInputItemUnionParam
	at    time.Time
}

func newMemory() *memory {
	return &memory{conversations: map[string]conversation{}}
}

// history returns the items of the conversation's recent turns, or nothing
// when it has expired.
func (m *memory) history(conversationID string, now time.Time) []responses.ResponseInputItemUnionParam {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conversations[conversationID]
	if !ok {
		return nil
	}
	if now.Sub(c.at) > conversationTTL {
		delete(m.conversations, conversationID)
		return nil
	}
	var items []responses.ResponseInputItemUnionParam
	for _, t := range c.turns {
		items = append(items, t...)
	}
	return items
}

// remember appends a completed turn, keeping the most recent maxTurns.
func (m *memory) remember(conversationID string, turn []responses.ResponseInputItemUnionParam, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.conversations {
		if now.Sub(c.at) > conversationTTL {
			delete(m.conversations, id)
		}
	}
	c := m.conversations[conversationID]
	turns := append(slices.Clone(c.turns), turn)
	if len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}
	m.conversations[conversationID] = conversation{turns: turns, at: now}
}

func (m *memory) forget(conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conversations, conversationID)
}
