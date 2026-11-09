package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/encoding/japanese"
)

func TestNoScripts(t *testing.T) {
	meta := `<meta http-equiv="Content-Security-Policy" content="` + previewCSP + `">`
	for in, want := range map[string]string{
		"<p>hi":                                  meta + "<p>hi",
		"<!DOCTYPE html><html>":                  "<!DOCTYPE html>" + meta + "<html>",
		"\n  <!-- a --> <!doctype html>\n<head>": "\n  <!-- a --> <!doctype html>" + meta + "\n<head>",
		"<!-- open":                              meta + "<!-- open",
		"":                                       meta,
	} {
		if got := noScripts(in); got != want {
			t.Errorf("noScripts(%q) = %q, want %q", in, got, want)
		}
	}
}

func respond(t *testing.T, ctype string, body []byte) (*app, *ui.Tester) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ctype)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	a, _, updates := sendingApp(srv.URL)
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("ping")
	tt.Frame()
	tt.Click("Send")
	tt.Frame()
	(<-updates)()
	tt.Frame()
	return a, tt
}

func TestHTMLResponseTabs(t *testing.T) {
	a, tt := respond(t, "text/html; charset=utf-8", []byte("<p>hi</p>"))
	if !tt.HasText("Preview") {
		t.Fatalf("no Preview tab: %q", tt.Texts())
	}
	// Without a window there's no web view to show it in.
	tt.Click("Preview")
	tt.Frame()
	if a.tabs[0].respView != "Preview" || !tt.HasText("No preview") {
		t.Errorf("view %q, texts %q", a.tabs[0].respView, tt.Texts())
	}
}

func TestJSONHasNoPreview(t *testing.T) {
	_, tt := respond(t, "application/json", []byte(`{"a":1}`))
	if tt.HasText("Preview") {
		t.Error("JSON has a preview")
	}
}

func TestImageShowsPreviewOnly(t *testing.T) {
	a, tt := respond(t, "image/png", []byte("\x89PNG\r\n\x1a\n"))
	if !tt.HasText("Preview") || a.tabs[0].respView != "Preview" {
		t.Errorf("view %q, texts %q", a.tabs[0].respView, tt.Texts())
	}
	if _, ok := tt.Find("Charset"); ok {
		t.Error("an image offers a charset")
	}
}

func TestChooseCharset(t *testing.T) {
	sjis, _ := japanese.ShiftJIS.NewEncoder().Bytes([]byte("こんにちは"))
	a, tt := respond(t, "text/plain", sjis)
	tb := a.tabs[0]
	if tb.decodedAs != "windows-1252" || strings.Contains(tb.body.Buffer.String(), "こんにちは") {
		t.Fatalf("read as %q: %q", tb.decodedAs, tb.body.Buffer.String())
	}
	if !tt.HasText("Auto (windows-1252)") {
		t.Fatalf("texts %q", tt.Texts())
	}
	updates := make(chan func(), 1)
	a.update = func(fn func()) { updates <- fn }
	tt.SetSize(1200, 1400) // room for every option the select lists
	tt.Frame()
	if err := tt.Click("Charset"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("shift_jis"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	(<-updates)()
	tt.Frame()
	if tb.charset != "shift_jis" || tb.decodedAs != "shift_jis" || tb.body.Buffer.String() != "こんにちは" {
		t.Errorf("charset %q, read as %q: %q", tb.charset, tb.decodedAs, tb.body.Buffer.String())
	}
}

func TestOpensOutside(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://example.com/a": true,
		"HTTP://example.com":    true,
		"mailto:a@b.c":          true,
		"file:///etc/passwd":    false,
		"telepath-preview://x":  false,
		"javascript:alert(1)":   false,
		"%zz":                   false,
	} {
		if got := opensOutside(raw); got != want {
			t.Errorf("opensOutside(%q) = %v, want %v", raw, got, want)
		}
	}
}
