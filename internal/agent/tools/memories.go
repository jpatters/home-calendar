package tools

import (
	"context"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/memories"
)

// MemoryStore is satisfied by *memories.Store.
type MemoryStore interface {
	Add(text string) (memories.Memory, error)
	Update(id, text string) (memories.Memory, error)
	Delete(id string) error
}

type rememberArgs struct {
	Text string `json:"text"`
}

type updateMemoryArgs struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type forgetArgs struct {
	ID string `json:"id"`
}

type memoryResult struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func remember(d Deps) agent.Tool {
	return tool[rememberArgs]{
		name:        "remember",
		description: "Save something lasting about the household to remember in future conversations, as one short self-contained sentence that names who it's about.",
		schema: object(map[string]any{
			"text": prop("string", "The fact to remember, at most 300 characters."),
		}),
		fn: func(ctx context.Context, a rememberArgs) (any, error) {
			m, err := d.Memories.Add(a.Text)
			if err != nil {
				return nil, err
			}
			return memoryResult{ID: m.ID, Text: m.Text}, nil
		},
	}
}

func updateMemory(d Deps) agent.Tool {
	return tool[updateMemoryArgs]{
		name:        "update_memory",
		description: "Replace the text of something you remember, by its id, when it has changed or was wrong.",
		schema: object(map[string]any{
			"id":   prop("string", "The memory's id."),
			"text": prop("string", "The new text, at most 300 characters."),
		}),
		fn: func(ctx context.Context, a updateMemoryArgs) (any, error) {
			m, err := d.Memories.Update(a.ID, a.Text)
			if err != nil {
				return nil, err
			}
			return memoryResult{ID: m.ID, Text: m.Text}, nil
		},
	}
}

func forget(d Deps) agent.Tool {
	return tool[forgetArgs]{
		name:        "forget",
		description: "Forget something you remember, by its id.",
		schema: object(map[string]any{
			"id": prop("string", "The memory's id."),
		}),
		fn: func(ctx context.Context, a forgetArgs) (any, error) {
			if err := d.Memories.Delete(a.ID); err != nil {
				return nil, err
			}
			return map[string]bool{"forgotten": true}, nil
		},
	}
}
