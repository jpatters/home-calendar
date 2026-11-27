package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/jpatters/home-calendar/internal/agent"
)

type scoresArgs struct {
	Sport string `json:"sport"`
}

func getScores(d Deps) agent.Tool {
	return tool[scoresArgs]{
		name:        "get_scores",
		description: "Get the family's team's live game, latest result and next game for a sport.",
		schema: object(map[string]any{
			"sport": map[string]any{"type": "string", "enum": []string{"hockey", "baseball"}, "description": "Which sport."},
		}),
		fn: func(ctx context.Context, a scoresArgs) (any, error) {
			switch a.Sport {
			case "hockey":
				if d.Hockey != nil {
					if s := d.Hockey.Snapshot(); s != nil {
						return s, nil
					}
				}
			case "baseball":
				if d.Baseball != nil {
					if s := d.Baseball.Snapshot(); s != nil {
						return s, nil
					}
				}
			default:
				return nil, fmt.Errorf("unknown sport %q; use hockey or baseball", a.Sport)
			}
			return nil, fmt.Errorf("%s scores aren't available: no %s team is set up, or they haven't loaded yet", a.Sport, a.Sport)
		},
	}
}

func getWeather(d Deps) agent.Tool {
	return tool[noArgs]{
		name:        "get_weather",
		description: "Get current weather conditions and the daily forecast. Temperatures are Celsius.",
		schema:      object(map[string]any{}),
		fn: func(ctx context.Context, _ noArgs) (any, error) {
			if d.Weather != nil {
				if s := d.Weather.Snapshot(); s != nil {
					return s, nil
				}
			}
			return nil, errors.New("the weather isn't available: it isn't set up, or it hasn't loaded yet")
		},
	}
}
