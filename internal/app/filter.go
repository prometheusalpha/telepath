package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"telepath/internal/httpx"
	"telepath/internal/jsonpath"
)

// filterDelay is how long typing pauses before a filter runs.
const filterDelay = 150 * time.Millisecond

// filter narrows a JSON response body to what a JSONPath selects. The
// expression stays for the responses that follow, but not past the tab.
type filter struct {
	expr  string
	input ui.Handle // focused again once cleared, to type the next
	at    time.Time // when the expression as typed runs
	// ran and ranOn are what the last run was for, which may still be
	// going.
	ran   string
	ranOn *httpx.Response
	seq   int
	err   error
	count int
	// filtered shows in place of the body while there's an expression;
	// from is the response it came from.
	filtered *editor.State
	from     *httpx.Response
	// doc is the body parsed, kept for the next expressions.
	doc   *jsonpath.Value
	docOf *httpx.Response
}

// shown is the editor the body area shows: the filtered body, or the body.
func (t *tab) shown() *editor.State {
	if t.json && t.filter.expr != "" && t.filter.filtered != nil {
		return t.filter.filtered
	}
	return t.body
}

func (a *app) filterBar(c *ui.Context, tb *tab) {
	t := c.Theme()
	f := &tb.filter
	var in ui.Element
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 12).Shrink(0).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			// Room on the right for the clear button, which sits inside.
			in = ui.TextInput(c, &f.expr).Grow(1).MinWidth(0).Font(mono).FontFeatures(monoFeatures).FontSize(12).
				PaddingEnd(30).Placeholder("JSONPath, as $.data[*].id").Label("Response filter").Bind(&f.input)
			if f.expr != "" && iconButton(c, closeIcon, "Clear filter").Attach(ui.AnchorRight, ui.AnchorRight).Right(3).Clicked() {
				f.expr = ""
				f.input.Focus(c)
			}
		})
		if f.filtered != nil && f.expr != "" {
			matches := "matches"
			if f.count == 1 {
				matches = "match"
			}
			ui.Text(c, fmt.Sprintf("%d %s", f.count, matches)).FontSize(12).TextColor(t.TextMuted)
		}
	})
	in.OnTextKey(func(k ui.TextKey) bool {
		if k.Key != ui.KeyEscape || k.Mods != 0 || k.Text() == "" {
			return false
		}
		k.SetText("")
		return true
	})
	if in.Changed() {
		f.at = c.Now().Add(filterDelay)
	}
	if f.err != nil && f.expr != "" {
		ui.Text(c, f.err.Error()).FontSize(12).TextColor(t.Danger).Padding(0, 12, 6, 12).Shrink(0)
	}

	expr := strings.TrimSpace(f.expr)
	if expr == f.ran && tb.resp == f.ranOn {
		return
	}
	// A new response runs at once, its at long past.
	if now := c.Now(); expr != "" && now.Before(f.at) {
		c.After(f.at.Sub(now))
		return
	}
	a.runFilter(tb, expr)
}

// runFilter selects off the main thread, as parsing a large body takes a
// while; what comes back from an expression since replaced is dropped.
func (a *app) runFilter(tb *tab, expr string) {
	f := &tb.filter
	f.seq++
	f.ran, f.ranOn = expr, tb.resp
	if expr == "" {
		f.filtered, f.from, f.err = nil, nil, nil
		return
	}
	seq, resp, doc := f.seq, tb.resp, f.doc
	if f.docOf != resp {
		doc = nil
	}
	go func() {
		text, count, doc, err := filterBody(resp.Body, doc, expr)
		a.update(func() {
			if f.seq != seq {
				return
			}
			if doc != nil {
				f.doc, f.docOf = doc, resp
			}
			f.err = err
			if err != nil {
				// A typo keeps what the last expression showed, but not
				// what it showed of another response.
				if f.from != resp {
					f.filtered, f.from = nil, nil
				}
				return
			}
			big := len(text) > bigBody
			buf := editor.NewBuffer(text)
			if !big {
				buf.SetLanguage(editor.LanguageNamed("JSON"))
			}
			f.filtered = &editor.State{Buffer: buf, ReadOnly: true, SoftWrap: !big}
			f.from, f.count = resp, count
		})
	}()
}

// filterBody returns what expr selects of body, with the body parsed when
// doc is nil. A singular expression shows its value; any other, the list
// of what it found, even of one.
func filterBody(body []byte, doc *jsonpath.Value, expr string) (string, int, *jsonpath.Value, error) {
	// Compiled first, so a typo doesn't wait on the body.
	p, err := jsonpath.Compile(expr)
	if err != nil {
		return "", 0, nil, err
	}
	if doc == nil {
		if doc, err = jsonpath.Parse(body); err != nil {
			return "", 0, nil, errors.New("the body isn't valid JSON: " + err.Error())
		}
	}
	got := p.Select(doc)
	if p.Singular() {
		if len(got) == 0 {
			return "", 0, doc, nil
		}
		return got[0].Indent(), 1, doc, nil
	}
	return (&jsonpath.Value{Kind: jsonpath.Array, Items: got}).Indent(), len(got), doc, nil
}
