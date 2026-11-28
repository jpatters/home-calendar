// Package memories keeps what Ziggy remembers about the household between
// conversations, in a private file beside the config.
package memories

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxTextLen is the longest a memory may be, in characters.
	MaxTextLen = 300
	// MaxMemories caps how many memories are kept; every one is sent to the
	// model on every turn.
	MaxMemories = 150
)

// The errors read as sentences because Ziggy relays them.
var (
	ErrNotFound = errors.New("that memory doesn't exist")
	ErrInvalid  = fmt.Errorf("a memory must be between 1 and %d characters", MaxTextLen)
	ErrFull     = fmt.Errorf("memory is full (%d memories); update or forget one first", MaxMemories)
)

// Memory is one thing Ziggy remembers.
type Memory struct {
	ID      string    `json:"id"`
	Text    string    `json:"text"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

type fileData struct {
	Memories []Memory `json:"memories"`
}

// Store holds the memories, saved to its file after every change.
type Store struct {
	path string
	mu   sync.Mutex
	data fileData
}

// PathFor returns the memories file path for a config file path:
// memories.json in the same directory.
func PathFor(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "memories.json")
}

// Open reads the memories at path. A missing file means none yet; a file that
// can't be read or parsed is an error and is left untouched, so memories are
// never silently lost.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("read memories: %w", err)
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("parse memories %s: %w", path, err)
	}
	return s, nil
}

// List returns the memories, oldest first.
func (s *Store) List() []Memory {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.Memories)
}

// Add remembers text.
func (s *Store) Add(text string) (Memory, error) {
	text, err := clean(text)
	if err != nil {
		return Memory{}, err
	}
	now := time.Now().UTC()
	m := Memory{ID: uuid.NewString(), Text: text, Created: now, Updated: now}
	err = s.update(func(d *fileData) error {
		if len(d.Memories) >= MaxMemories {
			return ErrFull
		}
		d.Memories = append(d.Memories, m)
		return nil
	})
	return m, err
}

// Update replaces the text of the memory with id.
func (s *Store) Update(id, text string) (Memory, error) {
	text, err := clean(text)
	if err != nil {
		return Memory{}, err
	}
	var m Memory
	err = s.update(func(d *fileData) error {
		i := slices.IndexFunc(d.Memories, func(m Memory) bool { return m.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		d.Memories[i].Text = text
		d.Memories[i].Updated = time.Now().UTC()
		m = d.Memories[i]
		return nil
	})
	return m, err
}

// Delete forgets the memory with id.
func (s *Store) Delete(id string) error {
	return s.update(func(d *fileData) error {
		i := slices.IndexFunc(d.Memories, func(m Memory) bool { return m.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		d.Memories = slices.Delete(d.Memories, i, i+1)
		return nil
	})
}

func clean(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > MaxTextLen {
		return "", ErrInvalid
	}
	return text, nil
}

// update applies fn to a copy and saves it; the in-memory data changes only
// once the file has been written.
func (s *Store) update(fn func(*fileData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := fileData{Memories: slices.Clone(s.data.Memories)}
	if err := fn(&next); err != nil {
		return err
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *Store) save(d fileData) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create memories dir: %w", err)
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("encode memories: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".memories-*.tmp")
	if err != nil {
		return fmt.Errorf("save memories: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("save memories: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("save memories: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("save memories: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save memories: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("save memories: %w", err)
	}
	return nil
}
