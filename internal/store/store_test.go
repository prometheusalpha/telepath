package store

import (
	"path/filepath"
	"testing"

	"telepath/internal/model"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "workspace.json")
	w, err := Load(path)
	if err != nil || len(w.Items) != 0 {
		t.Fatalf("missing file: %v, %d items", err, len(w.Items))
	}
	f := model.NewFolder("Auth")
	w.Add(f, "")
	w.Add(model.NewRequest("login"), f.ID)
	if err := Save(path, w); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "Auth" || !got.Items[0].Folder.Open ||
		got.Items[0].Folder.Children[0].Request.Method != "GET" {
		t.Errorf("loaded %+v", got.Items)
	}
}
