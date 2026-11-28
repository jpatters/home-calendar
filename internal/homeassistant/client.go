// Package homeassistant lets Ziggy see and control the Home Assistant devices
// the household has exposed to Assist, over Home Assistant's WebSocket API.
// Locks, alarm panels, garage doors and gates are look-only.
package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
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

// Device is an entity exposed to Assist, as Ziggy sees it.
type Device struct {
	EntityID string   `json:"entity_id"`
	Name     string   `json:"name"`
	Area     string   `json:"area,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	State    string   `json:"state"`
	// Details are readings worth saying, such as brightness_percent,
	// position_percent, target_temperature and current_temperature.
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

// New returns a client for the instance at baseURL (e.g.
// "http://homeassistant.local:8123") using a long-lived access token.
func New(baseURL, token string) *Client {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil {
		u = &url.URL{}
	}
	host := u.Host
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path += "/api/websocket"
	return &Client{wsURL: u.String(), host: host, token: token}
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

	var registry []registryEntry
	if err := conn.command(ctx, map[string]any{"type": "config/entity_registry/list"}, &registry); err != nil {
		return nil, err
	}
	exposed := map[string]registryEntry{}
	var ids []string
	for _, e := range registry {
		if e.exposed() {
			exposed[e.EntityID] = e
			ids = append(ids, e.EntityID)
		}
	}
	if len(ids) == 0 {
		return []Device{}, nil
	}
	var extended map[string]*registryEntry
	if err := conn.command(ctx, map[string]any{"type": "config/entity_registry/get_entries", "entity_ids": ids}, &extended); err != nil {
		return nil, err
	}
	areas, deviceAreas, err := conn.areas(ctx)
	if err != nil {
		return nil, err
	}
	var states []state
	if err := conn.command(ctx, map[string]any{"type": "get_states"}, &states); err != nil {
		return nil, err
	}
	devices := []Device{}
	for _, st := range states {
		entry, ok := exposed[st.EntityID]
		if !ok {
			continue
		}
		if ext := extended[st.EntityID]; ext != nil {
			entry.Aliases = ext.Aliases
		}
		areaID := entry.AreaID
		if areaID == "" {
			areaID = deviceAreas[entry.DeviceID]
		}
		devices = append(devices, describe(st, entry, areas[areaID]))
	}
	return devices, nil
}

// Control does action to the exposed entity entityID. value carries a number
// (brightness, speed, volume or position as a percentage; colour temperature
// in kelvin; temperature in the thermostat's unit) and option a name (a
// colour or an HVAC mode).
func (c *Client) Control(ctx context.Context, entityID, action string, value *float64, option string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	conn, err := c.connect(ctx)
	if err != nil {
		return Result{}, err
	}
	defer conn.close()

	var entries map[string]*registryEntry
	if err := conn.command(ctx, map[string]any{"type": "config/entity_registry/get_entries", "entity_ids": []string{entityID}}, &entries); err != nil {
		return Result{}, err
	}
	entry := entries[entityID]
	if entry == nil || !entry.exposed() {
		return Result{}, fmt.Errorf("%s isn't exposed to Assist in Home Assistant, so I can't use it", entityID)
	}
	var states []state
	if err := conn.command(ctx, map[string]any{"type": "get_states"}, &states); err != nil {
		return Result{}, err
	}
	i := slices.IndexFunc(states, func(s state) bool { return s.EntityID == entityID })
	if i < 0 {
		return Result{}, fmt.Errorf("Home Assistant doesn't have %s right now", entityID)
	}
	st := states[i]
	name := displayName(st, *entry)
	if lookOnly(st) {
		return Result{}, fmt.Errorf("%s is look-only: I can say how it is, but locks, alarms, garage doors and gates aren't mine to change", name)
	}
	spec, ok := actionsFor(st)[action]
	if !ok {
		return Result{}, fmt.Errorf("%s can't %s; it can %s", name, strings.ReplaceAll(action, "_", " "), strings.Join(actionNames(st), ", "))
	}
	data, err := spec.data(value, option)
	if err != nil {
		return Result{}, fmt.Errorf("%s for %s %w", strings.ReplaceAll(action, "_", " "), name, err)
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
		return Result{}, err
	}
	return Result{EntityID: entityID, Name: name, Action: action}, nil
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

type registryEntry struct {
	EntityID string   `json:"entity_id"`
	Name     string   `json:"name"`
	AreaID   string   `json:"area_id"`
	DeviceID string   `json:"device_id"`
	Aliases  []string `json:"aliases"`
	Options  struct {
		Conversation struct {
			ShouldExpose bool `json:"should_expose"`
		} `json:"conversation"`
	} `json:"options"`
}

func (e registryEntry) exposed() bool { return e.Options.Conversation.ShouldExpose }

// lookOnly reports security devices Ziggy may never change.
func lookOnly(s state) bool {
	switch s.domain() {
	case "lock", "alarm_control_panel":
		return true
	case "cover":
		dc := s.attr("device_class")
		return dc == "garage" || dc == "gate"
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

func describe(s state, e registryEntry, area string) Device {
	d := Device{
		EntityID: s.EntityID,
		Name:     displayName(s, e),
		Area:     area,
		Aliases:  e.Aliases,
		State:    s.State,
		Details:  details(s),
		Actions:  actionNames(s),
		LookOnly: lookOnly(s),
	}
	if d.Actions == nil {
		d.Actions = []string{}
	}
	return d
}

func details(s state) map[string]any {
	out := map[string]any{}
	num := func(key string) (float64, bool) {
		v, ok := s.Attributes[key].(float64)
		return v, ok
	}
	if b, ok := num("brightness"); ok {
		out["brightness_percent"] = math.Round(b / 255 * 100)
	}
	if p, ok := num("current_position"); ok {
		out["position_percent"] = p
	}
	if t, ok := num("temperature"); ok && s.domain() == "climate" {
		out["target_temperature"] = t
	}
	if t, ok := num("current_temperature"); ok {
		out["current_temperature"] = t
	}
	if p, ok := num("percentage"); ok {
		out["speed_percent"] = p
	}
	if v, ok := num("volume_level"); ok {
		out["volume_percent"] = math.Round(v * 100)
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

func actionsFor(s state) map[string]actionSpec {
	if lookOnly(s) {
		return nil
	}
	onOff := map[string]actionSpec{"turn_on": plain("turn_on"), "turn_off": plain("turn_off"), "toggle": plain("toggle")}
	with := func(extra map[string]actionSpec) map[string]actionSpec {
		out := map[string]actionSpec{}
		for k, v := range onOff {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	switch s.domain() {
	case "light":
		return with(map[string]actionSpec{
			"set_brightness":        numberAction("turn_on", "brightness_pct", 0, 100, 1),
			"set_color":             optionAction("turn_on", "color_name", "a colour name"),
			"set_color_temperature": numberAction("turn_on", "color_temp_kelvin", 1500, 9000, 1),
		})
	case "switch", "input_boolean":
		return with(nil)
	case "fan":
		return with(map[string]actionSpec{"set_speed": numberAction("set_percentage", "percentage", 0, 100, 1)})
	case "media_player":
		return map[string]actionSpec{
			"turn_on":    plain("turn_on"),
			"turn_off":   plain("turn_off"),
			"play":       plain("media_play"),
			"pause":      plain("media_pause"),
			"set_volume": numberAction("volume_set", "volume_level", 0, 100, 0.01),
		}
	case "cover":
		return map[string]actionSpec{
			"open":         plain("open_cover"),
			"close":        plain("close_cover"),
			"stop":         plain("stop_cover"),
			"set_position": numberAction("set_cover_position", "position", 0, 100, 1),
		}
	case "climate":
		return map[string]actionSpec{
			"turn_on":         plain("turn_on"),
			"turn_off":        plain("turn_off"),
			"set_temperature": numberAction("set_temperature", "temperature", -50, 120, 1),
			"set_mode":        optionAction("set_hvac_mode", "hvac_mode", "a mode such as heat, cool, auto or off"),
		}
	case "scene":
		return map[string]actionSpec{"activate": plain("turn_on")}
	case "script":
		return map[string]actionSpec{"run": plain("turn_on")}
	}
	return nil
}

func actionNames(s state) []string {
	specs := actionsFor(s)
	var names []string
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
	ws, _, err := websocket.Dial(ctx, c.wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach Home Assistant at %s", c.host)
	}
	ws.SetReadLimit(32 << 20)
	var hello struct {
		Type string `json:"type"`
	}
	if err := wsjson.Read(ctx, ws, &hello); err != nil || hello.Type != "auth_required" {
		ws.CloseNow()
		return nil, fmt.Errorf("%s didn't answer like Home Assistant", c.host)
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

func (c *conn) close() { c.ws.Close(websocket.StatusNormalClosure, "") }

// command sends msg with the next id and decodes its result into out
// (ignored when nil). Home Assistant's own error message is returned as the
// error.
func (c *conn) command(ctx context.Context, msg map[string]any, out any) error {
	id := c.nextID
	c.nextID++
	msg["id"] = id
	if err := wsjson.Write(ctx, c.ws, msg); err != nil {
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
				return errors.New("Home Assistant took too long to answer")
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
