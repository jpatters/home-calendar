package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// tool adapts a typed handler to agent.Tool. Args is decoded from the
// model's JSON arguments before fn runs.
type tool[Args any] struct {
	name        string
	description string
	schema      map[string]any
	fn          func(ctx context.Context, args Args) (any, error)
}

func (t tool[Args]) Name() string           { return t.name }
func (t tool[Args]) Description() string    { return t.description }
func (t tool[Args]) Schema() map[string]any { return t.schema }

func (t tool[Args]) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args Args
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments for %s: %w", t.name, err)
	}
	return t.fn(ctx, args)
}

// noArgs is the argument type of tools that take none.
type noArgs struct{}

// object builds a strict-mode schema: every property is required, so
// optional ones must be declared nullable.
func object(props map[string]any) map[string]any {
	required := make([]string, 0, len(props))
	for name := range props {
		required = append(required, name)
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

func prop(typ, description string) map[string]any {
	return map[string]any{"type": typ, "description": description}
}

func nullable(typ, description string) map[string]any {
	return map[string]any{"type": []string{typ, "null"}, "description": description}
}
