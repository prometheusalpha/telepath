package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"telepath/internal/httpx"
	"telepath/internal/model"
)

// bigBody is the size past which a response body shows unformatted.
const bigBody = 2 << 20

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// tab is an open request: what the workspace doesn't keep, as the last
// response, which lives only while the tab does.
type tab struct {
	id     string
	reqTab int
	// respView is the response's tab shown, "Body" or "Headers".
	respView string
	// lang is what the body is highlighted as, one of bodyLangs or "" for
	// plain text, and previewOn shows it in a web view instead. Both
	// start over from the type of each response.
	lang      string
	previewOn bool
	viewOpen  bool // the menu of the two

	// drafts are the blank rows of the tables, by table.
	drafts   map[string]*model.KV
	bodyEdit *editor.State
	reveal   bool           // shows the password, token or key typed in the Auth tab
	pasted   *model.Request // a curl command pasted into the URL, for next frame

	sending bool
	sentAt  time.Time // frame time of the send, for the clock while sending
	cancel  context.CancelFunc
	// seq tells a response apart from those of sends canceled or replaced
	// since, which may still arrive.
	seq  int
	resp *httpx.Response
	err  error
	body *editor.State
	json bool // the body is JSON, found off the main thread
	// decodedAs is the charset body was read in, and decodeErr why that
	// failed, in which case it's as received.
	decodedAs string
	decodeErr error
	preview   preview
	filter    filter
	// missing are the variables the last send found no value for, which
	// went as typed.
	missing []string
}

// tick is how often the clock while sending moves.
const tick = 100 * time.Millisecond

func (t *tab) stop() {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.sending = false
	t.seq++
}

func (a *app) send(c *ui.Context, tb *tab) {
	n := a.tree.byID[tb.id]
	if n == nil || n.Request == nil {
		return
	}
	tb.stop()
	tb.sentAt = c.Now()
	// The goroutine reads a copy, expanded: the user may keep editing
	// meanwhile.
	req, missing := a.env.Request(n.Request)
	ctx, cancel := context.WithCancel(context.Background())
	tb.sending, tb.cancel, tb.err, tb.missing = true, cancel, nil, missing
	seq := tb.seq
	client := a.client
	go func() {
		resp, err := httpx.Send(ctx, client, req)
		var b readBody
		if resp != nil {
			b = readResponse(resp)
		}
		a.update(func() {
			if tb.seq != seq {
				return
			}
			tb.sending, tb.cancel = false, nil
			tb.resp, tb.err = resp, err
			if resp != nil {
				tb.show(b)
				// A page or a picture reads best as it looks, and the rest as
				// text. A table of headers looked at over sends stays.
				tb.lang = b.language
				tb.previewOn = resp.Previewable() && (resp.HTML() || resp.Binary())
				if tb.respView != "Headers" {
					tb.respView = "Body"
				}
			}
		})
	}()
}

// readBody is a response's body as the panel shows it, made off the main
// thread.
type readBody struct {
	text      string
	language  string
	json, big bool
	charset   string
	err       error
}

// readResponse reads the body in the charset the response says, and
// formats it. A large body shows as it came, as indenting and
// highlighting it would stall the window.
func readResponse(resp *httpx.Response) readBody {
	b := readBody{big: len(resp.Body) > bigBody, json: resp.JSON()}
	if resp.Binary() {
		b.text = string(resp.Body)
		return b
	}
	mt := resp.MediaType()
	switch {
	case b.json:
		b.language = "JSON"
	case resp.HTML():
		b.language = "HTML"
	case strings.HasSuffix(mt, "/xml") || strings.HasSuffix(mt, "+xml"):
		b.language = "XML"
	}
	b.charset = resp.Charset()
	b.text, b.err = httpx.Decode(resp.Body, b.charset)
	if !b.big {
		b.text = httpx.PrettyText(b.text, b.json)
	}
	return b
}

// show puts what readResponse read in the tab's editors.
func (tb *tab) show(b readBody) {
	tb.json, tb.decodedAs, tb.decodeErr = b.json, b.charset, b.err
	buf := editor.NewBuffer(b.text)
	if !b.big {
		buf.SetLanguage(editor.LanguageNamed(b.language))
	}
	tb.body = &editor.State{Buffer: buf, ReadOnly: true, SoftWrap: !b.big}
}

func (a *app) cancelSend(tb *tab) {
	tb.stop()
	tb.err = context.Canceled
}

func (a *app) requestView(c *ui.Context, tb *tab) {
	t := c.Theme()
	n := a.tree.byID[tb.id]
	r := n.Request
	if tb.pasted != nil {
		*r, tb.pasted = *tb.pasted, nil
		tb.bodyEdit = nil // the editor leads once made, so it's made anew
		a.changed()
		c.Toast("Imported cURL")
	}
	if a.shortcut(c, "send") {
		a.send(c, tb)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(10, 12).Shrink(0).Children(func() {
		m := ui.Select(c, &r.Method, methods).Width(104).Font(mono).FontFeatures(monoFeatures).FontWeight(700).
			TextColor(methodColor(t, r.Method)).Label("Method")
		if m.Changed() {
			a.changed()
		}
		url := ui.TextInput(c, &r.URL).Grow(1).MinWidth(0).Font(mono).FontFeatures(monoFeatures).
			Placeholder("{{base}}/users").Label("URL").Bind(&a.urlHandle).
			OnPaste(func(text string) bool { return a.pasteCurl(tb, text) })
		// The card of the variable pointed at says more than the URL.
		if a.highlightVars(c, url, r.URL, "URL", t.FontSize) && a.tip.label != "URL" {
			url.Tooltip(a.env.Expand(r.URL, nil))
		}
		if a.suggestVars(c, url, &r.URL, "URL") || url.Changed() {
			r.QueryFromURL()
			r.PathFromURL()
			a.changed()
		}
		if url.Submitted() {
			a.send(c, tb)
		}
		if tb.sending {
			if ui.Button(c, "Cancel").Width(84).Clicked() {
				a.cancelSend(tb)
			}
		} else if ui.PrimaryButton(c, "Send").Width(84).Tooltip(a.hint("", "send")).Clicked() {
			a.send(c, tb)
		}
	})
	request := func() {
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).AlignItems(ui.Center).Padding(0, 12).Children(func() {
				countTabs(c, &tb.reqTab, tabItem{"Params", counted(r.Query)}, tabItem{"Path", counted(r.Path)},
					tabItem{"Headers", counted(r.Headers)}, tabItem{label: bodyLabel(&r.Body)}, tabItem{label: authLabel(&r.Auth)}).MinWidth(0).Shrink(1)
				ui.Spacer(c)
				svg, label := splitRightIcon, "Response on the Right"
				if a.responseRight {
					svg, label = splitBelowIcon, "Response Below"
				}
				if iconButton(c, svg, label).Clicked() {
					a.setResponseRight(!a.responseRight)
				}
			})
			// The body editor scrolls on its own, so it takes the pane's
			// height instead of sitting in the Scroll at a fixed size.
			if tb.reqTab == 3 && (r.Body.Kind == model.BodyJSON || r.Body.Kind == model.BodyText) {
				ui.Column(c).Grow(1).MinHeight(0).Children(func() { a.requestPane(c, tb, r) })
			} else {
				ui.Scroll(c).Grow(1).Children(func() { a.requestPane(c, tb, r) })
			}
		})
	}
	response := func() { a.responseView(c, tb) }
	if a.responseRight {
		ui.SplitPercent(c, &a.splitRight, request, response).SplitReset(50).Grow(1)
	} else {
		ui.SplitVerticalPercent(c, &a.split, request, response).SplitReset(45).Grow(1)
	}
}

func (a *app) setResponseRight(right bool) {
	a.responseRight = right
	if a.onLayout != nil {
		a.onLayout(right)
	}
	a.touch()
}

func (a *app) responseView(c *ui.Context, tb *tab) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		if len(tb.missing) > 0 && !tb.sending {
			names := make([]string, len(tb.missing))
			for i, m := range tb.missing {
				names[i] = "{{" + m + "}}"
			}
			ui.Text(c, "No value for "+strings.Join(names, ", ")+": sent as typed.").
				FontSize(12).TextColor(t.Warning).Padding(6, 12).Background(t.Warning.Alpha(0.1))
		}
		switch {
		case tb.sending:
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "Sending…").TextColor(t.TextMuted)
				// Tenths shown as they pass, not rounded, so the next frame is
				// due when the next tenth starts.
				since := c.Now().Sub(tb.sentAt)
				ui.Text(c, fmt.Sprintf("%.1fs", since.Truncate(tick).Seconds())).
					FontSize(12).Font(mono).FontFeatures(monoFeatures).TextColor(t.TextMuted)
				c.After(tick - since%tick)
			})
		case errors.Is(tb.err, context.Canceled):
			emptyState(c, "Request canceled", "")
		case tb.err != nil:
			ui.Column(c).Grow(1).Center().Gap(6).Padding(24).Children(func() {
				ui.Text(c, "Couldn't send the request").FontWeight(600).TextColor(t.Danger)
				// A URL in the message has no place to break, and the centered column
				// would let it run past the pane on both sides, so cap it at the pane.
				ui.Text(c, tb.err.Error()).FontSize(12).Font(mono).FontFeatures(monoFeatures).TextColor(t.TextMuted).Selectable().MaxWidthPercent(100)
			})
		case tb.resp == nil:
			hint := "Press Send"
			if k := a.keys("send"); !k.none() {
				hint += ", or " + k.String()
			}
			emptyState(c, "No response yet", hint)
		default:
			a.responseBody(c, tb)
		}
	})
}

func statusColor(t *ui.Theme, code int) ui.Color {
	switch {
	case code >= 500:
		return t.Danger
	case code >= 400:
		return t.Warning
	case code >= 300:
		return accentInk(t)
	}
	return t.Success
}

func (a *app) responseBody(c *ui.Context, tb *tab) {
	t := c.Theme()
	resp := tb.resp
	views := []string{"Body", "Headers"}
	at := max(slices.Index(views, tb.respView), 0)
	ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(6, 12).Shrink(0).Children(func() {
		ui.Text(c, resp.Status).Font(mono).FontFeatures(monoFeatures).FontWeight(700).FontSize(12).TextColor(statusColor(t, resp.StatusCode))
		ui.Text(c, httpx.FormatDuration(resp.Duration)).FontSize(12).TextColor(t.TextMuted)
		size := httpx.FormatSize(resp.Size)
		if resp.Encoding != "" {
			size += " " + resp.Encoding
		}
		if resp.Truncated {
			size += " (showing the first " + httpx.FormatSize(httpx.MaxBody) + ")"
		}
		ui.Text(c, size).FontSize(12).TextColor(t.TextMuted)
		// Where it went, variables and path parameters filled in.
		ui.Text(c, resp.URL).Font(mono).FontFeatures(monoFeatures).FontSize(11).TextColor(t.TextMuted).SingleLine().
			Grow(1).MinWidth(0).Tooltip(resp.URL).Selectable()
		a.viewMenu(c, tb)
		// An image's or a PDF's bytes aren't text to paste.
		if tb.body != nil && !(resp.Binary() && resp.Previewable()) && iconButton(c, copyIcon, "Copy body").Clicked() {
			c.Services().WriteClipboard(tb.shown().Buffer.String())
			c.Toast("Copied the body")
		}
		if iconButton(c, downloadIcon, "Save body").Clicked() {
			a.saveFile(bodyFileName(resp), resp.Body, func(err error) {
				if err != nil {
					a.toast = "Couldn't save the body: " + err.Error()
				} else {
					a.toast = "Saved the body"
				}
			})
		}
		items := make([]tabItem, len(views))
		for i, v := range views {
			items[i] = tabItem{label: v}
			if v == "Headers" {
				items[i].count = len(resp.Headers)
			}
		}
		countTabs(c, &at, items...)
	})
	tb.respView = views[at]
	note := func(text string, color ui.Color) {
		ui.Text(c, text).FontSize(12).TextColor(color).Padding(4, 12).Shrink(0)
	}
	if resp.EncodingErr != nil {
		note("Couldn't decode the body ("+resp.EncodingErr.Error()+"): shown as received.", t.Warning)
	}
	if tb.decodeErr != nil {
		note("Couldn't read the body as "+tb.decodedAs+": shown as received.", t.Warning)
	}
	previewing := tb.respView == "Body" && tb.previewOn
	if len(resp.Body) > bigBody && !previewing {
		note("A large body: shown as it came, without formatting.", t.TextMuted)
	}
	ui.Divider(c)
	switch tb.respView {
	case "Body":
		if len(resp.Body) == 0 {
			emptyState(c, "Empty body", "")
			return
		}
		if previewing {
			a.previewView(c, tb)
			return
		}
		if tb.json {
			a.filterBar(c, tb)
			if tb.filter.filtered != nil && tb.filter.count == 0 {
				emptyState(c, "No match", "")
				return
			}
		}
		styleCode(c, tb.shown())
		editor.View(c, tb.shown()).Grow(1).FillWidth().Label("Response body")
	case "Headers":
		cols := []ui.TableColumn{{Title: "Name", Width: 220}, {Title: "Value"}}
		ui.Table(c, nil, cols, len(resp.Headers), func(row, col int) {
			h := resp.Headers[row]
			if col == 0 {
				ui.Text(c, h.Key).Font(mono).FontFeatures(monoFeatures).FontSize(12).SingleLine()
			} else {
				// Shrinks below its longest word, which breaks: a cookie or a
				// token has no place to break, and would widen the column past
				// the table.
				ui.Text(c, h.Value).Font(mono).FontFeatures(monoFeatures).FontSize(12).Selectable().Shrink(1).MinWidth(0)
			}
		}).Grow(1)
	}
}

// bodyLangs are what the body can be highlighted as.
var bodyLangs = []string{"JSON", "HTML", "XML"}

var (
	eyeIcon      = icon(`<path d="M1.5 8S4 3.5 8 3.5 14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8z"/><circle cx="8" cy="8" r="2"/>`)
	checkIcon    = icon(`<path d="M3.5 8.5l3 3 6-7"/>`)
	dropdownIcon = icon(`<path d="M4.5 6.5L8 10l3.5-3.5"/>`)
	langIcons    = map[string]*ui.SVG{
		"JSON": icon(`<path d="M5.5 2.5c-1.5 0-2 .5-2 2v1.5c0 1-.5 2-1.5 2 1 0 1.5 1 1.5 2v1.5c0 1.5.5 2 2 2M10.5 2.5c1.5 0 2 .5 2 2v1.5c0 1 .5 2 1.5 2-1 0-1.5 1-1.5 2v1.5c0 1.5-.5 2-2 2"/>`),
		"HTML": icon(`<path d="M5 4.5L1.5 8 5 11.5M11 4.5L14.5 8 11 11.5M9.5 3l-3 10"/>`),
		"XML":  icon(`<path d="M3.5 1.5h6l3 3v10h-9z"/><path d="M9.5 1.5v3h3M6.5 8L5 9.5 6.5 11M9.5 8l1.5 1.5L9.5 11"/>`),
	}
)

// viewMenu picks how the body shows: in a web view, which only a page,
// an image or a PDF has, or as text highlighted as one of bodyLangs.
// Choosing either shows the body.
func (a *app) viewMenu(c *ui.Context, tb *tab) {
	t := c.Theme()
	can := tb.resp.Previewable()
	name := tb.lang
	if name == "" {
		name = "Text"
	}
	m := ui.MenuBase(c, &tb.viewOpen)
	m.Trigger.Gap(5).Padding(3, 6, 3, 8).Radius(6).AlignItems(ui.Center).Border(1, t.Border).
		FontSize(12).TextColor(t.TextMuted).Label("View as")
	if m.Trigger.Hovered() || tb.viewOpen {
		m.Trigger.Background(t.SurfaceHover).TextColor(t.Text)
	}
	m.Trigger.Children(func() {
		if tb.previewOn {
			ui.Icon(c, eyeIcon).Size(14, 14)
		}
		ui.Text(c, name).SingleLine()
		ui.Icon(c, dropdownIcon).Size(12, 12)
	})
	m.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(8).Background(t.Background).Border(1, t.Border).MinWidth(170)
		row := func(item ui.Element) ui.Element {
			item.Gap(8).Padding(6, 8).Radius(5).AlignItems(ui.Center)
			if item.Highlighted() {
				item.Background(t.SurfaceHover)
			}
			return item
		}
		// The menu stays open as Preview goes on and off, as a switch does.
		pv := row(m.CheckItem(tb.previewOn).KeepOpen().Disabled(!can)).Children(func() {
			color := t.Text
			if !can {
				color = t.TextMuted
			}
			ui.Text(c, "Preview").TextColor(color).Grow(1)
			knob := t.TextMuted
			track := t.SurfaceHover
			if tb.previewOn {
				knob, track = t.AccentText, t.Accent
			}
			ui.Row(c).Size(28, 16).Radius(8).Background(track).Padding(2).Children(func() {
				if tb.previewOn {
					ui.Spacer(c)
				}
				ui.Row(c).Size(12, 12).Radius(6).Background(knob)
			})
		})
		if pv.Clicked() {
			tb.previewOn = !tb.previewOn
			tb.respView = "Body"
		}
		ui.Divider(c).Margin(4, 0)
		for _, l := range bodyLangs {
			on := tb.lang == l && !tb.previewOn
			// Labeled apart from the button, which shows the name too.
			item := row(m.RadioItem(tb.lang == l)).Label("View as " + l).Children(func() {
				color := t.Text
				if on {
					color = accentInk(t)
				}
				ui.Icon(c, langIcons[l]).Size(14, 14).TextColor(color)
				ui.Text(c, l).TextColor(color).Grow(1)
				if tb.lang == l {
					ui.Icon(c, checkIcon).Size(14, 14).TextColor(color)
				}
			})
			if item.Clicked() {
				// Choosing the one shown again leaves plain text.
				if on {
					l = ""
				}
				tb.setLang(l)
				tb.previewOn = false
				tb.respView = "Body"
			}
		}
	})
}

// setLang highlights the body as lang, which a large body isn't.
func (tb *tab) setLang(lang string) {
	tb.lang = lang
	if tb.body != nil && len(tb.resp.Body) <= bigBody {
		tb.body.Buffer.SetLanguage(editor.LanguageNamed(lang))
	}
}

// counted is how many rows are on, for a tab's bubble.
func counted(rows []model.KV) int {
	n := 0
	for _, kv := range rows {
		if kv.Enabled && kv.Key != "" {
			n++
		}
	}
	return n
}

var bodyKinds = []string{model.BodyNone, model.BodyJSON, model.BodyText, model.BodyForm, model.BodyMultipart}

func bodyLabel(b *model.Body) string {
	if b.Kind == model.BodyNone {
		return "Body"
	}
	return "Body •"
}

func (tb *tab) draft(name string) *model.KV {
	if tb.drafts == nil {
		tb.drafts = map[string]*model.KV{}
	}
	d := tb.drafts[name]
	if d == nil {
		d = &model.KV{}
		tb.drafts[name] = d
	}
	return d
}

func (a *app) requestPane(c *ui.Context, tb *tab, r *model.Request) {
	switch tb.reqTab {
	case 0:
		if a.kvTable(c, &r.Query, tb.draft("query"), kvOptions{name: "Query"}) {
			r.URLFromQuery()
			a.changed()
		}
	case 1:
		if a.kvTable(c, &r.Path, nil, kvOptions{name: "Path", fixedKeys: true}) {
			a.changed()
		}
	case 2:
		if a.kvTable(c, &r.Headers, tb.draft("headers"), kvOptions{name: "Header", suggestions: commonHeaders}) {
			a.changed()
		}
	case 3:
		a.bodyPane(c, tb, r)
	case 4:
		a.authPane(c, tb, r)
	}
}

func (a *app) bodyPane(c *ui.Context, tb *tab, r *model.Request) {
	t := c.Theme()
	kind := slices.Index(bodyKinds, r.Body.Kind)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 12).Children(func() {
		if ui.Segmented(c, &kind, "None", "JSON", "Text", "Form", "Multipart").Label("Body type").Changed() {
			r.Body.Kind = bodyKinds[kind]
			if tb.bodyEdit != nil {
				setBodyLanguage(tb.bodyEdit, r.Body.Kind)
			}
			a.changed()
		}
		ui.Spacer(c)
		if r.Body.Kind == model.BodyJSON && ui.Button(c, "Format").FontSize(12).Clicked() {
			var out bytes.Buffer
			if err := json.Indent(&out, []byte(r.Body.Text), "", "  "); err != nil {
				c.Toast("Not valid JSON: " + err.Error())
			} else {
				a.bodyEditor(tb, r).Buffer.Set(out.String())
			}
		}
	})
	switch r.Body.Kind {
	case model.BodyNone:
		ui.Text(c, "This request has no body.").FontSize(12).TextColor(t.TextMuted).Padding(4, 12)
	case model.BodyJSON, model.BodyText:
		st := a.bodyEditor(tb, r)
		styleCode(c, st)
		editor.View(c, st).Grow(1).MinHeight(120).FillWidth().Label("Request body")
		if text := st.Buffer.String(); text != r.Body.Text {
			r.Body.Text = text
			a.changed()
		}
	case model.BodyForm, model.BodyMultipart:
		multi := r.Body.Kind == model.BodyMultipart
		if a.kvTable(c, &r.Body.Form, tb.draft("form"), kvOptions{name: "Field", files: multi}) {
			a.changed()
		}
	}
}

// bodyEditor is made from the request's text the first time the body
// shows; from then on the editor's buffer leads and the request follows.
func (a *app) bodyEditor(tb *tab, r *model.Request) *editor.State {
	if tb.bodyEdit == nil {
		tb.bodyEdit = &editor.State{Buffer: editor.NewBuffer(r.Body.Text)}
		setBodyLanguage(tb.bodyEdit, r.Body.Kind)
	}
	return tb.bodyEdit
}

func setBodyLanguage(st *editor.State, kind string) {
	if kind == model.BodyJSON {
		st.Buffer.SetLanguage(editor.LanguageNamed("JSON"))
	} else {
		st.Buffer.SetLanguage(nil)
	}
}

var authKinds = []string{model.AuthNone, model.AuthBasic, model.AuthBearer, model.AuthAPIKey}

func authLabel(au *model.Auth) string {
	if au.Kind == model.AuthNone {
		return "Auth"
	}
	return "Auth •"
}

func (a *app) authPane(c *ui.Context, tb *tab, r *model.Request) {
	t := c.Theme()
	au := &r.Auth
	kind := slices.Index(authKinds, au.Kind)
	changed := false
	ui.Row(c).Padding(8, 12).Children(func() {
		if ui.Segmented(c, &kind, "None", "Basic", "Bearer", "API Key").Label("Auth type").Changed() {
			au.Kind, changed = authKinds[kind], true
		}
	})
	if au.Kind == model.AuthNone {
		ui.Text(c, "This request sends no credentials.").FontSize(12).TextColor(t.TextMuted).Padding(4, 12)
		return
	}
	// A Field's label focuses its input when clicked.
	field := func(label string, value *string, secret bool) ui.Element {
		return ui.Field(c, label, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).MaxWidth(560).Children(func() {
				in := ui.TextInput(c, value).Grow(1).MinWidth(0).Font(mono).FontFeatures(monoFeatures).FontSize(12)
				if secret && !tb.reveal {
					in.Password()
				}
				changed = a.suggestVars(c, in, value, "Auth "+label) || in.Changed() || changed
				// What is sent, the {{name}}s filled.
				if iconButton(c, copyIcon, "Copy "+label).Clicked() {
					c.Services().WriteClipboard(a.env.Expand(*value, nil))
					c.Toast("Copied the " + strings.ToLower(label))
				}
				if !secret {
					return
				}
				show := "Show"
				if tb.reveal {
					show = "Hide"
				}
				if ui.Button(c, show).FontSize(12).MinWidth(64).Shrink(0).Clicked() {
					tb.reveal = !tb.reveal
				}
			})
		})
	}
	ui.Form(c, func() {
		switch au.Kind {
		case model.AuthBasic:
			field("Username", &au.Username, false)
			field("Password", &au.Password, true)
		case model.AuthBearer:
			field("Token", &au.Token, true).Description("Sent as Authorization: Bearer <token>.")
			a.jwtPayload(c, au.Token)
		case model.AuthAPIKey:
			field("Key", &au.Key, false)
			field("Value", &au.Value, true)
			in := 0
			if au.InQuery {
				in = 1
			}
			ui.Field(c, "Add to", func() {
				// In a row, the control keeps its own width rather than the
				// field's.
				ui.Row(c).Children(func() {
					if ui.Segmented(c, &in, "Header", "Query").Changed() {
						au.InQuery, changed = in == 1, true
					}
				})
			})
		}
	}).Padding(4, 12)
	ui.Text(c, "A header or query parameter of the same name set by hand wins.").
		FontSize(12).TextColor(t.TextMuted).Padding(8, 12)
	if changed {
		a.changed()
	}
}

// bodyFileName names a saved body by its type, as response.json.
func bodyFileName(resp *httpx.Response) string {
	ext := ".txt"
	switch {
	case resp.JSON():
		ext = ".json"
	default:
		for _, h := range resp.Headers {
			if !strings.EqualFold(h.Key, "Content-Type") {
				continue
			}
			mt, _, _ := mime.ParseMediaType(h.Value)
			if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
				ext = exts[0]
			}
			if mt == "text/html" {
				ext = ".html"
			}
		}
	}
	return "response" + ext
}
