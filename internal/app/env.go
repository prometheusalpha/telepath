package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"telepath/internal/vars"
)

var (
	braceIcon  = icon(`<path d="M5.5 2.5c-1.5 0-2 .7-2 2v1.6c0 .9-.4 1.4-1.5 1.9 1.1.5 1.5 1 1.5 1.9v1.6c0 1.3.5 2 2 2M10.5 2.5c1.5 0 2 .7 2 2v1.6c0 .9.4 1.4 1.5 1.9-1.1.5-1.5 1-1.5 1.9v1.6c0 1.3-.5 2-2 2"/>`)
	lockIcon   = icon(`<rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"/>`)
	unlockIcon = icon(`<rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 4.9-.7"/>`)
)

// highlightVars colors the {{name}}s of an input's text: the accent for
// those with a value, the danger color for the others. It reports whether
// the text has any.
func (a *app) highlightVars(c *ui.Context, in ui.Element, text string) bool {
	t := c.Theme()
	refs := vars.Refs(text)
	for _, r := range refs {
		color := t.Danger
		if _, ok := a.env[r.Name]; ok {
			color = accentInk(t)
		}
		in.TextRanges(ui.TextRange{Start: r.Start, End: r.End, Color: color, Weight: 600})
	}
	return len(refs) > 0
}

func (a *app) sidebarFooter(c *ui.Context) {
	ui.Column(c).Shrink(0).Padding(8).Gap(2).Children(func() {
		th := themeOf(a.ws.Theme)
		if footerButton(c, th.icon, "Appearance", th.label).Label("Appearance: " + th.label).Clicked() {
			a.setTheme(nextTheme(a.ws.Theme))
		}
		count := ""
		if n := len(a.env); n > 0 {
			count = fmt.Sprint(n)
		}
		if footerButton(c, braceIcon, "Variables", count).Tooltip(a.hint("", "variables")).Clicked() {
			a.envOpen = true
		}
		if footerButton(c, gearIcon, "Settings", "").Tooltip(a.hint("", "settings")).Clicked() {
			a.openSettings()
		}
	})
}

// footerButton is a row of the sidebar's footer: an icon, a name, and on
// the right what it is set to.
func footerButton(c *ui.Context, svg *ui.SVG, name, value string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Gap(8).AlignItems(ui.Center).Padding(6, 8).Radius(7).
		TextColor(t.TextMuted).Label(name)
	if b.Hovered() {
		b.Background(t.SurfaceHover.Alpha(0.6)).TextColor(t.Text)
	}
	b.Children(func() {
		ui.Icon(c, svg).Size(14, 14)
		ui.Text(c, name).FontSize(13)
		ui.Spacer(c)
		if value != "" {
			ui.Text(c, value).FontSize(11).TextColor(t.TextMuted)
		}
	})
	return b
}

func (a *app) envModal(c *ui.Context) {
	t := c.Theme()
	w, h := c.Size()
	ui.Modal(c, &a.envOpen, func() {
		ui.Column(c).Width(min(720, w-80)).Gap(4).Children(func() {
			ui.Text(c, "Variables").FontSize(16).FontWeight(600)
			ui.Text(c, "Use one anywhere in a request as {{name}}.").FontSize(12).TextColor(t.TextMuted)
			ui.Scroll(c).MaxHeight(h*0.6).Margin(4, -12, 0).Children(func() {
				if a.kvTable(c, &a.ws.Env, &a.envDraft, kvOptions{name: "Variable", secrets: true}) {
					a.changed()
				}
			})
			ui.Row(c).Justify(ui.End).Padding(8, 0, 0).Children(func() {
				if ui.PrimaryButton(c, "Done").Clicked() {
					a.envOpen = false
				}
			})
		})
	})
}
