package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

func TestTabMenu(t *testing.T) {
	ws := &model.Workspace{}
	for _, name := range []string{"one", "two", "three"} {
		ws.Add(model.NewRequest(name), "")
	}
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	for _, n := range ws.Items {
		a.openTab(n.ID)
	}
	tt.Frame()
	tabMenu(t, tt, "two", "Close Others")
	if len(a.tabs) != 1 || a.tabs[0].id != ws.Items[1].ID || a.active != 0 {
		t.Fatalf("tabs %d, active %d", len(a.tabs), a.active)
	}

	a.openTab(ws.Items[0].ID)
	tt.Frame()
	tabMenu(t, tt, "one", "Close")
	if len(a.tabs) != 1 || a.tabs[0].id != ws.Items[1].ID {
		t.Fatalf("after Close, tabs %d", len(a.tabs))
	}
}

// tabMenu chooses item in the menu of the tab of the request named name,
// opened on its name, left of its close button.
func tabMenu(t *testing.T, tt *ui.Tester, name, item string) {
	t.Helper()
	x, ok := tt.Find("Close " + name)
	if !ok {
		t.Fatalf("no tab %q, texts %q", name, tt.Texts())
	}
	tt.RightClickAt(x.X-24, x.Y+x.H/2)
	if err := tt.ChooseMenuItem(item); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

func TestSidebarFollowsTheTab(t *testing.T) {
	ws := &model.Workspace{}
	f := model.NewFolder("Users")
	ws.Add(f, "")
	n := model.NewRequest("list")
	ws.Add(n, f.ID)
	other := model.NewRequest("other")
	ws.Add(other, "")
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	a.openTab(n.ID)
	a.openTab(other.ID)
	tt.Frame()
	tt.Frame()
	if a.selected != other.ID {
		t.Fatalf("chosen %q", a.selected)
	}
	a.files.Open.Remove(f.ID)
	tt.Key(ui.Cmd, ui.Key1)
	tt.Frame()
	tt.Frame()
	if !a.files.Open.Has(f.ID) || a.selected != n.ID {
		t.Fatalf("after ⌘1, open %v, chosen %q", a.files.Open.Has(f.ID), a.selected)
	}
	// Closing it shows the other tab, and the sidebar goes there too.
	tt.Key(ui.Cmd, ui.KeyW)
	tt.Frame()
	tt.Frame()
	if a.selected != other.ID {
		t.Errorf("after closing, chosen %q", a.selected)
	}
}
