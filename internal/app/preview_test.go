package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/ui"
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

func TestHTMLResponsePreviews(t *testing.T) {
	a, tt := respond(t, "text/html; charset=utf-8", []byte("<p>hi</p>"))
	tb := a.tabs[0]
	// Without a window there's no web view to show it in.
	if tb.respView != "Body" || !tb.previewOn || tb.lang != "HTML" || !tt.HasText("No preview") {
		t.Fatalf("view %q, preview %v, lang %q, texts %q", tb.respView, tb.previewOn, tb.lang, tt.Texts())
	}
	// Choosing a language shows the text.
	tt.Click("View as")
	tt.Frame()
	tt.Click("View as HTML")
	tt.Frame()
	if tb.previewOn || tb.lang != "HTML" || tt.HasText("No preview") {
		t.Errorf("preview %v, lang %q, texts %q", tb.previewOn, tb.lang, tt.Texts())
	}
	tt.Click("View as")
	tt.Frame()
	tt.Click("Preview")
	tt.Frame()
	if !tb.previewOn || !tt.HasText("No preview") {
		t.Errorf("preview %v, texts %q", tb.previewOn, tt.Texts())
	}
}

func TestJSONShowsBody(t *testing.T) {
	a, tt := respond(t, "application/json", []byte(`{"a":1}`))
	tb := a.tabs[0]
	if tb.respView != "Body" || tb.previewOn || tb.lang != "JSON" {
		t.Errorf("view %q, preview %v, lang %q", tb.respView, tb.previewOn, tb.lang)
	}
	// Still in the menu, but there's nothing to preview.
	tt.Click("View as")
	tt.Frame()
	tt.Click("Preview")
	tt.Frame()
	if tb.previewOn {
		t.Error("JSON previews")
	}
	// Plain text, by choosing the language shown.
	if err := tt.Click("View as JSON"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tb.lang != "" || tb.body.Buffer.Language() != nil {
		t.Errorf("lang %q", tb.lang)
	}
}

func TestImagePreviews(t *testing.T) {
	a, _ := respond(t, "image/png", []byte("\x89PNG\r\n\x1a\n"))
	if tb := a.tabs[0]; !tb.previewOn || tb.respView != "Body" {
		t.Errorf("view %q, preview %v", tb.respView, tb.previewOn)
	}
}

func TestHeadersStayOverSends(t *testing.T) {
	a, tt := respond(t, "text/html", []byte("<p>hi</p>"))
	tb := a.tabs[0]
	tt.Click("Headers 3")
	tt.Frame()
	tb.previewOn = false
	updates := make(chan func(), 1)
	a.update = func(fn func()) { updates <- fn }
	tt.Click("Send")
	tt.Frame()
	(<-updates)()
	tt.Frame()
	if tb.respView != "Headers" || !tb.previewOn {
		t.Errorf("view %q, preview %v", tb.respView, tb.previewOn)
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
