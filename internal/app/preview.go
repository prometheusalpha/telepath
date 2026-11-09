package app

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"telepath/internal/httpx"
)

// previewScheme serves the bodies a preview shows that aren't HTML text,
// as images and PDFs, which LoadHTML can't take.
const previewScheme = "telepath-preview"

// previewCSP lets a preview load what it looks like, style sheets,
// images and fonts from anywhere, but run no script: a response is
// someone else's page.
const previewCSP = "script-src 'none'; object-src 'none'; frame-src 'none'; worker-src 'none'; form-action 'none'"

// previewDocs are the bodies served at previewScheme, by path. The
// scheme's handler reads them from its own goroutine.
var (
	previewDocs sync.Map // string → previewDoc
	previewSeq  atomic.Int64
)

type previewDoc struct {
	body      []byte
	mediaType string
}

func servePreview(w http.ResponseWriter, r *http.Request) {
	v, ok := previewDocs.Load(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	d := v.(previewDoc)
	w.Header().Set("Content-Type", d.mediaType)
	// An SVG is a document that could run scripts. Not a PDF, whose
	// viewer the policy would keep from showing it.
	if d.mediaType != "application/pdf" {
		w.Header().Set("Content-Security-Policy", previewCSP)
	}
	w.Write(d.body)
}

// preview is the web view of a tab's response, made the first time it
// shows and kept with the tab.
type preview struct {
	view *mygo.WebView
	// of is the response it was loaded with; path is its body at
	// previewScheme, if it went there.
	of   *httpx.Response
	path string
	// url is the document's, the one the page may move within, to its
	// anchors. The navigation handler reads it off the main thread.
	url atomic.Pointer[string]
}

func (p *preview) release() {
	if p.path != "" {
		previewDocs.Delete(p.path)
		p.path = ""
	}
	if p.view != nil {
		p.view.Destroy()
		p.view = nil
	}
	p.of = nil
}

func (a *app) previewView(c *ui.Context, tb *tab) {
	p := &tb.preview
	if p.view == nil {
		if a.newWebView == nil {
			emptyState(c, "No preview", "Previews show in the app's window.")
			return
		}
		v, err := a.newWebView()
		if err != nil {
			emptyState(c, "Couldn't make the preview", err.Error())
			return
		}
		p.view = v
		v.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			// The preview shows this response alone: anchors in it scroll,
			// a link the user clicks opens in the browser, and the rest,
			// as forms and meta refreshes, go nowhere.
			doc, frag, ok := strings.Cut(e.URL, "#")
			if u := p.url.Load(); ok && frag != "" && u != nil && doc == strings.Split(*u, "#")[0] {
				return
			}
			e.PreventDefault()
			if e.UserInitiated && opensOutside(e.URL) && a.openExternal != nil {
				a.openExternal(e.URL)
			}
		})
	}
	if p.of != tb.resp {
		a.loadPreview(tb)
	}
	ui.WebView(c, p.view).Grow(1).AlignSelf(ui.Stretch)
}

func (a *app) loadPreview(tb *tab) {
	p, resp := &tb.preview, tb.resp
	p.of = resp
	if p.path != "" {
		previewDocs.Delete(p.path)
		p.path = ""
	}
	if resp.HTML() && tb.body != nil {
		// The body as shown, which leaves HTML unformatted, read in its
		// charset, which the page's own <meta> would otherwise decide; at
		// the response's URL, so what it links to relatively loads.
		u := resp.URL
		p.url.Store(&u)
		p.view.Page().LoadHTML(noScripts(tb.body.Buffer.String()), u)
		return
	}
	p.path = fmt.Sprintf("/%d", previewSeq.Add(1))
	previewDocs.Store(p.path, previewDoc{body: resp.Body, mediaType: resp.MediaType()})
	u := previewScheme + "://preview" + p.path
	p.url.Store(&u)
	p.view.Page().LoadURL(u)
}

// noScripts puts previewCSP at the top of an HTML page, before anything
// that could run, and after a doctype, which a tag before it would put
// the page in quirks mode.
func noScripts(html string) string {
	meta := `<meta http-equiv="Content-Security-Policy" content="` + previewCSP + `">`
	at := 0
	for {
		rest := strings.TrimLeft(html[at:], " \t\r\n\f\ufeff")
		at = len(html) - len(rest)
		switch {
		case strings.HasPrefix(rest, "<!--"):
			end := strings.Index(rest, "-->")
			if end < 0 {
				return meta + html
			}
			at += end + len("-->")
			continue
		case len(rest) >= 9 && strings.EqualFold(rest[:9], "<!doctype"):
			if end := strings.IndexByte(rest, '>'); end >= 0 {
				at += end + 1
				return html[:at] + meta + html[at:]
			}
		}
		return meta + html
	}
}

// opensOutside reports whether a link of a preview is one for the
// browser or mail app to open, not a scheme of the app's own or a file.
func opensOutside(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return true
	}
	return false
}
