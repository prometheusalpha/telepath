package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

func TestFuzzyRanks(t *testing.T) {
	gu, ok1 := fuzzy("gu", "get user")
	lo, ok2 := fuzzy("gu", "logout")
	if !ok1 || !ok2 || gu <= lo {
		t.Errorf("get user %d, logout %d", gu, lo)
	}
	start, _ := fuzzy("user", "user audit log")
	mid, _ := fuzzy("user", "ban user")
	if start <= mid {
		t.Errorf("user audit log %d, ban user %d", start, mid)
	}
	if _, ok := fuzzy("xyz", "get user"); ok {
		t.Error("xyz matches get user")
	}
	if _, ok := fuzzy("ÜS", "über user"); !ok {
		t.Error("case and non-ASCII")
	}
}

func searchWorkspace() (*model.Workspace, *model.Node, *model.Node) {
	ws := &model.Workspace{}
	users := model.NewFolder("Users")
	ws.Add(users, "")
	admin := model.NewFolder("Admin")
	ws.Add(admin, users.ID)
	ban := model.NewRequest("ban user")
	ban.Request.Method, ban.Request.URL = "DELETE", "{{base}}/admin/ban"
	ws.Add(ban, admin.ID)
	ws.Add(model.NewRequest("debug"), "")
	users.Folder.Open, admin.Folder.Open = false, false
	return ws, admin, ban
}

func TestSearchOpensWithKeys(t *testing.T) {
	ws, admin, ban := searchWorkspace()
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.searchOpen || !tt.HasText("Requests you open show here") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Type("ban")
	tt.Frame()
	if len(a.hits) != 1 || a.hits[0].id != ban.ID || !tt.HasText("Users / Admin") {
		t.Fatalf("hits %+v", a.hits)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	if a.searchOpen || len(a.tabs) != 1 || a.tabs[0].id != ban.ID {
		t.Fatalf("after Enter: open %v, tabs %d", a.searchOpen, len(a.tabs))
	}
	if !a.files.Open.Has(admin.ID) || a.selected != ban.ID {
		t.Errorf("the sidebar doesn't show it: Admin open %v, chosen %q", a.files.Open.Has(admin.ID), a.selected)
	}

	// Recent first, then the arrows move the choice.
	tt.Click("Search")
	tt.Frame()
	if !tt.HasText("Recent") || len(a.hits) != 1 {
		t.Fatalf("recent %+v", a.hits)
	}
	tt.Type("u")
	tt.Frame()
	n := len(a.hits)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if n < 2 || a.searchSel != min(3, n-1) {
		t.Errorf("hits %d, chosen %d after three Downs", n, a.searchSel)
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if a.searchSel != min(3, n-1)-1 {
		t.Errorf("chosen %d after Up", a.searchSel)
	}
}

func TestSearchMatchesMethodAndURL(t *testing.T) {
	ws, _, ban := searchWorkspace()
	a := newApp(ws, "")
	a.ws.Env = []model.KV{{Key: "base", Value: "https://api.acme.dev", Enabled: true}}
	a.changed()
	// The URL as typed, and with its variables filled in.
	for _, q := range []string{"delete", "admin ban", "base", "/admin/ban", "acme"} {
		hits := a.searchHits(q)
		if len(hits) == 0 || hits[0].id != ban.ID {
			t.Errorf("%q: %+v", q, hits)
		}
	}
}
