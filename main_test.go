package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
	"telepath/internal/store"
)

func emptyApp() *app { return newApp(&model.Workspace{}, "") }

func TestEmptyWorkspace(t *testing.T) {
	tt := ui.NewTester(emptyApp().view, 1200, 780)
	for _, s := range []string{"COLLECTIONS", "No requests yet", "No request open"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q, texts %q", s, tt.Texts())
		}
	}
}

func TestCreateAndRename(t *testing.T) {
	a := emptyApp()
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.Click("New"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("New Folder"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.renaming == "" {
		t.Fatal("a new folder doesn't start renaming")
	}
	tt.Type("Users")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(a.ws.Items) != 1 || a.ws.Items[0].Name != "Users" || !tt.HasText("Users") {
		t.Fatalf("items %q, texts %q", names(a.ws.Items), tt.Texts())
	}
	if !a.dirty {
		t.Error("renaming doesn't mark the workspace for saving")
	}

	// The folder is chosen, so the next request goes inside it.
	if err := tt.Click("New"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("New Request"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	kids := a.ws.Items[0].Folder.Children
	if len(kids) != 1 || kids[0].Name != "New Request" || !tt.HasText("New Request") {
		t.Fatalf("children %q, texts %q", names(kids), tt.Texts())
	}
}

func TestDeleteAsks(t *testing.T) {
	ws := &model.Workspace{}
	ws.Add(model.NewRequest("ping"), "")
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.RightClick("ping"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(ws.Items) != 1 || !tt.HasText("Delete “ping”?") {
		t.Fatalf("deleted without asking, texts %q", tt.Texts())
	}
	if err := tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(ws.Items) != 0 {
		t.Errorf("items %q after confirming", names(ws.Items))
	}
}

func names(items []*model.Node) []string {
	var s []string
	for _, n := range items {
		s = append(s, n.Name)
	}
	return s
}

func TestAutosave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	a := newApp(&model.Workspace{}, path)
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.Click("New"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("New Folder"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, err := os.Stat(path); err == nil {
		t.Fatal("saved before the delay")
	}
	tt.Advance(saveDelay + 100*time.Millisecond)
	ws, err := store.Load(path)
	if err != nil || len(ws.Items) != 1 {
		t.Fatalf("after the delay: %v, %d items", err, len(ws.Items))
	}
}

func TestFolderOpensOnClick(t *testing.T) {
	ws := &model.Workspace{}
	f := model.NewFolder("Users")
	ws.Add(f, "")
	ws.Add(model.NewRequest("list"), f.ID)
	f.Folder.Open = false
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.Click("Users"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("list") || a.selected != f.ID {
		t.Fatalf("one click: open %v, chosen %q, texts %q", a.files.Open.Has(f.ID), a.selected, tt.Texts())
	}
	// Quick, within the double-click window: each press still toggles.
	if err := tt.Click("Users"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("list") {
		t.Fatalf("a quick second click leaves the folder open")
	}
	// Left of the arrow, in the row's indent.
	r, _ := tt.Find("Users")
	tt.ClickAt(14, r.Y+r.H/2)
	tt.Frame()
	if !tt.HasText("list") {
		t.Errorf("a click in the row's indent doesn't open the folder")
	}
}

func TestFolderMenuKeepsItClosed(t *testing.T) {
	ws := &model.Workspace{}
	f := model.NewFolder("Users")
	ws.Add(f, "")
	ws.Add(model.NewRequest("list"), f.ID)
	f.Folder.Open = false
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.RightClick("Users"); err != nil {
		t.Fatal(err)
	}
	tt.CloseMenu()
	tt.Frame()
	if a.files.Open.Has(f.ID) {
		t.Error("opening the folder's menu opens the folder")
	}
}
