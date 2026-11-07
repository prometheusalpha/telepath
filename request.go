package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"telepath/internal/httpx"
)

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// tab is an open request: what the workspace doesn't keep, as the last
// response, which lives only while the tab does.
type tab struct {
	id      string
	reqTab  int
	respTab int

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
	req := *n.Request
	ctx, cancel := context.WithCancel(context.Background())
	tb.sending, tb.cancel, tb.err = true, cancel, nil
	seq := tb.seq
	client := a.client
	go func() {
		resp, err := httpx.Send(ctx, client, &req)
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
			ui.Tabs(c, &tb.reqTab, "Params", "Headers", "Body", "Auth").Padding(0, 12)
			emptyState(c, "Nothing here yet", "")
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
