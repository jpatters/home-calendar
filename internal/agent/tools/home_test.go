package tools_test

import (
	"strings"
	"testing"

	"github.com/jpatters/home-calendar/internal/agent/tools"
	"github.com/jpatters/home-calendar/internal/homeassistant"
	"github.com/jpatters/home-calendar/internal/homeassistant/hatest"
)

func homeDeps(t *testing.T) (tools.Deps, *hatest.Server) {
	t.Helper()
	ha := hatest.New(t,
		[]hatest.Entity{
			{EntityID: "light.kitchen", State: "on", Exposed: true, AreaID: "galley",
				Attributes: map[string]any{"friendly_name": "Kitchen Lights", "supported_color_modes": []any{"hs"}}},
			{EntityID: "lock.front_door", State: "locked", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Front Door"}},
		},
		[]hatest.Area{{ID: "galley", Name: "Galley"}}, nil)
	d := baseDeps(t)
	d.Home = homeassistant.New(ha.URL, hatest.Token)
	return d, ha
}

func TestListDevicesShowsExposedDevicesWithTheirRoomAndState(t *testing.T) {
	d, _ := homeDeps(t)

	out := mustCall(t, d, "list_devices", `{}`)

	for _, want := range []string{"light.kitchen", "Kitchen Lights", `"area":"Galley"`, `"state":"on"`, "lock.front_door", `"look_only":true`} {
		if !strings.Contains(out, want) {
			t.Errorf("list_devices = %s, missing %s", out, want)
		}
	}
}

func TestControlDeviceChangesItAndSaysWhatWasDone(t *testing.T) {
	d, ha := homeDeps(t)

	out := mustCall(t, d, "control_device", `{"entity_id":"light.kitchen","action":"set_color","value":null,"option":"red"}`)

	calls := ha.Calls()
	if len(calls) != 1 || calls[0].Service != "turn_on" || calls[0].Data["color_name"] != "red" {
		t.Errorf("calls = %+v, want the light turned red", calls)
	}
	if !strings.Contains(out, "Kitchen Lights") || !strings.Contains(out, "set_color") {
		t.Errorf("result = %s, want the device and action", out)
	}
}

func TestDeviceToolsExplainWhenHomeAssistantIsNotConnected(t *testing.T) {
	d := baseDeps(t)
	for _, tc := range []struct{ name, args string }{
		{"list_devices", `{}`},
		{"control_device", `{"entity_id":"light.kitchen","action":"turn_on","value":null,"option":null}`},
	} {
		if err := mustFail(t, d, tc.name, tc.args); !strings.Contains(err.Error(), "Home Assistant isn't connected") {
			t.Errorf("%s err = %v, want it to say Home Assistant isn't connected", tc.name, err)
		}
	}
}
