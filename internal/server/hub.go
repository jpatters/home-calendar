package server

import (
	"encoding/json"
	"sync"

	"github.com/jpatters/home-calendar/internal/types"
)

type Frame struct {
	Type    string                 `json:"type"`
	Config  *types.Config          `json:"config,omitempty"`
	Events  []types.Event          `json:"events,omitempty"`
	Weather *types.WeatherSnapshot `json:"weather,omitempty"`
	SnowDay *types.SnowDaySnapshot `json:"snowday,omitempty"`
}

// Hub fans out frames to every connected WebSocket client. Each client owns a
// buffered channel; if its channel is full it gets disconnected to keep the
// broadcast cheap.
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

type client struct {
	send chan []byte
	done chan struct{}
}

func (h *Hub) register() *client {
	c := &client{
		send: make(chan []byte, 8),
		done: make(chan struct{}),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.done)
	}
	h.mu.Unlock()
}

func (h *Hub) Broadcast(frame Frame) {
	data, err := json.Marshal(frame)
	if err != nil {
		return
	}
	h.mu.RLock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			// drop slow clients
		}
	}
	h.mu.RUnlock()
}
