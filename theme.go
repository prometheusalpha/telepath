package main

import "github.com/egoist/mygo/ui"

// Built once: c.Theme() starts every frame as the system's light or dark
// theme, and the view swaps in the matching one of these.
var (
	lightTheme = newTheme(ui.LightTheme())
	darkTheme  = newTheme(ui.DarkTheme())
)

const mono = "monospace"

func newTheme(t *ui.Theme) *ui.Theme {
	// Teal is light enough that white text on it reads poorly in either
	// appearance, so text on the accent stays dark.
	t.Accent = ui.Oklch(0.7, 0.12, 190)
	t.AccentHover = ui.Oklch(0.75, 0.12, 190)
	t.AccentPressed = ui.Oklch(0.64, 0.12, 190)
	t.AccentText = ui.Hex("#0b1a1c")
	t.Focus = ui.Oklch(0.7, 0.12, 190).Alpha(0.6)
	t.Selection = ui.Oklch(0.7, 0.12, 190).Alpha(0.3)
	t.Radius = 7
	if t.Dark {
		// Sidebar sits a step darker than the content card.
		t.Background = ui.Hex("#1c1d21")
		t.Surface = ui.Hex("#26272c")
		t.SurfaceHover = ui.Hex("#2f3036")
		t.SurfacePressed = ui.Hex("#393a41")
		t.Border = ui.Hex("#34353b")
	}
	return t
}

func applyTheme(c *ui.Context) {
	if c.Theme().Dark {
		c.SetTheme(darkTheme)
	} else {
		c.SetTheme(lightTheme)
	}
}

// sidebarColor is the sidebar's fill where no window material shows.
func sidebarColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#141518")
	}
	return ui.Hex("#eeeef1")
}

// methodColor tints a method's badge; lightness differs per appearance so
// each stays readable on the card.
func methodColor(t *ui.Theme, method string) ui.Color {
	l := float32(0.55)
	if t.Dark {
		l = 0.75
	}
	switch method {
	case "GET":
		return ui.Oklch(l, 0.15, 145)
	case "POST":
		return ui.Oklch(l, 0.14, 80)
	case "PUT":
		return ui.Oklch(l, 0.14, 250)
	case "PATCH":
		return ui.Oklch(l, 0.15, 300)
	case "DELETE":
		return ui.Oklch(l, 0.17, 25)
	}
	return t.TextMuted
}
