package app

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
	if tb := a.current(); tb == nil || tb.id != kids[0].ID {
		t.Error("the new request doesn't open in a tab")
	}

	// Its tab opening leaves the focus on the name being typed.
	tt.Key(ui.Cmd, ui.KeyN)
	tt.Frame()
	tt.Frame()
	tt.Type("list")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if kids = a.ws.Items[0].Folder.Children; len(kids) != 2 || kids[1].Name != "list" || a.current().id != kids[1].ID {
		t.Errorf("children %q, tabs %d", names(kids), len(a.tabs))
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
	ws, _, err := store.Load(path, nil)
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

func TestSessionComesBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.json")
	ws := &model.Workspace{}
	one, two := model.NewRequest("one"), model.NewRequest("two")
	ws.Add(one, "")
	ws.Add(two, "")
	a := newApp(ws, path)
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("one")
	tt.Frame()
	tt.Click("two")
	tt.Frame()
	a.sidebarWidth, a.split = 310, 62
	tt.Advance(saveDelay + 100*time.Millisecond)

	ws2, _, err := store.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A request gone since, as if deleted in another copy of the file.
	ws2.Remove(one.ID)
	b := newApp(ws2, path)
	b.restore(store.LoadSession(b.sessionPath))
	if len(b.tabs) != 1 || b.tabs[0].id != two.ID || b.active != 0 || b.sidebarWidth != 310 || b.split != 62 || b.dirty {
		t.Errorf("restored tabs %d, active %d, widths %v %v, dirty %v", len(b.tabs), b.active, b.sidebarWidth, b.split, b.dirty)
	}
}

func TestShortcuts(t *testing.T) {
	a := emptyApp()
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyN)
	tt.Frame()
	tt.Type("Users")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyN)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(a.ws.Items) != 1 || a.ws.Items[0].Name != "Users" || len(a.ws.Items[0].Folder.Children) != 1 {
		t.Fatalf("items %q", names(a.ws.Items))
	}
	ws := a.ws
	for _, name := range []string{"two", "three"} {
		ws.Add(model.NewRequest(name), "")
	}
	a.changed()
	tt.Frame()
	for _, name := range []string{"New Request", "two", "three"} {
		tt.Click(name)
		tt.Frame()
	}
	tt.Key(ui.Cmd, ui.Key1)
	tt.Frame()
	if a.active != 0 {
		t.Errorf("⌘1: tab %d", a.active)
	}
	tt.Key(ui.Cmd, ui.Key9)
	tt.Frame()
	if a.active != 2 {
		t.Errorf("⌘9: tab %d, want the last", a.active)
	}
	tt.Click("Collections")
	tt.Key(ui.Cmd, ui.KeyL)
	tt.Frame()
	tt.Type("x")
	tt.Frame()
	if got := ws.Items[2].Request.URL; got != "x" {
		t.Errorf("⌘L didn't focus the URL: %q", got)
	}
}

func TestEnterRenames(t *testing.T) {
	ws := &model.Workspace{}
	n := model.NewRequest("ping")
	ws.Add(n, "")
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.renaming != n.ID {
		t.Fatal("Enter on the chosen row doesn't rename it")
	}
	tt.Type("pong")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if n.Name != "pong" || a.renaming != "" {
		t.Fatalf("name %q, renaming %q", n.Name, a.renaming)
	}
	// The Enter that ends the rename doesn't start another.
	tt.Frame()
	if a.renaming != "" {
		t.Error("renaming again after Enter")
	}
	a.closeTab(0)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyDown)
	tt.Frame()
	if len(a.tabs) != 1 {
		t.Errorf("⌘↓ doesn't open the request: %d tabs", len(a.tabs))
	}
}

func TestRevealCurrent(t *testing.T) {
	ws := &model.Workspace{}
	f := model.NewFolder("Users")
	ws.Add(f, "")
	n := model.NewRequest("list")
	ws.Add(n, f.ID)
	ws.Add(model.NewRequest("other"), "")
	f.Folder.Open = false
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	if tt.HasText("Reveal Current Request") {
		t.Error("the button shows with no tab open")
	}
	a.openTab(n.ID)
	tt.Click("other")
	tt.Frame()
	tt.Key(ui.Cmd, ui.Key1)
	tt.Frame()
	if err := tt.Click("Reveal Current Request"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !a.files.Open.Has(f.ID) || a.selected != n.ID {
		t.Errorf("open %v, chosen %q", a.files.Open.Has(f.ID), a.selected)
	}
}
