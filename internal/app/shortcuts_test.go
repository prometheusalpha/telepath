package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

func TestKeysRoundTrip(t *testing.T) {
	for _, k := range []keys{{ui.Super | ui.Shift, ui.KeyN}, {ui.Super, ui.KeyEnter}, {ui.Ctrl | ui.Alt, ui.KeyF12}, {ui.Super, ui.KeyComma}} {
		got, ok := parseKeys(k.accel())
		if !ok || got != k {
			t.Errorf("%q came back as %v", k.accel(), got)
		}
	}
	if s := (keys{ui.Super | ui.Shift, ui.KeyW}).String(); s != "⇧⌘W" {
		t.Errorf("shown as %q", s)
	}
	if _, ok := parseKeys("Hyper+N"); ok {
		t.Error("an unknown modifier parses")
	}
}

func TestCloseOtherTabsKeys(t *testing.T) {
	ws := &model.Workspace{}
	for _, name := range []string{"one", "two", "three"} {
		ws.Add(model.NewRequest(name), "")
	}
	a := newApp(ws, "")
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyW) // no tab open: nothing to keep
	tt.Frame()
	for _, n := range ws.Items {
		a.openTab(n.ID)
	}
	a.active = 1
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyW)
	tt.Frame()
	if len(a.tabs) != 1 || a.tabs[0].id != ws.Items[1].ID {
		t.Fatalf("tabs %d", len(a.tabs))
	}
}

func TestPasteCurlIntoTheURL(t *testing.T) {
	ws := &model.Workspace{}
	n := model.NewRequest("Users")
	n.Request.Body = model.Body{Kind: model.BodyJSON, Text: `{"old":1}`}
	ws.Add(n, "")
	a := newApp(ws, "")
	a.openTab(n.ID)
	a.current().reqTab = 3 // the body, so its editor is made from the old text
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.Click("URL"); err != nil {
		t.Fatal(err)
	}
	tt.SetClipboard(`curl -X PUT 'https://api.dev/users/1?x=2' -H 'Accept: text/plain' --data-raw '{"name":"a"}'`)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	tt.Frame()
	r := n.Request
	if n.Name != "Users" || r.Method != "PUT" || r.URL != "https://api.dev/users/1?x=2" || len(r.Query) != 1 ||
		len(r.Headers) != 1 || r.Body.Text != `{"name":"a"}` {
		t.Fatalf("request %+v", r)
	}
	if !a.dirty || !tt.HasText("Imported cURL") {
		t.Errorf("dirty %v, texts %q", a.dirty, tt.Texts())
	}

	// Anything else pastes as text.
	r.URL = ""
	tt.Frame()
	tt.SetClipboard("https://b.dev/curl")
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	if r.URL != "https://b.dev/curl" || r.Method != "PUT" {
		t.Errorf("URL %q, method %s", r.URL, r.Method)
	}
}

func shortcutsTab(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a := emptyApp()
	tt := ui.NewTester(a.view, 1200, 780)
	a.openSettings()
	tt.Frame()
	if err := tt.Click("Shortcuts"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	return a, tt
}

func TestRecordAShortcut(t *testing.T) {
	a, tt := shortcutsTab(t)
	if err := tt.Click("Search… shortcut"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.recording != "search" || !tt.HasText("Press keys…") {
		t.Fatalf("recording %q", a.recording)
	}
	tt.Key(0, ui.KeyJ) // a key on its own would type instead
	tt.Frame()
	if a.recording != "search" || !tt.HasText("J needs ⌘ or ⌃ with it.") {
		t.Fatalf("recording %q, texts %q", a.recording, tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyN) // New Request's
	tt.Frame()
	if a.recording != "" || a.ws.Shortcuts["search"] != "Cmd+N" || a.ws.Shortcuts["new-request"] != "" {
		t.Fatalf("recording %q, shortcuts %v", a.recording, a.ws.Shortcuts)
	}
	if !a.keys("new-request").none() || !tt.HasText("Removed from New Request.") || !a.dirty {
		t.Errorf("new request %v, texts %q", a.keys("new-request"), tt.Texts())
	}

	// Esc stops recording and leaves Settings open.
	if err := tt.Click("Close Tab shortcut"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.recording != "" || !a.settingsOpen || a.keys("close-tab") != (keys{ui.Cmd, ui.KeyW}) {
		t.Fatalf("recording %q, open %v", a.recording, a.settingsOpen)
	}
	// ⌫ takes the keys away.
	if err := tt.Click("Close Tab shortcut"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if !a.keys("close-tab").none() || a.ws.Shortcuts["close-tab"] != "" {
		t.Fatalf("shortcuts %v", a.ws.Shortcuts)
	}

	a.settingsOpen = false
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyN)
	tt.Frame()
	if !a.searchOpen || len(a.ws.Items) != 0 {
		t.Fatalf("⌘N: search %v, items %d", a.searchOpen, len(a.ws.Items))
	}
}

func TestRestoreDefaultShortcuts(t *testing.T) {
	a, tt := shortcutsTab(t)
	a.ws.Shortcuts = map[string]string{"search": "Cmd+J", "send": ""}
	tt.Frame()
	if err := tt.Click("Reset Search…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := a.ws.Shortcuts["search"]; ok {
		t.Fatalf("shortcuts %v", a.ws.Shortcuts)
	}
	if err := tt.Click("Restore Defaults"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.ws.Shortcuts != nil || a.keys("send") != (keys{ui.Cmd, ui.KeyEnter}) {
		t.Fatalf("shortcuts %v", a.ws.Shortcuts)
	}
}
