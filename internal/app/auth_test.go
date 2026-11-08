package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

func TestBearerAuthSends(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()
	a, n, updates := sendingApp(srv.URL)
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	tt.Click("Auth")
	tt.Frame()
	tt.Click("Bearer")
	tt.Frame()
	if err := tt.Click("Token"); err != nil {
		t.Fatal(err)
	}
	tt.Type("s3cret")
	tt.Frame()
	if n.Request.Auth.Kind != model.AuthBearer || n.Request.Auth.Token != "s3cret" || !a.dirty {
		t.Fatalf("auth %+v, dirty %v", n.Request.Auth, a.dirty)
	}
	// The Tester's texts leave out what inputs hold, so this checks the
	// state the input follows; the screenshot checks the dots.
	if a.tabs[0].reveal || !tt.HasText("Show") {
		t.Fatal("the token starts shown")
	}
	tt.Click("Show")
	tt.Frame()
	if !a.tabs[0].reveal || !tt.HasText("Hide") {
		t.Error("Show doesn't show the token")
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	(<-updates)()
	if got != "Bearer s3cret" {
		t.Errorf("server saw Authorization %q", got)
	}
}

func TestCopyAuthFillsVariables(t *testing.T) {
	a, n, tt := suggestApp()
	n.Request.Auth = model.Auth{Kind: model.AuthBasic, Username: "me@{{base}}", Password: "{{token}}"}
	a.changed()
	tt.Click("Auth")
	tt.Frame()
	tt.Click("Copy Username")
	tt.Frame()
	if tt.Clipboard() != "me@https://a.dev" {
		t.Errorf("copied the username as %q", tt.Clipboard())
	}
	tt.Click("Copy Password")
	tt.Frame()
	if tt.Clipboard() != "t" || n.Request.Auth.Password != "{{token}}" {
		t.Errorf("copied the password as %q, auth %+v", tt.Clipboard(), n.Request.Auth)
	}
}
