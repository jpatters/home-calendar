package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // homelab: no browser Origin check
	})
	if err != nil {
		log.Printf("ws: accept: %v", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "server closing")

	client := s.hub.register()
	defer s.hub.unregister(client)

	cfg := s.cfg.Get()
	snap := Frame{
		Type:    "snapshot",
		Config:  &cfg,
		Events:  s.ical.Events(),
		Weather: s.weather.Snapshot(),
		SnowDay: s.snowday.Snapshot(),
	}
	if data, err := json.Marshal(snap); err == nil {
		if err := conn.Write(r.Context(), websocket.MessageText, data); err != nil {
			return
		}
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				cancel()
				return
			}
		}
	}()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-client.done:
			return
		case msg := <-client.send:
			writeCtx, wc := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, msg)
			wc()
			if err != nil {
				return
			}
		case <-ping.C:
			pCtx, pc := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pCtx)
			pc()
			if err != nil {
				return
			}
		}
	}
}
