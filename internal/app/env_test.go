package app

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVariablesFillTheRequest(t *testing.T) {
	var path, tok string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, tok = r.URL.Path, r.Header.Get("X-Token")
	}))
	defer srv.Close()
	a, n, updates := sendingApp("{{base}}/users")
	n.Request.Headers = append(n.Request.Headers, keyValue("X-Token", "{{token}}"))
	tt := ui.NewTester(a.view, 1200, 780)

	if err := tt.Click("Variables"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Click("Variable new key")
	tt.Type("base")
	tt.Frame()
	tt.Click("Variable value 1")
	tt.Type(srv.URL)
	tt.Frame()
	if err := tt.Click("Hide Variable 1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.ws.Env[0].Secret || !tt.HasText("Show Variable 1") {
		t.Errorf("hiding a value: %+v", a.ws.Env[0])
	}
	tt.Click("Done")
	tt.Frame()
	if a.envOpen || a.env["base"] != srv.URL || !a.dirty {
		t.Fatalf("after Done: open %v, env %v", a.envOpen, a.env)
	}

	tt.Click("ping")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	(<-updates)()
	tt.Frame()
	if path != "/users" || tok != "{{token}}" {
		t.Errorf("server saw path %q, token %q", path, tok)
	}
	if !tt.HasText("No value for {{token}}: sent as typed.") {
		t.Errorf("no warning for {{token}}, texts %q", tt.Texts())
	}
}

func TestHoveringAVariableShowsItsValue(t *testing.T) {
	a, n, tt := suggestApp()
	n.Request.URL = "{{base}}/x/{{token}}/{{nope}}"
	tt.Frame()
	in, ok := tt.Find("URL")
	if !ok {
		t.Fatal("no URL input")
	}
	// Sweeping over the URL meets each reference once, in order, and the
	// / between them shows none.
	var met []string
	y := in.Y + in.H/2
	for x := in.X; x < in.X+in.W; x++ {
		tt.Move(x, y)
		tt.Frame()
		if name := a.tip.name; name != "" && (len(met) == 0 || met[len(met)-1] != name) {
			met = append(met, name)
		}
		if name := a.tip.name; len(met) > 0 && name == "" {
			met = append(met, "")
		}
	}
	if got := strings.Join(slices.Compact(met), ","); got != "base,,token,,nope," {
		t.Fatalf("sweeping met %q", got)
	}

	// Going down from {{base}} to its card keeps it open, to copy the value.
	tt.Move(in.X+in.W, y)
	tt.Frame()
	for x := in.X; a.tip.name != "base"; x++ {
		tt.Move(x, y)
		tt.Frame()
	}
	x := in.X + a.tip.x + 4
	for ; y < in.Y+in.H+16; y += 2 {
		tt.Move(x, y)
		tt.Frame()
		if a.tip.name != "base" {
			t.Fatalf("the card closed at y %v", y)
		}
	}
	if !tt.HasText("https://a.dev") {
		t.Fatalf("no value in %q", tt.Texts())
	}
	if err := tt.Click("Copy value"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.Clipboard() != "https://a.dev" {
		t.Errorf("copied %q", tt.Clipboard())
	}
	tt.Move(0, 0)
	tt.Frame()
	if a.tip != (varTip{}) || tt.HasText("https://a.dev") {
		t.Errorf("leaving left the card %+v", a.tip)
	}
}
