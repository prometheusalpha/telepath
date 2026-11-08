package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"telepath/internal/model"
)

// Built once: c.Theme() starts every frame as the system's light or dark
// theme, and the view swaps in the matching one of these.
var (
	lightTheme = newTheme(ui.LightTheme())
	darkTheme  = newTheme(ui.DarkTheme())
	lightCode  = newCodeTheme(lightTheme)
	darkCode   = newCodeTheme(darkTheme)
	lightSplit = lineless(lightTheme)
	darkSplit  = lineless(darkTheme)
)

// mono is the family of code, URLs and key/value text, monoFeatures the
// OpenType features it shows with, and codeSize the size of the request
// and response bodies. Like the themes they are read
// everywhere, so setMono changes them in place rather than passing them
// down.
var (
	mono         = builtinMono + ", monospace"
	monoFeatures = builtinFeatures
	codeSize     = defaultCodeSize
)

// builtinFeatures are those of the user's Ghostty config, for JetBrains
// Mono alone: the stylistic sets of another family mean other glyphs.
const builtinFeatures = "ss01,ss02,ss03,ss04,ss05,ss06,ss07,ss08,calt,cv09,dlig,pnum,lnum,case"

const defaultCodeSize float32 = 13

// setMono applies the workspace's font settings; "" and 0 are the
// defaults. The system's monospaced font follows, for what a chosen family
// lacks.
func setMono(ws *model.Workspace) {
	mono, monoFeatures = builtinMono, builtinFeatures
	if ws.MonoFont != "" {
		mono, monoFeatures = ws.MonoFont+", "+builtinMono, ""
	}
	mono += ", monospace"
	codeSize = defaultCodeSize
	if ws.MonoSize > 0 {
		codeSize = ws.MonoSize
	}
}

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

// lineless is t for the sidebar's Split alone, which draws its divider in
// t.Border: a straight line beside the card's rounded corners. The divider
// still shows in the accent while hovered or dragged.
func lineless(t *ui.Theme) *ui.Theme {
	s := *t
	s.Border = ui.Transparent
	return &s
}

type themeChoice struct {
	theme, label string
	icon         *ui.SVG
}

var themes = []themeChoice{
	{model.ThemeSystem, "System", icon(`<circle cx="8" cy="8" r="5.5"/><path d="M8 2.5v11a5.5 5.5 0 0 0 0-11z" fill="currentColor"/>`)},
	{model.ThemeLight, "Light", icon(`<circle cx="8" cy="8" r="2.8"/><path d="M8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1 1M11.6 11.6l1 1M3.4 12.6l1-1M11.6 4.4l1-1"/>`)},
	{model.ThemeDark, "Dark", icon(`<path d="M13 9.5A5.5 5.5 0 0 1 6.5 3a5.5 5.5 0 1 0 6.5 6.5z"/>`)},
}

func (a *app) setTheme(theme string) {
	a.ws.Theme = theme
	if a.setAppearance != nil {
		a.setAppearance(theme)
	}
	a.changed()
}

// nextTheme is the theme the sidebar's button goes to, round the three.
func nextTheme(theme string) string {
	for i, th := range themes {
		if th.theme == theme {
			return themes[(i+1)%len(themes)].theme
		}
	}
	return model.ThemeSystem
}

func themeOf(theme string) themeChoice {
	for _, th := range themes {
		if th.theme == theme {
			return th
		}
	}
	return themes[0]
}

// newCodeTheme gives each kind of JSON token a hue of its own: the
// editor's default has keys, numbers and true/false/null in one blue, and
// strings in a paler one.
func newCodeTheme(t *ui.Theme) *editor.Theme {
	ct := editor.DefaultTheme(t)
	l, c := float32(0.5), float32(0.14)
	if t.Dark {
		l, c = 0.8, 0.11
	}
	tok := func(k editor.TokenKind, color ui.Color) { ct.Tokens[k] = editor.TokenStyle{Color: color} }
	tok(editor.TokenProperty, ui.Oklch(l, c, 250))
	tok(editor.TokenString, ui.Oklch(l, c, 145))
	tok(editor.TokenNumber, ui.Oklch(l, c, 55))
	tok(editor.TokenConstant, ui.Oklch(l, c, 310))
	tok(editor.TokenKeyword, ui.Oklch(l, c, 310))
	tok(editor.TokenEscape, ui.Oklch(l, c, 200))
	tok(editor.TokenOperator, t.TextMuted)
	tok(editor.TokenPunctuation, t.TextMuted)
	ct.Cursor = accentInk(t)
	return ct
}

// styleCode gives an editor the theme of the window's appearance and the
// font of the settings, every frame, as both can change.
func styleCode(c *ui.Context, st *editor.State) {
	st.Theme = lightCode
	if c.Theme().Dark {
		st.Theme = darkCode
	}
	st.Font = ui.Font{Family: mono, Size: codeSize, Features: monoFeatures}
}

func applyTheme(c *ui.Context) {
	if c.Theme().Dark {
		c.SetTheme(darkTheme)
	} else {
		c.SetTheme(lightTheme)
	}
}

// accentInk is the accent for text and icons on the card: the light
// theme's teal is a fill color, too pale to read on white.
func accentInk(t *ui.Theme) ui.Color {
	if t.Dark {
		return t.Accent
	}
	return ui.Oklch(0.5, 0.1, 190)
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
