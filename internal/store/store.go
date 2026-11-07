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

// Load returns an empty workspace when the file does not exist yet.
func Load(path string) (*model.Workspace, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &model.Workspace{Version: model.Version, Items: []*model.Node{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var w model.Workspace
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, err
	}
	if w.Items == nil {
		w.Items = []*model.Node{}
	}
	return &w, nil
}

// Save writes through a temporary file so a crash mid-write leaves the old
// workspace intact.
func Save(path string, w *model.Workspace) error {
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
