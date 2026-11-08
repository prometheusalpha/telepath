package app

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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

func TestBearerShowsJWTClaims(t *testing.T) {
	a, n, tt := suggestApp()
	// {"alg":"HS256"}.{"sub":"42","name":"Ann","exp":1000000000}
	a.ws.Env = append(a.ws.Env, model.KV{Key: "jwt", Value: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI0MiIsIm5hbWUiOiJBbm4iLCJleHAiOjEwMDAwMDAwMDB9.sig", Enabled: true})
	n.Request.Auth = model.Auth{Kind: model.AuthBearer, Token: "{{jwt}}"}
	a.changed()
	tt.Click("Auth")
	tt.Frame()
	for _, s := range []string{"Claim", "sub", "42", "name", "Ann", "exp"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q, texts %q", s, tt.Texts())
		}
	}
	if !slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.HasSuffix(s, "expired") }) {
		t.Errorf("exp isn't shown expired, texts %q", tt.Texts())
	}

	tt.Click("Copy claim exp")
	tt.Frame()
	if tt.Clipboard() != "1000000000" {
		t.Errorf("copied exp as %q", tt.Clipboard())
	}
	tt.Click("Copy claim name")
	tt.Frame()
	if tt.Clipboard() != "Ann" {
		t.Errorf("copied name as %q", tt.Clipboard())
	}

	n.Request.Auth.Token = "opaque-token"
	tt.Frame()
	if tt.HasText("Claim") {
		t.Error("an opaque token shows claims")
	}
	n.Request.Auth.Token = "a.!!.c"
	tt.Frame()
	if !tt.HasText("Looks like a JWT, but its payload doesn't decode.") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestJWTClaimsKeepOrder(t *testing.T) {
	// {"b":1,"a":[true]}
	claims, ok, err := jwtClaims(" x.eyJiIjoxLCJhIjpbdHJ1ZV19.y ")
	if !ok || err != nil || len(claims) != 2 || claims[0].name != "b" || claims[1].value != "[true]" {
		t.Fatalf("claims %+v, ok %v, err %v", claims, ok, err)
	}
	// Padded, as some issuers send it: {"b":1}
	if claims, _, err := jwtClaims("x.eyJiIjoxfQ==.y"); err != nil || len(claims) != 1 {
		t.Errorf("a padded payload: %+v, %v", claims, err)
	}
	if _, ok, err := jwtClaims("x.WzFd.y"); !ok || err == nil {
		t.Errorf("an array payload: ok %v, err %v", ok, err)
	}
}
