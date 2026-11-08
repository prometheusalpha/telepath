package app

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
	"telepath/internal/vars"
)

var (
	braceIcon  = icon(`<path d="M5.5 2.5c-1.5 0-2 .7-2 2v1.6c0 .9-.4 1.4-1.5 1.9 1.1.5 1.5 1 1.5 1.9v1.6c0 1.3.5 2 2 2M10.5 2.5c1.5 0 2 .7 2 2v1.6c0 .9.4 1.4 1.5 1.9-1.1.5-1.5 1-1.5 1.9v1.6c0 1.3-.5 2-2 2"/>`)
	lockIcon   = icon(`<rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"/>`)
	unlockIcon = icon(`<rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 4.9-.7"/>`)
)

// highlightVars colors the {{name}}s of an input's text: the accent for
// those with a value, the danger color for the others. The one the pointer
// is over shows its value in a card under it. size is the input's font
// size, label one no other input has. It reports whether the text has any.
func (a *app) highlightVars(c *ui.Context, in ui.Element, text, label string, size float32) bool {
	t := c.Theme()
	refs := vars.Refs(text)
	for _, r := range refs {
		color := t.Danger
		if _, ok := a.env[r.Name]; ok {
			color = accentInk(t)
		}
		in.TextRanges(ui.TextRange{Start: r.Start, End: r.End, Color: color, Weight: 600})
	}
	if len(refs) > 0 || a.tip.label == label {
		a.varTip(c, in, text, refs, label, size)
	}
	return len(refs) > 0
}

// varTip is the variable whose {{name}} the pointer is over; one at a
// time, as there is one pointer.
type varTip struct {
	label, name string
	x           float32 // where the reference starts, from the input's left
}

func (a *app) varTip(c *ui.Context, in ui.Element, text string, refs []vars.Ref, label string, size float32) {
	tip := &a.tip
	r, x, onRef := refAt(c, in, text, refs, size)
	if onRef {
		if tip.label != label || tip.name != r.Name {
			*tip = varTip{label: label, name: r.Name}
		}
		tip.x = x
	}
	if tip.label != label {
		return
	}
	t := c.Theme()
	value, ok := a.env[tip.name]
	secret := slices.ContainsFunc(a.ws.Env, func(kv model.KV) bool { return kv.Enabled && kv.Key == tip.name && kv.Secret })
	ui.Overlay(c, func() {
		// The card's padding on top bridges the gap from the input, so the
		// pointer going down to it doesn't close it.
		card := ui.Column(c).AttachTo(in, ui.AnchorBottomLeft, ui.AnchorTopLeft).Margin(0, 0, 0, tip.x).Padding(4, 0, 0, 0)
		card.Children(func() {
			ui.Column(c).MinWidth(200).MaxWidth(420).Padding(8, 10).Gap(4).Radius(8).
				Background(t.Background).Border(1, t.Border).Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.18)).
				Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, tip.name).Font(mono).FontFeatures(monoFeatures).FontSize(12).FontWeight(600).
							SingleLine().MinWidth(0).Shrink(1)
						ui.Spacer(c)
						if ok && iconButton(c, copyIcon, "Copy value").Clicked() {
							c.Services().WriteClipboard(value)
							c.Toast("Copied " + tip.name)
						}
					})
					switch {
					case !ok:
						ui.Text(c, "Not set: sent as typed.").FontSize(12).TextColor(t.Danger)
					case secret:
						ui.Text(c, "••••••").Font(mono).FontFeatures(monoFeatures).FontSize(12).TextColor(t.TextMuted)
					case value == "":
						ui.Text(c, "Empty").FontSize(12).TextColor(t.TextMuted)
					default:
						ui.Text(c, value).Font(mono).FontFeatures(monoFeatures).FontSize(12).MaxLines(6).Selectable()
					}
				})
		})
		// The pointer leaves the reference for the card to reach the copy
		// button: the card is where it last was, so whether the pointer is
		// over it is known only once built.
		if _, _, onCard := card.PointerPosition(); !onRef && !onCard {
			*tip = varTip{}
			c.Invalidate()
		}
	})
}

// refAt finds the reference under the pointer, and where it starts. The
// input doesn't tell which rune is under a point, so the text is measured
// as the input lays it out: after its padding and border, and from its
// start, which is where it shows unless it has the focus and scrolled to
// the caret.
func refAt(c *ui.Context, in ui.Element, text string, refs []vars.Ref, size float32) (vars.Ref, float32, bool) {
	px, _, over := in.PointerPosition()
	if !over || len(refs) == 0 {
		return vars.Ref{}, 0, false
	}
	runes := []rune(text)
	left := c.Theme().Space(2.5) + 1
	// width measures the first n runes, the references bold as highlighted.
	width := func(n int) float32 {
		var spans []ui.Span
		at := 0
		add := func(to, weight int) {
			if to > at {
				spans = append(spans, ui.Span{Text: string(runes[at:to]), Font: mono, Features: monoFeatures, Size: size, Weight: weight})
				at = to
			}
		}
		for _, r := range refs {
			if r.Start >= n {
				break
			}
			add(r.Start, 0)
			add(min(r.End, n), 600)
		}
		add(n, 0)
		w, _ := c.MeasureText(0, spans...)
		return w
	}
	if in.Focused() && width(len(runes)) > in.Bounds().W-2*left-1 {
		return vars.Ref{}, 0, false
	}
	for _, r := range refs {
		if x := left + width(r.Start); px >= x && px < left+width(r.End) {
			return r, x, true
		}
	}
	return vars.Ref{}, 0, false
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
