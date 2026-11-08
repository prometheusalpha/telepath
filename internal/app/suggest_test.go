package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

func TestVarHelpers(t *testing.T) {
	text := []rune("x/{{ba")
	if open, q, ok := varQuery(text, len(text)); !ok || open != 2 || q != "ba" {
		t.Errorf("varQuery = %d %q %v", open, q, ok)
	}
	for _, s := range []string{"{{a b", "{{a}}", "{a", "plain"} {
		if _, _, ok := varQuery([]rune(s), len([]rune(s))); ok {
			t.Errorf("varQuery(%q) found a name", s)
		}
	}
	env := []model.KV{{Key: "token", Enabled: true}, {Key: "base", Enabled: true}, {Key: "api_base", Enabled: true}, {Key: "off", Value: "x"}}
	if got := matchVars(env, "ba"); !slices.Equal(got, []string{"base", "api_base"}) {
		t.Errorf("matchVars = %q", got)
	}
	got, at := insertVar([]rune("{{ba}}/x"), 0, 4, "base")
	if got != "{{base}}/x" || at != 8 {
		t.Errorf("insertVar = %q %d", got, at)
	}
}

func suggestApp() (*app, *model.Node, *ui.Tester) {
	a, n, _ := sendingApp("")
	a.ws.Env = []model.KV{{Key: "base", Value: "https://a.dev", Enabled: true}, {Key: "token", Value: "t", Enabled: true, Secret: true}}
	a.changed()
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	return a, n, tt
}

func TestSuggestInTheURL(t *testing.T) {
	a, n, tt := suggestApp()
	tt.Click("URL")
	tt.Type("{{ba")
	tt.Frame()
	if !tt.HasText("Variable base") || tt.HasText("Variable token") || !tt.HasText("https://a.dev") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Type("/users")
	tt.Frame()
	if n.Request.URL != "{{base}}/users" || len(a.tabs[0].missing) != 0 || tt.HasText("Variable base") {
		t.Errorf("url %q", n.Request.URL)
	}
}

func TestSuggestKeysAndClicks(t *testing.T) {
	_, n, tt := suggestApp()
	tt.Click("Headers")
	tt.Frame()
	tt.Click("Header new value")
	tt.Type("Bearer {{")
	tt.Frame()
	if !tt.HasText("Variable token") || !tt.HasText("••••••") {
		t.Fatalf("a secret's value shows: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if len(n.Request.Headers) != 1 || n.Request.Headers[0].Value != "Bearer {{token}}" {
		t.Fatalf("headers %+v", n.Request.Headers)
	}

	tt.Click("Header value 1")
	tt.Key(ui.Cmd, ui.KeyRight)
	tt.Type(" {{b")
	tt.Frame()
	if err := tt.Click("Variable base"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if n.Request.Headers[0].Value != "Bearer {{token}} {{base}}" {
		t.Errorf("after a click: %q", n.Request.Headers[0].Value)
	}

	// Escape closes the list, which stays closed while the name is typed.
	tt.Type(" {{")
	tt.Frame()
	if !tt.HasText("Variable token") {
		t.Fatal("no list to close")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Type("t")
	tt.Frame()
	if tt.HasText("Variable token") {
		t.Error("the list came back after Escape")
	}
}

func TestSuggestHeaderNamesAndVariables(t *testing.T) {
	a, n, tt := suggestApp()
	tt.Click("Headers")
	tt.Frame()
	// The blank row's input turns into the first row's at the first key:
	// the names still show.
	tt.Click("Header new key")
	tt.Type("c")
	tt.Frame()
	if !tt.HasText("Name Content-Type") || !tt.HasText("Name Accept") {
		t.Fatalf("after one key: %q", tt.Texts())
	}
	tt.Type("ont")
	tt.Frame()
	if !tt.HasText("Name Content-Type") || tt.HasText("Name Accept") {
		t.Fatalf("names for cont: %q", tt.Texts())
	}
	// None is taken until Down: Enter keeps a name typed in full.
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if n.Request.Headers[0].Key != "cont" {
		t.Fatalf("Enter took a name: %q", n.Request.Headers[0].Key)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Type("!")
	tt.Frame()
	if n.Request.Headers[0].Key != "Content-Type!" || tt.HasText("Name Content-Type") {
		t.Fatalf("key %q, texts %q", n.Request.Headers[0].Key, tt.Texts())
	}

	// After {{ the variables come, first and chosen.
	tt.Click("Header key 1")
	tt.Key(ui.Cmd, ui.KeyRight)
	tt.Type("{{to")
	tt.Frame()
	if !tt.HasText("Variable token") {
		t.Fatalf("no variables: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if n.Request.Headers[0].Key != "Content-Type!{{token}}" {
		t.Fatalf("key %q", n.Request.Headers[0].Key)
	}

	// Escape closes the names until typing again; a click takes one. The
	// pointer leaves {{token}} first, whose card covers the row below.
	tt.Move(0, 0)
	tt.Frame()
	tt.Click("Header new key")
	tt.Type("x-")
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tt.HasText("Name X-API-Key") {
		t.Fatal("Escape left the names")
	}
	tt.Type("r")
	tt.Frame()
	if err := tt.Click("Name X-Request-ID"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	if n.Request.Headers[1].Key != "X-Request-ID" || a.suggest.label != "" {
		t.Errorf("after a click: %+v", n.Request.Headers)
	}
}
