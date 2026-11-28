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
			{EntityID: "light.kitchen", State: "on", Exposed: true, AreaID: "kitchen",
				Attributes: map[string]any{"friendly_name": "Kitchen Lights"}},
			{EntityID: "lock.front_door", State: "locked", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Front Door"}},
		},
		[]hatest.Area{{ID: "kitchen", Name: "Kitchen"}}, nil)
	d := baseDeps(t)
	d.Home = homeassistant.New(ha.URL, hatest.Token)
	return d, ha
}

func TestListDevicesShowsExposedDevicesWithTheirRoomAndState(t *testing.T) {
	d, _ := homeDeps(t)

	out := mustCall(t, d, "list_devices", `{}`)

	for _, want := range []string{"light.kitchen", "Kitchen Lights", "Kitchen", `"state":"on"`, "lock.front_door", `"look_only":true`} {
		if !strings.Contains(out, want) {
			t.Errorf("list_devices = %s, missing %s", out, want)
		}
	}
}

func TestControlDeviceChangesItAndSaysWhatWasDone(t *testing.T) {
	d, ha := homeDeps(t)

	out := mustCall(t, d, "control_device", `{"entity_id":"light.kitchen","action":"set_brightness","value":30,"option":null}`)

	calls := ha.Calls()
	if len(calls) != 1 || calls[0].Service != "turn_on" || calls[0].Data["brightness_pct"] != 30.0 {
		t.Errorf("calls = %+v, want the light set to 30%%", calls)
	}
	if !strings.Contains(out, "Kitchen Lights") || !strings.Contains(out, "set_brightness") {
		t.Errorf("result = %s, want the device and action", out)
	}
}

func TestControlDeviceRefusesSecurityDevices(t *testing.T) {
	d, ha := homeDeps(t)

	err := mustFail(t, d, "control_device", `{"entity_id":"lock.front_door","action":"unlock","value":null,"option":null}`)

	if !strings.Contains(err.Error(), "look-only") || len(ha.Calls()) != 0 {
		t.Errorf("err = %v, calls = %+v, want a look-only refusal and no call", err, ha.Calls())
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
