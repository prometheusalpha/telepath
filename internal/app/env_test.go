package app

import (
	"net/http"
	"net/http/httptest"
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
