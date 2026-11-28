// Package hatest is an in-memory stand-in for Home Assistant's WebSocket API,
// for tests. It implements the token handshake, get_states, the entity, area
// and device registry commands and call_service, and records service calls.
package hatest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Token is the long-lived access token the fake accepts.
const Token = "test-ha-token"

// Entity is one entity in the fake instance.
type Entity struct {
	EntityID   string
	State      string
	Attributes map[string]any
	// Name is the entity registry's user-set name, if any.
	Name     string
	AreaID   string
	DeviceID string
	Aliases  []string
	// Exposed is whether the entity is exposed to Assist (conversation).
	Exposed bool
	// Hidden marks an entity hidden in the registry (hidden_by "user"), as
	// Home Assistant does to a switch wrapped by switch_as_x.
	Hidden bool
}

type Area struct{ ID, Name string }

// Device is a device registry entry; entities without their own area
// inherit its area.
type Device struct{ ID, AreaID string }

// Call is a recorded call_service.
type Call struct {
	Domain   string
	Service  string
	EntityID string
	Data     map[string]any
}

// Server is a running fake Home Assistant.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	entities []Entity
	areas    []Area
	devices  []Device
	calls    []Call
	// ServiceError, when set, makes every call_service fail with it.
	serviceError *Error
	serviceDelay time.Duration
}

// services are the services Home Assistant 2026.8 provides for each domain
// (from each integration's services.yaml), so a call to a service that
// doesn't exist fails here as it would on a real instance.
var services = map[string][]string{
	"light":         {"turn_on", "turn_off", "toggle"},
	"switch":        {"turn_on", "turn_off", "toggle"},
	"input_boolean": {"turn_on", "turn_off", "toggle"},
	"fan":           {"turn_on", "turn_off", "toggle", "set_percentage", "increase_speed", "decrease_speed", "oscillate", "set_direction", "set_preset_mode"},
	"media_player":  {"turn_on", "turn_off", "toggle", "volume_up", "volume_down", "volume_set", "volume_mute", "media_play", "media_pause", "media_play_pause", "media_stop", "media_next_track", "media_previous_track", "select_source"},
	"cover":         {"open_cover", "close_cover", "stop_cover", "toggle", "set_cover_position", "open_cover_tilt", "close_cover_tilt", "stop_cover_tilt", "set_cover_tilt_position"},
	"climate":       {"turn_on", "turn_off", "toggle", "set_temperature", "set_hvac_mode", "set_fan_mode", "set_preset_mode", "set_humidity", "set_swing_mode"},
	"scene":         {"turn_on"},
	"script":        {"turn_on", "turn_off", "toggle"},
	"lock":          {"lock", "unlock", "open"},
}

// Error is a Home Assistant WebSocket error result.
type Error struct{ Code, Message string }

// New starts a fake instance with the given entities, areas and devices.
func New(t *testing.T, entities []Entity, areas []Area, devices []Device) *Server {
	t.Helper()
	s := &Server{entities: entities, areas: areas, devices: devices}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/websocket", s.handle)
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// Calls returns the service calls received so far.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// DelayServices makes every later call_service take d before answering, as a
// slow cloud device would.
func (s *Server) DelayServices(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serviceDelay = d
}

// FailServices makes every later call_service fail with code and message.
func (s *Server) FailServices(code, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serviceError = &Error{Code: code, Message: message}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()
	if wsjson.Write(ctx, conn, map[string]any{"type": "auth_required", "ha_version": "2026.8.0"}) != nil {
		return
	}
	var auth struct {
		Type        string `json:"type"`
		AccessToken string `json:"access_token"`
	}
	if wsjson.Read(ctx, conn, &auth) != nil {
		return
	}
	if auth.Type != "auth" || auth.AccessToken != Token {
		wsjson.Write(ctx, conn, map[string]any{"type": "auth_invalid", "message": "Invalid access token or password"})
		conn.Close(websocket.StatusPolicyViolation, "")
		return
	}
	if wsjson.Write(ctx, conn, map[string]any{"type": "auth_ok", "ha_version": "2026.8.0"}) != nil {
		return
	}
	lastID := 0
	for {
		var msg map[string]any
		if wsjson.Read(ctx, conn, &msg) != nil {
			return
		}
		id, _ := msg["id"].(float64)
		if int(id) <= lastID {
			s.reply(ctx, conn, int(id), nil, &Error{Code: "id_reuse", Message: "Identifier values have to increase."})
			continue
		}
		lastID = int(id)
		result, e := s.dispatch(msg)
		if msg["type"] == "call_service" {
			s.mu.Lock()
			delay := s.serviceDelay
			s.mu.Unlock()
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		}
		s.reply(ctx, conn, lastID, result, e)
	}
}

func (s *Server) reply(ctx context.Context, conn *websocket.Conn, id int, result any, e *Error) {
	out := map[string]any{"id": id, "type": "result", "success": e == nil}
	if e != nil {
		out["error"] = map[string]string{"code": e.Code, "message": e.Message}
	} else {
		out["result"] = result
	}
	wsjson.Write(ctx, conn, out)
}

func (s *Server) dispatch(msg map[string]any) (any, *Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch msg["type"] {
	case "get_states":
		states := []map[string]any{}
		for _, e := range s.entities {
			states = append(states, map[string]any{"entity_id": e.EntityID, "state": e.State, "attributes": orEmpty(e.Attributes)})
		}
		return states, nil
	case "config/entity_registry/list":
		list := []map[string]any{}
		for _, e := range s.entities {
			list = append(list, s.registryEntry(e, false))
		}
		return list, nil
	case "config/entity_registry/get_entries":
		raw, _ := json.Marshal(msg["entity_ids"])
		var ids []string
		json.Unmarshal(raw, &ids)
		out := map[string]any{}
		for _, id := range ids {
			out[id] = nil
			for _, e := range s.entities {
				if e.EntityID == id {
					out[id] = s.registryEntry(e, true)
				}
			}
		}
		return out, nil
	case "get_config":
		return map[string]any{"unit_system": map[string]any{"temperature": "°C", "length": "km"}}, nil
	case "config/area_registry/list":
		list := []map[string]any{}
		for _, a := range s.areas {
			list = append(list, map[string]any{"area_id": a.ID, "name": a.Name, "aliases": []string{}})
		}
		return list, nil
	case "config/device_registry/list":
		list := []map[string]any{}
		for _, d := range s.devices {
			list = append(list, map[string]any{"id": d.ID, "area_id": nilIfEmpty(d.AreaID)})
		}
		return list, nil
	case "call_service":
		if s.serviceError != nil {
			return nil, s.serviceError
		}
		call := Call{}
		call.Domain, _ = msg["domain"].(string)
		call.Service, _ = msg["service"].(string)
		known := false
		for _, svc := range services[call.Domain] {
			known = known || svc == call.Service
		}
		if !known {
			return nil, &Error{Code: "not_found", Message: "Service " + call.Domain + "." + call.Service + " not found."}
		}
		if target, ok := msg["target"].(map[string]any); ok {
			call.EntityID, _ = target["entity_id"].(string)
		}
		call.Data, _ = msg["service_data"].(map[string]any)
		s.calls = append(s.calls, call)
		return map[string]any{"context": map[string]any{"id": "ctx"}}, nil
	}
	return nil, &Error{Code: "unknown_command", Message: "Unknown command."}
}

func (s *Server) registryEntry(e Entity, extended bool) map[string]any {
	entry := map[string]any{
		"entity_id": e.EntityID,
		"name":      nilIfEmpty(e.Name),
		"area_id":   nilIfEmpty(e.AreaID),
		"device_id": nilIfEmpty(e.DeviceID),
		"options":   map[string]any{"conversation": map[string]any{"should_expose": e.Exposed}},
		"hidden_by": nil,
	}
	if e.Hidden {
		entry["hidden_by"] = "user"
	}
	if extended {
		entry["aliases"] = append([]string{}, e.Aliases...)
	}
	return entry
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
