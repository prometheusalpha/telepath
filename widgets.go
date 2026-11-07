package main

import "github.com/egoist/mygo/ui"

func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	plusIcon       = icon(`<path d="M8 3v10M3 8h10"/>`)
	moreIcon       = icon(`<circle cx="3.5" cy="8" r=".6" fill="currentColor"/><circle cx="8" cy="8" r=".6" fill="currentColor"/><circle cx="12.5" cy="8" r=".6" fill="currentColor"/>`)
	searchIcon     = icon(`<circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5l3 3"/>`)
	copyIcon       = icon(`<rect x="5.5" y="5.5" width="8" height="8" rx="1.5"/><path d="M10.5 5.5V4a1.5 1.5 0 0 0-1.5-1.5H4A1.5 1.5 0 0 0 2.5 4v5A1.5 1.5 0 0 0 4 10.5h1.5"/>`)
	downloadIcon   = icon(`<path d="M8 2.5v8M4.5 7.5L8 11l3.5-3.5M3 13.5h10"/>`)
	splitRightIcon = icon(`<rect x="1.5" y="2.5" width="13" height="11" rx="2"/><path d="M8.5 2.5v11"/>`)
	splitBelowIcon = icon(`<rect x="1.5" y="2.5" width="13" height="11" rx="2"/><path d="M1.5 8.5h13"/>`)
	sendIcon       = icon(`<path d="M2.5 8h10M9 4.5L12.5 8 9 11.5"/>`)
	locateIcon     = icon(`<circle cx="8" cy="8" r="4.5"/><circle cx="8" cy="8" r="1.2" fill="currentColor"/><path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2"/>`)
)

// methodBadge shortens the long methods so badges keep one width in the
// sidebar and the tabs.
func methodBadge(c *ui.Context, method string) ui.Element {
	label := method
	switch method {
	case "DELETE":
		label = "DEL"
	case "OPTIONS":
		label = "OPT"
	case "PATCH":
		label = "PAT"
	}
	t := c.Theme()
	return ui.Text(c, label).Font(mono).FontFeatures(monoFeatures).FontSize(10).FontWeight(700).
		TextColor(methodColor(t, method)).Width(28).SingleLine()
}

// iconButton is a quiet square button; label names it for the tooltip and
// for assistive technology, since it shows no text.
func iconButton(c *ui.Context, svg *ui.SVG, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Size(26, 26).Radius(6).Center().
		TextColor(t.TextMuted).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(t.SurfaceHover).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, svg).Size(15, 15) })
	return b
}

func emptyState(c *ui.Context, title, hint string) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(6).Padding(24).Children(func() {
		ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(t.TextMuted)
		if hint != "" {
			ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted).Opacity(0.8)
		}
	})
}
