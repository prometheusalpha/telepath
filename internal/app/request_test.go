package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

// sendingApp has one request in the sidebar; its update hands the
// response over to the test, which runs it where the Tester runs.
func sendingApp(url string) (*app, *model.Node, chan func()) {
	ws := &model.Workspace{}
	n := model.NewRequest("ping")
	n.Request.URL = url
	ws.Add(n, "")
	a := newApp(ws, "")
	updates := make(chan func(), 1)
	a.update = func(fn func()) { updates <- fn }
	return a, n, updates
}

func TestSendShowsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()
	a, _, updates := sendingApp(srv.URL)
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.Click("ping"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.tabs) != 1 || !tt.HasText("No response yet") {
		t.Fatalf("tabs %d, texts %q", len(a.tabs), tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText("Sending…") || !tt.HasText("Cancel") {
		t.Fatalf("while sending, texts %q", tt.Texts())
	}
	(<-updates)()
	tt.Frame()
	if !tt.HasText("404 Not Found") || !tt.HasText("Headers 3") {
		t.Fatalf("after the response, texts %q", tt.Texts())
	}
	if got := a.tabs[0].body.Buffer.String(); got != "{\n  \"error\": \"nope\"\n}" {
		t.Errorf("body %q", got)
	}
}

func TestCancelIgnoresLateResponse(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)
	a, _, updates := sendingApp(srv.URL)
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	tt.Click("Send")
	tt.Frame()
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	(<-updates)() // the canceled send comes back with its error
	tt.Frame()
	if !tt.HasText("Request canceled") || a.tabs[0].resp != nil {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestEditAndCloseTabs(t *testing.T) {
	a, n, _ := sendingApp("")
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	if err := tt.Click("URL"); err != nil {
		t.Fatal(err)
	}
	tt.Type("example.com")
	tt.Frame()
	if n.Request.URL != "example.com" || !a.dirty {
		t.Errorf("url %q, dirty %v", n.Request.URL, a.dirty)
	}
	if err := tt.Click("Close ping"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.tabs) != 0 || !tt.HasText("No request open") {
		t.Errorf("after closing, tabs %d", len(a.tabs))
	}

	tt.Click("ping")
	tt.Frame()
	a.ws.Remove(n.ID)
	a.changed()
	tt.Frame()
	if len(a.tabs) != 0 {
		t.Error("deleting a request leaves its tab open")
	}
}

func keyValue(k, v string) model.KV { return model.KV{Key: k, Value: v, Enabled: true} }

func TestCopyAndSaveTheBody(t *testing.T) {
	big := strings.Repeat(`{"a":1}`, bigBody/7+1)
	for _, body := range []string{`{"a":1}`, big} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(body))
		}))
		a, _, updates := sendingApp(srv.URL)
		var savedAs string
		var saved []byte
		a.saveFile = func(name string, data []byte, done func(error)) { savedAs, saved = name, data; done(nil) }
		tt := ui.NewTester(a.view, 1200, 780)
		tt.Click("ping")
		tt.Frame()
		tt.Click("Send")
		tt.Frame()
		(<-updates)()
		tt.Frame()
		srv.Close()

		if len(body) > bigBody {
			if a.tabs[0].body.Buffer.String() != body || !tt.HasText("A large body: shown as it came, without formatting.") {
				t.Error("a large body is formatted")
			}
			continue
		}
		tt.Click("Copy body")
		tt.Frame()
		if tt.Clipboard() != "{\n  \"a\": 1\n}" {
			t.Errorf("clipboard %q", tt.Clipboard())
		}
		tt.Click("Save body")
		tt.Frame()
		if savedAs != "response.json" || string(saved) != body || !tt.HasText("Saved the body") {
			t.Errorf("saved %q as %q", saved, savedAs)
		}
	}
}

func TestResponseOnTheRight(t *testing.T) {
	a, _, _ := sendingApp("https://a.dev")
	var told []bool
	a.onLayout = func(right bool) { told = append(told, right) }
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	if err := tt.Click("Response on the Right"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.responseRight || !tt.HasText("Response Below") || len(told) != 1 || !a.dirty {
		t.Fatalf("right %v, told %v", a.responseRight, told)
	}
	// Beside the request now: the response's text sits right of the URL's
	// start rather than below the tabs.
	url, _ := tt.Find("URL")
	empty, _ := tt.Find("No response yet")
	if empty.X < url.X+url.W/3 {
		t.Errorf("the response isn't on the right: at %v, the URL at %v", empty, url)
	}
	if s := a.session(); !s.ResponseRight || s.SplitRight != 50 || s.Split != 45 {
		t.Errorf("session %+v", s)
	}
}

func TestLongHeaderValueWraps(t *testing.T) {
	long := "https://a.nel.cloudflare.com/report/v4?s=" + strings.Repeat("DcJd9Aiu8C5QsKODUsq7w4gu", 12)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Report-To", long)
	}))
	defer srv.Close()
	a, _, updates := sendingApp(srv.URL)
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	(<-updates)()
	tt.Frame()
	if err := tt.Click("Headers 3"); err != nil {
		t.Fatalf("%v, texts %q", err, tt.Texts())
	}
	tt.Frame()
	r, ok := tt.Find(long)
	if !ok {
		t.Fatalf("no value, texts %q", tt.Texts())
	}
	// Broken only at the "?", the value is two lines, the second running
	// past the table; broken in the token too, it is more.
	if r.H < 40 {
		t.Errorf("value %+v is not wrapped in its token", r)
	}
}
