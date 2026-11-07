package store

import (
	"path/filepath"
	"testing"

	"telepath/internal/model"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "workspace.json")
	w, _, err := Load(path, nil)
	if err != nil || len(w.Items) != 0 {
		t.Fatalf("missing file: %v, %d items", err, len(w.Items))
	}
	f := model.NewFolder("Auth")
	w.Add(f, "")
	w.Add(model.NewRequest("login"), f.ID)
	if err := Save(path, w, nil); err != nil {
		t.Fatal(err)
	}
	got, _, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "Auth" || !got.Items[0].Folder.Open ||
		got.Items[0].Folder.Children[0].Request.Method != "GET" {
		t.Errorf("loaded %+v", got.Items)
	}
}

func TestSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if s := LoadSession(path); len(s.Tabs) != 0 {
		t.Fatalf("missing file: %+v", s)
	}
	want := Session{Tabs: []string{"a", "b"}, Active: 1, SidebarWidth: 300, Split: 60}
	if err := SaveSession(path, want); err != nil {
		t.Fatal(err)
	}
	if got := LoadSession(path); got.Active != 1 || len(got.Tabs) != 2 || got.SidebarWidth != 300 || got.Split != 60 {
		t.Errorf("loaded %+v", got)
	}
}
