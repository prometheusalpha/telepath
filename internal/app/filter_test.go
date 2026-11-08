package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestFilterResponse(t *testing.T) {
	bodies := []string{
		`{"users":[{"name":"Ada","age":36},{"name":"Bob","age":25}],"total":2}`,
		`{"users":[],"total":0}`,
	}
	sent := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := bodies[min(sent, len(bodies)-1)]
		if strings.HasPrefix(body, "{") {
			w.Header().Set("Content-Type", "application/json")
		}
		w.Write([]byte(body))
		sent++
	}))
	defer srv.Close()
	a, _, updates := sendingApp(srv.URL)
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	tt.Click("Send")
	tt.Frame()
	(<-updates)()
	tt.Frame()
	tb := a.tabs[0]
	shown := func() string { return tb.shown().Buffer.String() }

	if err := tt.Click("Response filter"); err != nil {
		t.Fatal(err)
	}
	seq := tb.filter.seq
	tt.Type("users[?(@.age > 30)].name")
	tt.Frame()
	if len(updates) != 0 || tb.filter.seq != seq {
		t.Fatal("the filter ran before typing paused")
	}
	tt.Advance(filterDelay + 50*time.Millisecond)
	(<-updates)()
	tt.Frame()
	if shown() != "[\n  \"Ada\"\n]" || !tt.HasText("1 match") {
		t.Fatalf("shown %q, texts %q", shown(), tt.Texts())
	}
	tt.Click("Copy body")
	tt.Frame()
	if tt.Clipboard() != shown() {
		t.Errorf("clipboard %q", tt.Clipboard())
	}

	// A typo says so and keeps the last result.
	tt.Click("Response filter")
	tt.Type("[")
	tt.Advance(filterDelay + 50*time.Millisecond)
	(<-updates)()
	tt.Frame()
	if !hasTextPrefix(tt, "unexpected end at character") || shown() != "[\n  \"Ada\"\n]" {
		t.Errorf("after a typo, shown %q, texts %q", shown(), tt.Texts())
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tb.filter.expr != "" || shown() != tb.body.Buffer.String() || hasTextPrefix(tt, "unexpected") {
		t.Errorf("Escape left %q, shown %q", tb.filter.expr, shown())
	}

	// The button clears it as Escape does, the input focused to type anew.
	tt.Type("$.users")
	tt.Advance(filterDelay + 50*time.Millisecond)
	(<-updates)()
	tt.Frame()
	if err := tt.Click("Clear filter"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tb.filter.expr != "" || shown() != tb.body.Buffer.String() || !tt.Focused("Response filter") {
		t.Errorf("clearing left %q, shown %q", tb.filter.expr, shown())
	}
	if _, ok := tt.Find("Clear filter"); ok {
		t.Error("an empty filter has a clear button")
	}

	// A singular expression shows the value; the next response is
	// filtered as it comes, its body parsed anew.
	tt.Type("$.total")
	tt.Advance(filterDelay + 50*time.Millisecond)
	(<-updates)()
	tt.Frame()
	if shown() != "2" {
		t.Fatalf("shown %q", shown())
	}
	tt.Click("Send")
	tt.Frame()
	(<-updates)()
	tt.Frame()
	(<-updates)()
	tt.Frame()
	if shown() != "0" || tb.filter.docOf != tb.resp {
		t.Errorf("after sending again, shown %q", shown())
	}

	tt.Click("Response filter")
	tt.Type("x[*]")
	tt.Advance(filterDelay + 50*time.Millisecond)
	(<-updates)()
	tt.Frame()
	if !tt.HasText("No match") {
		t.Errorf("texts %q", tt.Texts())
	}

	// A body that isn't JSON shows as it came, the filter kept for later.
	bodies = append(bodies, "plain")
	tt.Click("Send")
	tt.Frame()
	(<-updates)()
	tt.Frame()
	if shown() != "plain" || tt.HasText("No match") || tb.filter.expr == "" {
		t.Errorf("a text body shows %q", shown())
	}
}

func TestFilterNeedsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain"))
	}))
	defer srv.Close()
	a, _, updates := sendingApp(srv.URL)
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	tt.Click("Send")
	tt.Frame()
	(<-updates)()
	tt.Frame()
	if _, ok := tt.Find("Response filter"); ok {
		t.Error("a text body has a filter")
	}
}

func hasTextPrefix(tt *ui.Tester, prefix string) bool {
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
