package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"telepath/internal/httpx"
	"telepath/internal/model"
)

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// tab is an open request: what the workspace doesn't keep, as the last
// response, which lives only while the tab does.
type tab struct {
	id      string
	reqTab  int
	respTab int

	// drafts are the blank rows of the tables, by table.
	drafts   map[string]*model.KV
	bodyEdit *editor.State
	reveal   bool // shows the password, token or key typed in the Auth tab

	sending bool
	cancel  context.CancelFunc
	// seq tells a response apart from those of sends canceled or replaced
	// since, which may still arrive.
	seq  int
	resp *httpx.Response
	err  error
	body *editor.State
}

func (t *tab) stop() {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.sending = false
	t.seq++
}

func (a *app) send(tb *tab) {
	n := a.tree.byID[tb.id]
	if n == nil || n.Request == nil {
		return
	}
	tb.stop()
	// The goroutine reads a copy: the user may keep editing meanwhile.
	req := n.Request.Clone()
	ctx, cancel := context.WithCancel(context.Background())
	tb.sending, tb.cancel, tb.err = true, cancel, nil
	seq := tb.seq
	client := a.client
	go func() {
		resp, err := httpx.Send(ctx, client, req)
		a.update(func() {
			if tb.seq != seq {
				return
			}
			tb.sending, tb.cancel = false, nil
			tb.resp, tb.err = resp, err
			if resp != nil {
				buf := editor.NewBuffer(resp.Pretty())
				if resp.JSON() {
					buf.SetLanguage(editor.LanguageNamed("JSON"))
				}
				tb.body = &editor.State{Buffer: buf, ReadOnly: true, SoftWrap: true, NoScrollPastEnd: true}
			}
		})
	}()
}

func (a *app) cancelSend(tb *tab) {
	tb.stop()
	tb.err = context.Canceled
}

func (a *app) requestView(c *ui.Context, tb *tab) {
	t := c.Theme()
	n := a.tree.byID[tb.id]
	r := n.Request
	if c.Shortcut(ui.Cmd, ui.KeyEnter) {
		a.send(tb)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(10, 12).Shrink(0).Children(func() {
		m := ui.Select(c, &r.Method, methods).Width(104).Font(mono).FontWeight(700).
			TextColor(methodColor(t, r.Method)).Label("Method")
		if m.Changed() {
			a.changed()
		}
		url := ui.TextInput(c, &r.URL).Grow(1).MinWidth(0).Font(mono).
			Placeholder("https://api.example.com/users").Label("URL")
		if url.Changed() {
			r.QueryFromURL()
			r.PathFromURL()
			a.changed()
		}
		if url.Submitted() {
			a.send(tb)
		}
		if tb.sending {
			if ui.Button(c, "Cancel").Width(84).Clicked() {
				a.cancelSend(tb)
			}
		} else if ui.PrimaryButton(c, "Send").Width(84).Tooltip("⌘↵").Clicked() {
			a.send(tb)
		}
	})
	ui.SplitVerticalPercent(c, &a.split, func() {
		ui.Column(c).Fill().Children(func() {
			ui.Tabs(c, &tb.reqTab, counted("Params", r.Query), counted("Path", r.Path),
				counted("Headers", r.Headers), bodyLabel(&r.Body), authLabel(&r.Auth)).Padding(0, 12)
			ui.Scroll(c).Grow(1).Children(func() { a.requestPane(c, tb, r) })
		})
	}, func() {
		a.responseView(c, tb)
	}).SplitReset(45).Grow(1)
}

func (a *app) responseView(c *ui.Context, tb *tab) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		switch {
		case tb.sending:
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "Sending…").TextColor(t.TextMuted)
			})
		case errors.Is(tb.err, context.Canceled):
			emptyState(c, "Request canceled", "")
		case tb.err != nil:
			ui.Column(c).Grow(1).Center().Gap(6).Padding(24).Children(func() {
				ui.Text(c, "Couldn't send the request").FontWeight(600).TextColor(t.Danger)
				ui.Text(c, tb.err.Error()).FontSize(12).Font(mono).TextColor(t.TextMuted).Selectable()
			})
		case tb.resp == nil:
			emptyState(c, "No response yet", "Press Send, or ⌘↵")
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
		return t.Accent
	}
	return t.Success
}

func (a *app) responseBody(c *ui.Context, tb *tab) {
	t := c.Theme()
	resp := tb.resp
	ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(6, 12).Shrink(0).Children(func() {
		ui.Text(c, resp.Status).Font(mono).FontWeight(700).FontSize(12).TextColor(statusColor(t, resp.StatusCode))
		ui.Text(c, httpx.FormatDuration(resp.Duration)).FontSize(12).TextColor(t.TextMuted)
		size := httpx.FormatSize(resp.Size)
		if resp.Truncated {
			size += " (showing the first " + httpx.FormatSize(httpx.MaxBody) + ")"
		}
		ui.Text(c, size).FontSize(12).TextColor(t.TextMuted)
		ui.Spacer(c)
		ui.Tabs(c, &tb.respTab, "Body", fmt.Sprintf("Headers %d", len(resp.Headers)))
	})
	ui.Divider(c)
	switch tb.respTab {
	case 0:
		if len(resp.Body) == 0 {
			emptyState(c, "Empty body", "")
			return
		}
		editor.View(c, tb.body).Grow(1).FillWidth().Label("Response body")
	case 1:
		cols := []ui.TableColumn{{Title: "Name", Width: 220}, {Title: "Value"}}
		ui.Table(c, nil, cols, len(resp.Headers), func(row, col int) {
			h := resp.Headers[row]
			if col == 0 {
				ui.Text(c, h.Key).Font(mono).FontSize(12).SingleLine()
			} else {
				ui.Text(c, h.Value).Font(mono).FontSize(12).Selectable()
			}
		}).Grow(1)
	}
}

// counted adds how many rows are on to a tab's label, as "Headers 2".
func counted(label string, rows []model.KV) string {
	n := 0
	for _, kv := range rows {
		if kv.Enabled && kv.Key != "" {
			n++
		}
	}
	if n == 0 {
		return label
	}
	return fmt.Sprintf("%s %d", label, n)
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
		editor.View(c, st).Height(260).FillWidth().Label("Request body")
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
		tb.bodyEdit = &editor.State{Buffer: editor.NewBuffer(r.Body.Text), NoScrollPastEnd: true}
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
				in := ui.TextInput(c, value).Grow(1).MinWidth(0).Font(mono).FontSize(12)
				if secret && !tb.reveal {
					in.Password()
				}
				changed = in.Changed() || changed
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
