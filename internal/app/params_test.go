package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParamsFollowTheURL(t *testing.T) {
	a, n, _ := sendingApp("https://a.dev/users")
	r := n.Request
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	tt.Click("URL")
	tt.Type("?page=1")
	tt.Frame()
	if len(r.Query) != 1 || r.Query[0].Key != "page" || !tt.HasText("Params 1") {
		t.Fatalf("query %+v, texts %q", r.Query, tt.Texts())
	}

	// Typing on in the blank row keeps the focus as it becomes a row.
	if err := tt.Click("Query new key"); err != nil {
		t.Fatal(err)
	}
	tt.Type("li")
	tt.Frame()
	tt.Type("mit")
	tt.Frame()
	if len(r.Query) != 2 || r.Query[1].Key != "limit" || r.URL != "https://a.dev/users?page=1&limit" {
		t.Fatalf("query %+v, url %q", r.Query, r.URL)
	}

	if err := tt.Click("Query 1 enabled"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if r.URL != "https://a.dev/users?limit" || len(r.Query) != 2 {
		t.Errorf("after disabling page: url %q, query %+v", r.URL, r.Query)
	}
}

func TestPathHeadersAndBodySend(t *testing.T) {
	var path, token, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		path, token, body = r.URL.Path, r.Header.Get("X-Token"), string(b)
	}))
	defer srv.Close()
	a, n, updates := sendingApp(srv.URL + "/users/:id")
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()

	tt.Click("Path 1")
	tt.Frame()
	if err := tt.Click("Path value 1"); err != nil {
		t.Fatal(err)
	}
	tt.Type("7")

	tt.Click("Headers")
	tt.Frame()
	tt.Click("Header new key")
	tt.Type("X-Token")
	tt.Frame()
	tt.Click("Header value 1")
	tt.Type("secret")

	tt.Click("Body")
	tt.Frame()
	tt.Click("JSON")
	tt.Frame()
	if err := tt.Click("Request body"); err != nil {
		t.Fatal(err)
	}
	tt.Type(`{"a":1}`)
	tt.Frame()
	if n.Request.Body.Text == "" {
		t.Fatal("typing in the body doesn't reach the request")
	}

	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	(<-updates)()
	tt.Frame()
	if path != "/users/7" || token != "secret" || body != n.Request.Body.Text {
		t.Errorf("server saw path %q, token %q, body %q (request body %q)", path, token, body, n.Request.Body.Text)
	}
}
