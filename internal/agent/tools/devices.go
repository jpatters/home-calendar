package tools

import (
	"context"
	"errors"

	"github.com/jpatters/home-calendar/internal/agent"
)

var (
	errHotTubOff = errors.New("the hot tub isn't set up on this calendar")
	errPoolOff   = errors.New("the pool heater isn't set up on this calendar")
)

func getHotTub(d Deps) agent.Tool {
	return tool[noArgs]{
		name:        "get_hot_tub",
		description: "Get the hot tub's water temperature, target and heater state, in Fahrenheit.",
		schema:      object(map[string]any{}),
		fn: func(ctx context.Context, _ noArgs) (any, error) {
			if !d.Config().HotTub.Enabled || d.HotTub == nil {
				return nil, errHotTubOff
			}
			if s := d.HotTub.Snapshot(); s != nil {
				return s, nil
			}
			return nil, errors.New("the hot tub isn't reporting right now")
		},
	}
}

type hotTubTargetArgs struct {
	TargetF float64 `json:"target_f"`
}

func setHotTubTarget(d Deps) agent.Tool {
	return tool[hotTubTargetArgs]{
		name:        "set_hot_tub_target",
		description: "Set the hot tub's target temperature, a whole number of degrees Fahrenheit within the tub's limits.",
		schema: object(map[string]any{
			"target_f": prop("number", "Target temperature in whole degrees Fahrenheit."),
		}),
		fn: func(ctx context.Context, a hotTubTargetArgs) (any, error) {
			cfg := d.Config().HotTub
			if !cfg.Enabled || d.HotTub == nil {
				return nil, errHotTubOff
			}
			return d.HotTub.SetTarget(ctx, cfg, a.TargetF)
		},
	}
}

func getPool(d Deps) agent.Tool {
	return tool[noArgs]{
		name:        "get_pool",
		description: "Get the pool's water temperature, target and heater state, in Celsius.",
		schema:      object(map[string]any{}),
		fn: func(ctx context.Context, _ noArgs) (any, error) {
			if !d.Config().Pool.Enabled || d.Pool == nil {
				return nil, errPoolOff
			}
			if s := d.Pool.Snapshot(); s != nil {
				return s, nil
			}
			return nil, errors.New("the pool heater isn't reporting right now")
		},
	}
}

type poolTargetArgs struct {
	TargetC float64 `json:"target_c"`
}

func setPoolTarget(d Deps) agent.Tool {
	return tool[poolTargetArgs]{
		name:        "set_pool_target",
		description: "Set the pool heater's target temperature in degrees Celsius, within the heater's limits.",
		schema: object(map[string]any{
			"target_c": prop("number", "Target temperature in degrees Celsius."),
		}),
		fn: func(ctx context.Context, a poolTargetArgs) (any, error) {
			cfg := d.Config().Pool
			if !cfg.Enabled || d.Pool == nil {
				return nil, errPoolOff
			}
			return d.Pool.SetTarget(ctx, cfg, a.TargetC)
		},
	}
}
