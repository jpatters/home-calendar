package tools

import (
	"context"
	"errors"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/homeassistant"
)

// HomeControl is satisfied by *homeassistant.Client.
type HomeControl interface {
	Devices(ctx context.Context) ([]homeassistant.Device, error)
	Control(ctx context.Context, entityID, action string, value *float64, option string) (homeassistant.Result, error)
}

var errHomeOff = errors.New("Home Assistant isn't connected to this calendar yet")

func listDevices(d Deps) agent.Tool {
	return tool[noArgs]{
		name:        "list_devices",
		description: "List the home's devices exposed to Home Assistant's Assist: each one's entity id, name, room, aliases, state, readings and the actions control_device can do. Use it to find a device by name or room, or to answer questions like whether a light is on or a door is locked.",
		schema:      object(map[string]any{}),
		fn: func(ctx context.Context, _ noArgs) (any, error) {
			if d.Home == nil {
				return nil, errHomeOff
			}
			return d.Home.Devices(ctx)
		},
	}
}

type controlArgs struct {
	EntityID string   `json:"entity_id"`
	Action   string   `json:"action"`
	Value    *float64 `json:"value"`
	Option   *string  `json:"option"`
}

func controlDevice(d Deps) agent.Tool {
	return tool[controlArgs]{
		name:        "control_device",
		description: "Do one of a device's actions, as listed by list_devices. set_brightness, set_speed, set_volume and set_position take value as a percentage; set_color_temperature takes kelvin; set_temperature takes the thermostat's unit. set_color takes option as a colour name and set_mode an HVAC mode such as heat, cool, auto or off. Locks, alarms, garage doors and gates are look-only.",
		schema: object(map[string]any{
			"entity_id": prop("string", "The device's entity id from list_devices."),
			"action":    prop("string", "One of the device's actions."),
			"value":     nullable("number", "The number the action needs, or null."),
			"option":    nullable("string", "The name the action needs (a colour or a mode), or null."),
		}),
		fn: func(ctx context.Context, a controlArgs) (any, error) {
			if d.Home == nil {
				return nil, errHomeOff
			}
			option := ""
			if a.Option != nil {
				option = *a.Option
			}
			return d.Home.Control(ctx, a.EntityID, a.Action, a.Value, option)
		},
	}
}
