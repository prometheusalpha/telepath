// Package store reads and writes the workspace file.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"telepath/internal/model"
)

// Load returns an empty workspace when the file does not exist yet. It
// decrypts the secrets s sealed; locked counts those it couldn't, as
// without the key (a nil s), which stay sealed.
func Load(path string, s *Sealer) (w *model.Workspace, locked int, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &model.Workspace{Version: model.Version, Items: []*model.Node{}}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	w = &model.Workspace{}
	if err := json.Unmarshal(b, w); err != nil {
		return nil, 0, err
	}
	if w.Items == nil {
		w.Items = []*model.Node{}
	}
	return w, s.opened(w), nil
}

// Save writes w with its secrets sealed by s, or as they are with a nil s,
// where the system has no credential store.
func Save(path string, w *model.Workspace, s *Sealer) error {
	b, err := s.sealed(w)
	if err != nil {
		return err
	}
	return write(path, b)
}

// write goes through a temporary file so a crash mid-write leaves the old
// file intact.
func write(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Session is how the window was left: kept apart from the workspace, as it
// isn't the user's data and an export shouldn't carry it.
type Session struct {
	Tabs         []string `json:"tabs"` // request IDs, in order
	Active       int      `json:"active"`
	SidebarWidth float32  `json:"sidebarWidth,omitempty"`
	Split        float32  `json:"split,omitempty"` // the request's share above the response, in percent
	// ResponseRight puts the response right of the request, which then
	// takes SplitRight percent of the width.
	ResponseRight bool    `json:"responseRight,omitempty"`
	SplitRight    float32 `json:"splitRight,omitempty"`
}

// LoadSession returns a zero session for a missing or unreadable file: a
// lost session only reopens fewer tabs.
func LoadSession(path string) Session {
	var s Session
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func SaveSession(path string, s Session) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return write(path, b)
}
