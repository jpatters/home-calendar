package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jpatters/home-calendar/internal/memories"
)

// Ziggy's memories are private household data: they are served only by these
// endpoints and never broadcast or included in the config. Edits apply on
// Ziggy's next turn.

func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]memories.Memory{"memories": s.memories.List()})
}

func (s *Server) handleAddMemory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	m, err := s.memories.Add(body.Text)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleUpdateMemory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	m, err := s.memories.Update(r.PathValue("id"), body.Text)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if err := s.memories.Delete(r.PathValue("id")); err != nil {
		writeMemoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeMemoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, memories.ErrNotFound):
		writeError(w, http.StatusNotFound, "That memory doesn't exist.")
	case errors.Is(err, memories.ErrInvalid), errors.Is(err, memories.ErrFull):
		writeError(w, http.StatusBadRequest, sentence(err.Error()))
	default:
		writeError(w, http.StatusInternalServerError, "The memory couldn't be saved.")
	}
}

// sentence turns an error message written for Ziggy ("memory is full …") into
// one for the admin page ("Memory is full ….").
func sentence(msg string) string {
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:] + "."
}
