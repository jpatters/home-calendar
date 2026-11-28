// Package homeassistant lets Ziggy see and control the Home Assistant devices
// the household has exposed to Assist, over Home Assistant's WebSocket API.
// Security devices (locks, alarms, garage doors, gates, doors, and anything
// sharing hardware with them) are look-only.
package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// callTimeout bounds one call (connect, sign in and every command), well
// inside Ziggy's per-turn deadline.
const callTimeout = 8 * time.Second

// ErrBadToken means Home Assistant rejected the access token.
var ErrBadToken = errors.New("the Home Assistant token was rejected")

var errTimeout = errors.New("Home Assistant took too long to answer")

// Device is an entity exposed to Assist, as Ziggy sees it.
type Device struct {
	EntityID string   `json:"entity_id"`
	Name     string   `json:"name"`
	Area     string   `json:"area,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	State    string   `json:"state"`
	// Details are readings worth saying, such as brightness_percent,
	// position_percent and a thermostat's temperatures, range, unit and modes.
	Details map[string]any `json:"details,omitempty"`
	// Actions are what control_device can do to it.
	Actions []string `json:"actions"`
	// LookOnly marks security devices Ziggy may report on but never change.
	LookOnly bool `json:"look_only,omitempty"`
}

// Result is a completed control.
type Result struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name"`
	Action   string `json:"action"`
}

// Client talks to one Home Assistant instance. It connects afresh for each
// call, so it recovers from Home Assistant restarts on its own.
type Client struct {
	wsURL string
	host  string
	token string
}

// New returns a client for the instance at baseURL using a long-lived access
// token. Only the scheme and host are used, so an address copied from a
// dashboard page (http://ha:8123/lovelace/0) works too.
func New(baseURL, token string) *Client {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		u = &url.URL{}
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	ws := url.URL{Scheme: scheme, Host: u.Host, Path: "/api/websocket"}
	return &Client{wsURL: ws.String(), host: u.Host, token: token}
}

// Check connects and signs in.
func (c *Client) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	conn, err := c.connect(ctx)
	if err != nil {
		return err
	}
	conn.close()
	return nil
}

// Devices lists every entity exposed to Assist with its room, aliases, state
// and what can be done to it.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	conn, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.close()
	h, err := conn.load(ctx)
	if err != nil {
		return nil, err
	}
	return h.devices, nil
}

// Control does action to the exposed entity entityID. value carries a number
// (brightness, speed, volume or position as a percentage; colour temperature
// in kelvin; temperature in the thermostat's unit) and option a name (a
// colour or an HVAC mode). It acts only on what Devices would list, with the
// same look-only rules and actions.
func (c *Client) Control(ctx context.Context, entityID, action string, value *float64, option string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	conn, err := c.connect(ctx)
	if err != nil {
		return Result{}, err
	}
	defer conn.close()
	h, err := conn.load(ctx)
	if err != nil {
		return Result{}, err
	}
	i := slices.IndexFunc(h.devices, func(d Device) bool { return d.EntityID == entityID })
	if i < 0 {
		return Result{}, fmt.Errorf("%s isn't exposed to Assist in Home Assistant, so I can't use it", entityID)
	}
	d := h.devices[i]
	if d.LookOnly {
		return Result{}, fmt.Errorf("%s is look-only: I can say how it is, but locks, alarms, doors, garage doors and gates aren't mine to change", d.Name)
	}
	if !slices.Contains(d.Actions, action) {
		if len(d.Actions) == 0 {
			return Result{}, fmt.Errorf("%s can't be changed, only read", d.Name)
		}
		return Result{}, fmt.Errorf("%s can't %s; it can %s", d.Name, strings.ReplaceAll(action, "_", " "), strings.Join(d.Actions, ", "))
	}
	st := h.states[entityID]
	spec := actionsFor(st, h.tempUnit)[action]
	data, err := spec.data(value, option)
	if err != nil {
		return Result{}, fmt.Errorf("%s for %s %w", strings.ReplaceAll(action, "_", " "), d.Name, err)
	}
	call := map[string]any{
		"type":    "call_service",
		"domain":  st.domain(),
		"service": spec.service,
		"target":  map[string]any{"entity_id": entityID},
	}
	if len(data) > 0 {
		call["service_data"] = data
	}
	if err := conn.command(ctx, call, nil); err != nil {
		if errors.Is(err, errTimeout) {
			return Result{}, fmt.Errorf("Home Assistant didn't confirm %s in time; it may still happen, so check before trying again", d.Name)
		}
		return Result{}, err
	}
	return Result{EntityID: entityID, Name: d.Name, Action: action}, nil
}

// home is everything one call loaded: the exposed devices, every entity's
// state and the temperature unit.
type home struct {
	devices  []Device
	states   map[string]state
	tempUnit string
}

type state struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

func (s state) domain() string {
	d, _, _ := strings.Cut(s.EntityID, ".")
	return d
}

func (s state) attr(key string) string {
	v, _ := s.Attributes[key].(string)
	return v
}

func (s state) num(key string) (float64, bool) {
	v, ok := s.Attributes[key].(float64)
	return v, ok
}

func (s state) features() int {
	f, _ := s.num("supported_features")
	return int(f)
}

func (s state) strings(key string) []string {
	list, _ := s.Attributes[key].([]any)
	var out []string
	for _, v := range list {
		if str, ok := v.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

type registryEntry struct {
	EntityID string   `json:"entity_id"`
	Name     string   `json:"name"`
	AreaID   string   `json:"area_id"`
	DeviceID string   `json:"device_id"`
	HiddenBy *string  `json:"hidden_by"`
	Aliases  []string `json:"aliases"`
	Options  struct {
		Conversation struct {
			ShouldExpose bool `json:"should_expose"`
		} `json:"conversation"`
	} `json:"options"`
}

// exposed follows Home Assistant's own rule: exposed to Assist and not
// hidden (a hidden switch is usually the relay behind another entity).
func (e registryEntry) exposed() bool {
	return e.Options.Conversation.ShouldExpose && e.HiddenBy == nil
}

var securityWords = regexp.MustCompile(`\b(garage|gate|gates|lock|locks|deadbolt)\b`)

// securityDevice reports entities that open or secure the house: locks,
// alarms, door-like covers (and covers that don't say what they are), and
// switches named for a garage, gate or lock, which are usually relays.
func securityDevice(s state) bool {
	switch s.domain() {
	case "lock", "alarm_control_panel":
		return true
	case "cover":
		switch s.attr("device_class") {
		case "garage", "gate", "door", "":
			return true
		}
	case "switch", "input_boolean", "button", "input_button":
		words := strings.ToLower(strings.ReplaceAll(s.EntityID+" "+s.attr("friendly_name"), "_", " "))
		return securityWords.MatchString(words)
	}
	return false
}

func displayName(s state, e registryEntry) string {
	if n := s.attr("friendly_name"); n != "" {
		return n
	}
	if e.Name != "" {
		return e.Name
	}
	return s.EntityID
}

func details(s state, tempUnit string) map[string]any {
	out := map[string]any{}
	if b, ok := s.num("brightness"); ok {
		out["brightness_percent"] = math.Round(b / 255 * 100)
	}
	if p, ok := s.num("current_position"); ok {
		out["position_percent"] = p
	}
	if p, ok := s.num("percentage"); ok {
		out["speed_percent"] = p
	}
	if v, ok := s.num("volume_level"); ok {
		out["volume_percent"] = math.Round(v * 100)
	}
	if s.domain() == "climate" {
		if t, ok := s.num("temperature"); ok {
			out["target_temperature"] = t
		}
		if t, ok := s.num("min_temp"); ok {
			out["min_temperature"] = t
		}
		if t, ok := s.num("max_temp"); ok {
			out["max_temperature"] = t
		}
		if modes, ok := s.Attributes["hvac_modes"].([]any); ok {
			out["modes"] = modes
		}
		if tempUnit != "" {
			out["unit"] = tempUnit
		}
	}
	if t, ok := s.num("current_temperature"); ok {
		out["current_temperature"] = t
	}
	if u := s.attr("unit_of_measurement"); u != "" {
		out["unit"] = u
	}
	if dc := s.attr("device_class"); dc != "" {
		out["kind"] = dc
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// actionSpec is one control_device action: the service it calls and how its
// value or option becomes service data.
type actionSpec struct {
	service string
	data    func(value *float64, option string) (map[string]any, error)
}

func plain(service string) actionSpec {
	return actionSpec{service: service, data: func(*float64, string) (map[string]any, error) { return nil, nil }}
}

func numberAction(service, field string, lo, hi float64, scale float64) actionSpec {
	return actionSpec{service: service, data: func(v *float64, _ string) (map[string]any, error) {
		if v == nil {
			return nil, errors.New("needs a number")
		}
		if *v < lo || *v > hi {
			return nil, fmt.Errorf("needs a number between %v and %v", lo, hi)
		}
		return map[string]any{field: *v * scale}, nil
	}}
}

func optionAction(service, field, what string) actionSpec {
	return actionSpec{service: service, data: func(_ *float64, o string) (map[string]any, error) {
		if strings.TrimSpace(o) == "" {
			return nil, fmt.Errorf("needs %s", what)
		}
		return map[string]any{field: strings.TrimSpace(o)}, nil
	}}
}

// actionOrder is the order actions are offered in.
var actionOrder = []string{
	"turn_on", "turn_off", "toggle", "open", "close", "stop", "play", "pause",
	"set_brightness", "set_color", "set_color_temperature", "set_speed", "set_volume",
	"set_position", "set_temperature", "set_mode", "activate", "run",
}

// Supported-feature bits, from Home Assistant's entity feature enums.
const (
	fanSetSpeed       = 1
	mediaPause        = 1
	mediaVolumeSet    = 4
	mediaTurnOn       = 128
	mediaTurnOff      = 256
	mediaPlay         = 16384
	coverOpen         = 1
	coverClose        = 2
	coverSetPosition  = 4
	coverStop         = 8
	climateTargetTemp = 1
	climateTurnOff    = 128
	climateTurnOn     = 256
)

// actionsFor is what the entity can do, given the features it reports. It
// doesn't apply the look-only rule; load does.
func actionsFor(s state, tempUnit string) map[string]actionSpec {
	f := s.features()
	has := func(bit int) bool { return f&bit != 0 }
	out := map[string]actionSpec{}
	add := func(ok bool, name string, spec actionSpec) {
		if ok {
			out[name] = spec
		}
	}
	onOff := func() {
		add(true, "turn_on", plain("turn_on"))
		add(true, "turn_off", plain("turn_off"))
		add(true, "toggle", plain("toggle"))
	}
	switch s.domain() {
	case "light":
		onOff()
		modes := s.strings("supported_color_modes")
		dimmable := slices.ContainsFunc(modes, func(m string) bool { return m != "onoff" })
		colour := slices.ContainsFunc(modes, func(m string) bool {
			return m == "hs" || m == "xy" || m == "rgb" || m == "rgbw" || m == "rgbww"
		})
		add(dimmable, "set_brightness", numberAction("turn_on", "brightness_pct", 0, 100, 1))
		add(colour, "set_color", optionAction("turn_on", "color_name", "a colour name"))
		add(slices.Contains(modes, "color_temp"), "set_color_temperature", numberAction("turn_on", "color_temp_kelvin", 1500, 9000, 1))
	case "switch", "input_boolean":
		onOff()
	case "fan":
		onOff()
		add(has(fanSetSpeed), "set_speed", numberAction("set_percentage", "percentage", 0, 100, 1))
	case "media_player":
		add(has(mediaTurnOn), "turn_on", plain("turn_on"))
		add(has(mediaTurnOff), "turn_off", plain("turn_off"))
		add(has(mediaPlay), "play", plain("media_play"))
		add(has(mediaPause), "pause", plain("media_pause"))
		add(has(mediaVolumeSet), "set_volume", numberAction("volume_set", "volume_level", 0, 100, 0.01))
	case "cover":
		add(has(coverOpen), "open", plain("open_cover"))
		add(has(coverClose), "close", plain("close_cover"))
		add(has(coverStop), "stop", plain("stop_cover"))
		add(has(coverSetPosition), "set_position", numberAction("set_cover_position", "position", 0, 100, 1))
	case "climate":
		add(has(climateTurnOn), "turn_on", plain("turn_on"))
		add(has(climateTurnOff), "turn_off", plain("turn_off"))
		lo, okLo := s.num("min_temp")
		hi, okHi := s.num("max_temp")
		if !okLo || !okHi {
			lo, hi = -50, 120
		}
		add(has(climateTargetTemp), "set_temperature", numberAction("set_temperature", "temperature", lo, hi, 1))
		if modes := s.strings("hvac_modes"); len(modes) > 0 {
			add(true, "set_mode", optionAction("set_hvac_mode", "hvac_mode", "a mode: "+strings.Join(modes, ", ")))
		}
	case "scene":
		add(true, "activate", plain("turn_on"))
	case "script":
		add(true, "run", plain("turn_on"))
	}
	return out
}

func actionNames(specs map[string]actionSpec) []string {
	names := []string{}
	for _, a := range actionOrder {
		if _, ok := specs[a]; ok {
			names = append(names, a)
		}
	}
	return names
}

// conn is one signed-in WebSocket connection.
type conn struct {
	ws     *websocket.Conn
	nextID int
}

func (c *Client) connect(ctx context.Context) (*conn, error) {
	ws, resp, err := websocket.Dial(ctx, c.wsURL, nil)
	if err != nil {
		log.Printf("homeassistant: connect %s: %v", c.wsURL, err)
		if resp != nil {
			return nil, fmt.Errorf("something answered at %s, but it isn't Home Assistant", c.host)
		}
		return nil, fmt.Errorf("couldn't reach Home Assistant at %s", c.host)
	}
	ws.SetReadLimit(32 << 20)
	var hello struct {
		Type string `json:"type"`
	}
	if err := wsjson.Read(ctx, ws, &hello); err != nil || hello.Type != "auth_required" {
		ws.CloseNow()
		return nil, fmt.Errorf("something answered at %s, but it isn't Home Assistant", c.host)
	}
	if err := wsjson.Write(ctx, ws, map[string]string{"type": "auth", "access_token": c.token}); err != nil {
		ws.CloseNow()
		return nil, fmt.Errorf("couldn't reach Home Assistant at %s", c.host)
	}
	var reply struct {
		Type string `json:"type"`
	}
	if err := wsjson.Read(ctx, ws, &reply); err != nil || reply.Type != "auth_ok" {
		ws.CloseNow()
		if err == nil && reply.Type == "auth_invalid" {
			return nil, ErrBadToken
		}
		return nil, fmt.Errorf("Home Assistant at %s didn't let me sign in", c.host)
	}
	return &conn{ws: ws, nextID: 1}, nil
}

// close drops the connection without waiting on Home Assistant, so a call
// never runs past its deadline.
func (c *conn) close() { c.ws.CloseNow() }

// command sends msg with the next id and decodes its result into out
// (ignored when nil). Home Assistant's own error message is returned as the
// error; running out of time is errTimeout.
func (c *conn) command(ctx context.Context, msg map[string]any, out any) error {
	id := c.nextID
	c.nextID++
	msg["id"] = id
	if err := wsjson.Write(ctx, c.ws, msg); err != nil {
		if ctx.Err() != nil {
			return errTimeout
		}
		return errors.New("lost the connection to Home Assistant")
	}
	for {
		var reply struct {
			ID      int             `json:"id"`
			Type    string          `json:"type"`
			Success bool            `json:"success"`
			Result  json.RawMessage `json:"result"`
			Error   struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := wsjson.Read(ctx, c.ws, &reply); err != nil {
			if ctx.Err() != nil {
				return errTimeout
			}
			return errors.New("lost the connection to Home Assistant")
		}
		if reply.ID != id || reply.Type != "result" {
			continue
		}
		if !reply.Success {
			if reply.Error.Message != "" {
				return fmt.Errorf("Home Assistant said: %s", reply.Error.Message)
			}
			return fmt.Errorf("Home Assistant refused (%s)", reply.Error.Code)
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(reply.Result, out); err != nil {
			return fmt.Errorf("couldn't read Home Assistant's answer to %v", msg["type"])
		}
		return nil
	}
}

// load reads the registries, states and unit system and works out what is
// exposed, what each exposed entity can do and what is look-only. An entity
// is look-only when it is a security device or shares a device with one,
// whether or not that one is exposed.
func (c *conn) load(ctx context.Context) (home, error) {
	var registry []registryEntry
	if err := c.command(ctx, map[string]any{"type": "config/entity_registry/list"}, &registry); err != nil {
		return home{}, err
	}
	var stateList []state
	if err := c.command(ctx, map[string]any{"type": "get_states"}, &stateList); err != nil {
		return home{}, err
	}
	var config struct {
		UnitSystem struct {
			Temperature string `json:"temperature"`
		} `json:"unit_system"`
	}
	if err := c.command(ctx, map[string]any{"type": "get_config"}, &config); err != nil {
		return home{}, err
	}
	h := home{states: map[string]state{}, tempUnit: config.UnitSystem.Temperature}
	for _, s := range stateList {
		h.states[s.EntityID] = s
	}
	secureDevices := map[string]bool{}
	exposed := map[string]registryEntry{}
	var ids []string
	for _, e := range registry {
		if s, ok := h.states[e.EntityID]; ok && e.DeviceID != "" && securityDevice(s) {
			secureDevices[e.DeviceID] = true
		}
		if e.exposed() {
			exposed[e.EntityID] = e
			ids = append(ids, e.EntityID)
		}
	}
	h.devices = []Device{}
	if len(ids) == 0 {
		return h, nil
	}
	var extended map[string]*registryEntry
	if err := c.command(ctx, map[string]any{"type": "config/entity_registry/get_entries", "entity_ids": ids}, &extended); err != nil {
		return home{}, err
	}
	areas, deviceAreas, err := c.areas(ctx)
	if err != nil {
		return home{}, err
	}
	for _, s := range stateList {
		e, ok := exposed[s.EntityID]
		if !ok {
			continue
		}
		if ext := extended[s.EntityID]; ext != nil {
			e.Aliases = ext.Aliases
		}
		areaID := e.AreaID
		if areaID == "" {
			areaID = deviceAreas[e.DeviceID]
		}
		d := Device{
			EntityID: s.EntityID,
			Name:     displayName(s, e),
			Area:     areas[areaID],
			Aliases:  e.Aliases,
			State:    s.State,
			Details:  details(s, h.tempUnit),
			LookOnly: securityDevice(s) || (e.DeviceID != "" && secureDevices[e.DeviceID]),
		}
		d.Actions = []string{}
		if !d.LookOnly {
			d.Actions = actionNames(actionsFor(s, h.tempUnit))
		}
		h.devices = append(h.devices, d)
	}
	return h, nil
}

// areas returns area names by id, and each device's area id.
func (c *conn) areas(ctx context.Context) (map[string]string, map[string]string, error) {
	var areaList []struct {
		AreaID string `json:"area_id"`
		Name   string `json:"name"`
	}
	if err := c.command(ctx, map[string]any{"type": "config/area_registry/list"}, &areaList); err != nil {
		return nil, nil, err
	}
	var deviceList []struct {
		ID     string `json:"id"`
		AreaID string `json:"area_id"`
	}
	if err := c.command(ctx, map[string]any{"type": "config/device_registry/list"}, &deviceList); err != nil {
		return nil, nil, err
	}
	areas := map[string]string{}
	for _, a := range areaList {
		areas[a.AreaID] = a.Name
	}
	deviceAreas := map[string]string{}
	for _, d := range deviceList {
		deviceAreas[d.ID] = d.AreaID
	}
	return areas, deviceAreas, nil
}
