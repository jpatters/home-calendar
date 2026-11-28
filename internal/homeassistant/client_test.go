package homeassistant_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/homeassistant"
	"github.com/jpatters/home-calendar/internal/homeassistant/hatest"
)

// home is a small house: a few exposed devices of each kind, one device that
// isn't exposed, and the security devices Ziggy may only look at.
func home(t *testing.T) *hatest.Server {
	t.Helper()
	return hatest.New(t,
		[]hatest.Entity{
			{EntityID: "light.kitchen", State: "on", Exposed: true, AreaID: "kitchen", Aliases: []string{"kitchen lights"},
				Attributes: map[string]any{"friendly_name": "Kitchen Ceiling", "brightness": 255.0, "supported_color_modes": []any{"hs", "color_temp"}}},
			{EntityID: "light.lamp", State: "off", Exposed: true, DeviceID: "lamp-device",
				Attributes: map[string]any{"friendly_name": "Reading Lamp", "supported_color_modes": []any{"onoff"}}},
			{EntityID: "light.basement", State: "off", Exposed: false,
				Attributes: map[string]any{"friendly_name": "Basement"}},
			{EntityID: "switch.kettle", State: "off", Exposed: true, AreaID: "kitchen",
				Attributes: map[string]any{"friendly_name": "Kettle"}},
			{EntityID: "fan.bedroom", State: "on", Exposed: true, Attributes: map[string]any{"friendly_name": "Bedroom Fan", "supported_features": 1.0}},
			{EntityID: "fan.vent", State: "off", Exposed: true, Attributes: map[string]any{"friendly_name": "Bathroom Vent", "supported_features": 0.0}},
			{EntityID: "media_player.tv", State: "playing", Exposed: true,
				Attributes: map[string]any{"friendly_name": "TV", "supported_features": float64(1 | 4 | 128 | 256 | 16384)}},
			{EntityID: "media_player.radio", State: "off", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Radio", "supported_features": float64(128 | 256)}},
			{EntityID: "cover.blinds", State: "open", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Living Room Blinds", "device_class": "blind", "current_position": 100.0, "supported_features": 15.0}},
			{EntityID: "cover.curtain", State: "closed", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Curtain", "device_class": "curtain", "supported_features": 3.0}},
			{EntityID: "climate.upstairs", State: "heat", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Upstairs", "temperature": 20.0, "current_temperature": 19.5,
					"min_temp": 7.0, "max_temp": 35.0, "hvac_modes": []any{"off", "heat", "cool"}, "supported_features": float64(1 | 128 | 256)}},
			{EntityID: "scene.movie_night", State: "unknown", Exposed: true, Attributes: map[string]any{"friendly_name": "Movie Night"}},
			{EntityID: "script.goodnight", State: "off", Exposed: true, Attributes: map[string]any{"friendly_name": "Goodnight"}},
			{EntityID: "lock.front_door", State: "locked", Exposed: true, Attributes: map[string]any{"friendly_name": "Front Door"}},
			{EntityID: "alarm_control_panel.house", State: "armed_home", Exposed: true, Attributes: map[string]any{"friendly_name": "House Alarm"}},
			{EntityID: "cover.garage", State: "closed", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Garage Door", "device_class": "garage", "supported_features": 3.0}, DeviceID: "garage-opener"},
			{EntityID: "switch.garage_opener_relay", State: "off", Exposed: true, DeviceID: "garage-opener",
				Attributes: map[string]any{"friendly_name": "Opener Relay"}},
			{EntityID: "switch.garage_door_button", State: "off", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Garage Door Button"}},
			{EntityID: "cover.side_door", State: "closed", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Side Door", "device_class": "door", "supported_features": 3.0}},
			{EntityID: "cover.mystery", State: "closed", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Shed", "supported_features": 3.0}},
			{EntityID: "switch.hidden_relay", State: "off", Exposed: true, Hidden: true,
				Attributes: map[string]any{"friendly_name": "Hidden Relay"}},
			{EntityID: "cover.gate", State: "closed", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Driveway Gate", "device_class": "gate", "supported_features": 3.0}},
			{EntityID: "binary_sensor.back_door", State: "off", Exposed: true,
				Attributes: map[string]any{"friendly_name": "Back Door", "device_class": "door"}},
		},
		[]hatest.Area{{ID: "kitchen", Name: "Kitchen"}, {ID: "den", Name: "Den"}},
		[]hatest.Device{{ID: "lamp-device", AreaID: "den"}, {ID: "garage-opener"}},
	)
}

func client(s *hatest.Server) *homeassistant.Client {
	return homeassistant.New(s.URL, hatest.Token)
}

func ptr(v float64) *float64 { return &v }

func find(devices []homeassistant.Device, id string) *homeassistant.Device {
	for i := range devices {
		if devices[i].EntityID == id {
			return &devices[i]
		}
	}
	return nil
}

func mustDevices(t *testing.T, s *hatest.Server) []homeassistant.Device {
	t.Helper()
	devices, err := client(s).Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	return devices
}

func mustFind(t *testing.T, devices []homeassistant.Device, id string) homeassistant.Device {
	t.Helper()
	d := find(devices, id)
	if d == nil {
		t.Fatalf("%s isn't listed", id)
	}
	return *d
}

func sorted(list []string) []string {
	out := append([]string{}, list...)
	sort.Strings(out)
	return out
}

func TestCheckAcceptsTheTokenAndRejectsAWrongOne(t *testing.T) {
	s := home(t)
	if err := client(s).Check(context.Background()); err != nil {
		t.Errorf("Check with the right token: %v", err)
	}
	if err := homeassistant.New(s.URL, "wrong").Check(context.Background()); !errors.Is(err, homeassistant.ErrBadToken) {
		t.Errorf("Check with a wrong token err = %v, want ErrBadToken", err)
	}
}

func TestSomethingThatIsntHomeAssistantIsReportedAsSuch(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>router login</html>"))
	}))
	t.Cleanup(plain.Close)

	err := homeassistant.New(plain.URL, hatest.Token).Check(context.Background())

	if err == nil || !strings.Contains(err.Error(), "isn't Home Assistant") {
		t.Errorf("err = %v, want it to say the address isn't Home Assistant", err)
	}
}

func TestAnAddressCopiedFromTheBrowserStillConnects(t *testing.T) {
	s := home(t)
	if err := homeassistant.New(s.URL+"/lovelace/0?edit=1", hatest.Token).Check(context.Background()); err != nil {
		t.Errorf("Check with a dashboard address: %v", err)
	}
}

func TestAnUnreachableHomeAssistantSaysWhere(t *testing.T) {
	err := homeassistant.New("http://127.0.0.1:1", hatest.Token).Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("err = %v, want it to name the address it couldn't reach", err)
	}
}

func TestDevicesListsOnlyWhatIsExposedToAssist(t *testing.T) {
	devices := mustDevices(t, home(t))
	if find(devices, "light.basement") != nil {
		t.Error("an entity not exposed to Assist was listed")
	}
	if find(devices, "switch.hidden_relay") != nil {
		t.Error("a hidden entity was listed")
	}
	if find(devices, "light.kitchen") == nil || find(devices, "binary_sensor.back_door") == nil {
		t.Errorf("exposed entities missing from %v", devices)
	}
}

func TestDevicesDescribeNameRoomAliasesAndState(t *testing.T) {
	devices := mustDevices(t, home(t))

	kitchen := mustFind(t, devices, "light.kitchen")
	if kitchen.Name != "Kitchen Ceiling" || kitchen.Area != "Kitchen" || kitchen.State != "on" ||
		!reflect.DeepEqual(kitchen.Aliases, []string{"kitchen lights"}) {
		t.Errorf("kitchen light = %+v", kitchen)
	}
	if lamp := mustFind(t, devices, "light.lamp"); lamp.Area != "Den" {
		t.Errorf("lamp area = %q, want the Den from its device", lamp.Area)
	}
	if kitchen.Details["brightness_percent"] != 100.0 {
		t.Errorf("kitchen details = %v, want brightness as a percentage", kitchen.Details)
	}
	if blinds := mustFind(t, devices, "cover.blinds"); blinds.Details["position_percent"] != 100.0 {
		t.Errorf("blinds details = %v, want the position", blinds.Details)
	}
	upstairs := mustFind(t, devices, "climate.upstairs").Details
	if upstairs["target_temperature"] != 20.0 || upstairs["current_temperature"] != 19.5 ||
		upstairs["min_temperature"] != 7.0 || upstairs["max_temperature"] != 35.0 || upstairs["unit"] != "°C" ||
		!reflect.DeepEqual(upstairs["modes"], []any{"off", "heat", "cool"}) {
		t.Errorf("thermostat details = %v, want temperatures, range, unit and modes", upstairs)
	}
}

func TestDevicesOfferOnlyWhatEachOneSupports(t *testing.T) {
	devices := mustDevices(t, home(t))

	cases := map[string][]string{
		"light.kitchen":           {"turn_on", "turn_off", "toggle", "set_brightness", "set_color", "set_color_temperature"},
		"light.lamp":              {"turn_on", "turn_off", "toggle"},
		"switch.kettle":           {"turn_on", "turn_off", "toggle"},
		"fan.bedroom":             {"turn_on", "turn_off", "toggle", "set_speed"},
		"fan.vent":                {"turn_on", "turn_off", "toggle"},
		"media_player.tv":         {"turn_on", "turn_off", "play", "pause", "set_volume"},
		"media_player.radio":      {"turn_on", "turn_off"},
		"cover.blinds":            {"open", "close", "stop", "set_position"},
		"cover.curtain":           {"open", "close"},
		"climate.upstairs":        {"turn_on", "turn_off", "set_temperature", "set_mode"},
		"scene.movie_night":       {"activate"},
		"script.goodnight":        {"run"},
		"binary_sensor.back_door": {},
	}
	for id, want := range cases {
		if got := mustFind(t, devices, id).Actions; !reflect.DeepEqual(sorted(got), sorted(want)) {
			t.Errorf("%s actions = %v, want %v", id, got, want)
		}
	}
}

func TestADeviceIsNeverAskedForSomethingItCantDo(t *testing.T) {
	cases := []struct {
		entity, action string
	}{
		{"light.lamp", "set_brightness"},
		{"cover.curtain", "set_position"},
		{"media_player.radio", "set_volume"},
		{"fan.vent", "set_speed"},
	}
	for _, tc := range cases {
		t.Run(tc.entity, func(t *testing.T) {
			s := home(t)
			_, err := client(s).Control(context.Background(), tc.entity, tc.action, ptr(50), "")
			if err == nil || !strings.Contains(err.Error(), "can't") || len(s.Calls()) != 0 {
				t.Errorf("err = %v, calls = %+v, want a refusal without calling Home Assistant", err, s.Calls())
			}
		})
	}
}

// A real home's device list is far bigger than the WebSocket library's
// default message limit.
func TestAHomeWithHundredsOfDevicesIsListed(t *testing.T) {
	var entities []hatest.Entity
	for i := 0; i < 600; i++ {
		entities = append(entities, hatest.Entity{
			EntityID: fmt.Sprintf("light.room_%03d", i), State: "off", Exposed: true,
			Attributes: map[string]any{"friendly_name": fmt.Sprintf("Room %d ceiling light with a fairly long name", i),
				"supported_color_modes": []any{"brightness"}, "icon": strings.Repeat("x", 100)},
		})
	}
	s := hatest.New(t, entities, nil, nil)

	if got := mustDevices(t, s); len(got) != 600 {
		t.Errorf("listed %d devices, want 600", len(got))
	}
}

func TestSecurityDevicesAreLookOnly(t *testing.T) {
	devices := mustDevices(t, home(t))

	for _, id := range []string{
		"lock.front_door", "alarm_control_panel.house", "cover.garage", "cover.gate",
		// The relay behind the garage door, a switch named for one, a door
		// cover, and a cover that doesn't say what it is.
		"switch.garage_opener_relay", "switch.garage_door_button", "cover.side_door", "cover.mystery",
	} {
		d := mustFind(t, devices, id)
		if !d.LookOnly || len(d.Actions) != 0 || d.State == "" {
			t.Errorf("%s = %+v, want listed with its state, look-only and no actions", id, d)
		}
	}
}

func TestControlSendsTheRightServiceCall(t *testing.T) {
	cases := []struct {
		entity, action string
		value          *float64
		option         string
		want           hatest.Call
	}{
		{"light.kitchen", "turn_off", nil, "", hatest.Call{Domain: "light", Service: "turn_off", EntityID: "light.kitchen"}},
		{"light.kitchen", "set_brightness", ptr(30), "", hatest.Call{Domain: "light", Service: "turn_on", EntityID: "light.kitchen", Data: map[string]any{"brightness_pct": 30.0}}},
		{"light.kitchen", "set_color", nil, "red", hatest.Call{Domain: "light", Service: "turn_on", EntityID: "light.kitchen", Data: map[string]any{"color_name": "red"}}},
		{"light.kitchen", "set_color_temperature", ptr(2700), "", hatest.Call{Domain: "light", Service: "turn_on", EntityID: "light.kitchen", Data: map[string]any{"color_temp_kelvin": 2700.0}}},
		{"switch.kettle", "turn_on", nil, "", hatest.Call{Domain: "switch", Service: "turn_on", EntityID: "switch.kettle"}},
		{"fan.bedroom", "set_speed", ptr(50), "", hatest.Call{Domain: "fan", Service: "set_percentage", EntityID: "fan.bedroom", Data: map[string]any{"percentage": 50.0}}},
		{"media_player.tv", "pause", nil, "", hatest.Call{Domain: "media_player", Service: "media_pause", EntityID: "media_player.tv"}},
		{"media_player.tv", "set_volume", ptr(40), "", hatest.Call{Domain: "media_player", Service: "volume_set", EntityID: "media_player.tv", Data: map[string]any{"volume_level": 0.4}}},
		{"cover.blinds", "close", nil, "", hatest.Call{Domain: "cover", Service: "close_cover", EntityID: "cover.blinds"}},
		{"cover.blinds", "set_position", ptr(25), "", hatest.Call{Domain: "cover", Service: "set_cover_position", EntityID: "cover.blinds", Data: map[string]any{"position": 25.0}}},
		{"climate.upstairs", "set_temperature", ptr(21.5), "", hatest.Call{Domain: "climate", Service: "set_temperature", EntityID: "climate.upstairs", Data: map[string]any{"temperature": 21.5}}},
		{"climate.upstairs", "set_mode", nil, "cool", hatest.Call{Domain: "climate", Service: "set_hvac_mode", EntityID: "climate.upstairs", Data: map[string]any{"hvac_mode": "cool"}}},
		{"scene.movie_night", "activate", nil, "", hatest.Call{Domain: "scene", Service: "turn_on", EntityID: "scene.movie_night"}},
		{"script.goodnight", "run", nil, "", hatest.Call{Domain: "script", Service: "turn_on", EntityID: "script.goodnight"}},
	}
	for _, tc := range cases {
		t.Run(tc.entity+" "+tc.action, func(t *testing.T) {
			s := home(t)

			res, err := client(s).Control(context.Background(), tc.entity, tc.action, tc.value, tc.option)
			if err != nil {
				t.Fatalf("Control: %v", err)
			}
			calls := s.Calls()
			if len(calls) != 1 {
				t.Fatalf("calls = %+v, want one", calls)
			}
			got := calls[0]
			if got.Domain != tc.want.Domain || got.Service != tc.want.Service || got.EntityID != tc.want.EntityID ||
				(len(got.Data) > 0 || len(tc.want.Data) > 0) && !reflect.DeepEqual(got.Data, tc.want.Data) {
				t.Errorf("call = %+v, want %+v", got, tc.want)
			}
			if res.Name == "" || res.Action != tc.action {
				t.Errorf("result = %+v, want the device's name and the action done", res)
			}
		})
	}
}

func TestControlRefusesWithoutCallingHomeAssistant(t *testing.T) {
	cases := []struct {
		name, entity, action string
		value                *float64
		option               string
		says                 string
	}{
		{"an entity that isn't exposed", "light.basement", "turn_on", nil, "", "isn't exposed"},
		{"an unknown entity", "light.nowhere", "turn_on", nil, "", "isn't exposed"},
		{"unlocking a lock", "lock.front_door", "unlock", nil, "", "look-only"},
		{"disarming the alarm", "alarm_control_panel.house", "disarm", nil, "", "look-only"},
		{"opening the garage", "cover.garage", "open", nil, "", "look-only"},
		{"opening the gate", "cover.gate", "open", nil, "", "look-only"},
		{"pulsing the garage door's relay", "switch.garage_opener_relay", "turn_on", nil, "", "look-only"},
		{"a switch named for the garage door", "switch.garage_door_button", "toggle", nil, "", "look-only"},
		{"opening a door cover", "cover.side_door", "open", nil, "", "look-only"},
		{"opening a cover of unknown kind", "cover.mystery", "open", nil, "", "look-only"},
		{"a hidden entity", "switch.hidden_relay", "turn_on", nil, "", "isn't exposed"},
		{"a volume over 100", "media_player.tv", "set_volume", ptr(140), "", "between 0 and 100"},
		{"an action the device can't do", "switch.kettle", "set_brightness", ptr(50), "", "can't"},
		{"a missing value", "light.kitchen", "set_brightness", nil, "", "needs a number"},
		{"a brightness over 100", "light.kitchen", "set_brightness", ptr(150), "", "between 0 and 100"},
		{"a missing option", "climate.upstairs", "set_mode", nil, "", "needs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := home(t)

			_, err := client(s).Control(context.Background(), tc.entity, tc.action, tc.value, tc.option)

			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("err = %v, want it to say %q", err, tc.says)
			}
			if calls := s.Calls(); len(calls) != 0 {
				t.Errorf("Home Assistant was called: %+v", calls)
			}
		})
	}
}

func TestHomeAssistantErrorsComeBackAsItsMessage(t *testing.T) {
	s := home(t)
	s.FailServices("home_assistant_error", "Kitchen Ceiling is unavailable")

	_, err := client(s).Control(context.Background(), "light.kitchen", "turn_on", nil, "")

	if err == nil || !strings.Contains(err.Error(), "Kitchen Ceiling is unavailable") {
		t.Errorf("err = %v, want Home Assistant's message", err)
	}
}

// A service call that outlasts the deadline may still be carried out, so it
// mustn't be reported as a plain failure (Ziggy might try a toggle again).
func TestASlowServiceCallIsReportedAsPossiblyDone(t *testing.T) {
	s := home(t)
	s.DelayServices(2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := client(s).Control(ctx, "light.kitchen", "toggle", nil, "")

	if err == nil || !strings.Contains(err.Error(), "may still") {
		t.Errorf("err = %v, want it to say the change may still happen", err)
	}
}
