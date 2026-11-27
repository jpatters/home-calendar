package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// Tool is one function the model may call. Schema is the JSON schema of the
// arguments object and must satisfy OpenAI strict mode: every property listed
// in "required" (optional ones nullable) and "additionalProperties": false.
// Call's result is sent to the model as JSON.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Call(ctx context.Context, args json.RawMessage) (any, error)
}

// Registry is the ordered set of tools offered to the model.
type Registry struct {
	tools []Tool
}

func NewRegistry(tools ...Tool) *Registry {
	return &Registry{tools: tools}
}

// toolNamespace groups every tool, as Sign in with ChatGPT requires function
// tools to be offered inside a namespace.
const toolNamespace = "calendar"

func (r *Registry) params() []responses.ToolUnionParam {
	if len(r.tools) == 0 {
		return nil
	}
	fns := make([]responses.NamespaceToolToolUnionParam, len(r.tools))
	for i, t := range r.tools {
		fns[i] = responses.NamespaceToolToolUnionParam{OfFunction: &responses.NamespaceToolToolFunctionParam{
			Name:        t.Name(),
			Description: openai.String(t.Description()),
			Parameters:  t.Schema(),
			Strict:      openai.Bool(true),
		}}
	}
	return []responses.ToolUnionParam{{OfNamespace: &responses.NamespaceToolParam{
		Name:        toolNamespace,
		Description: "Tools for the family's wall calendar: calendars, scores, weather, hot tub and pool.",
		Tools:       fns,
	}}}
}

// call runs the named tool and renders its result, or its failure, as the
// text the model receives. Failures are for the model to explain, so they are
// never returned as Go errors.
func (r *Registry) call(ctx context.Context, name, args string) string {
	for _, t := range r.tools {
		if t.Name() != name {
			continue
		}
		res, err := t.Call(ctx, json.RawMessage(args))
		if err != nil {
			return "error: " + err.Error()
		}
		b, err := json.Marshal(res)
		if err != nil {
			return fmt.Sprintf("error: %s returned an unencodable result: %v", name, err)
		}
		return string(b)
	}
	return fmt.Sprintf("error: there is no tool named %q", name)
}
