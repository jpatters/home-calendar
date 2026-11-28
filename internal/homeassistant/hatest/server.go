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
