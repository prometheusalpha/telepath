package app

import (
	"slices"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

// suggestion is the list shown under the input being typed in: the
// variables, as a {{ is opened in it, then the names the input may take,
// as a header key's; one input at a time has the focus, so the app keeps
// one.
type suggestion struct {
	label       string // the input's, which tells inputs apart
	open, caret int    // runes: where the {{ starts, and the caret after the name begun
	length      int    // the text's runes, to follow typing between frames
	items       []choice
	// sel is -1 until Down when the list starts with names: Enter after
	// a name typed in full would take a longer one.
	sel int
}

type choice struct {
	name     string
	variable bool // put in as {{name}}, where the other names take the whole text
}

// dismissal is a {{ Escape closed the list of: it stays closed until the
// name typed after it ends.
type dismissal struct {
	label string
	open  int
}

// varQuery finds the variable name being typed before caret: after a
// {{ that isn't closed yet, made of the runes names have.
func varQuery(text []rune, caret int) (open int, name string, ok bool) {
	for i := caret - 1; i >= 0; i-- {
		r := text[i]
		if r == '{' && i > 0 && text[i-1] == '{' {
			return i - 1, string(text[i+1 : caret]), true
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.') {
			return 0, "", false
		}
	}
	return 0, "", false
}

// matchVars lists the variables whose names hold q, those starting with
// it first, each group in the table's order.
func matchVars(env []model.KV, q string) []string {
	var starts, holds []string
	lq := strings.ToLower(q)
	for _, kv := range env {
		k := strings.ToLower(kv.Key)
		switch {
		case !kv.Enabled || kv.Key == "" || slices.Contains(starts, kv.Key) || slices.Contains(holds, kv.Key):
		case strings.HasPrefix(k, lq):
			starts = append(starts, kv.Key)
		case strings.Contains(k, lq):
			holds = append(holds, kv.Key)
		}
	}
	return append(starts, holds...)
}

// insertVar puts {{name}} in place of the {{ and the name begun, taking
// the }} already after the caret, and returns where the caret goes.
func insertVar(text []rune, open, caret int, name string) (string, int) {
	rest := string(text[caret:])
	rest = strings.TrimPrefix(rest, "}}")
	ref := "{{" + name + "}}"
	return string(text[:open]) + ref + rest, open + len([]rune(ref))
}

// matchNames lists the names holding what was typed, those starting
// with it first, but not the name typed itself.
func matchNames(names []string, typed string) []string {
	var starts, holds []string
	q := strings.ToLower(strings.TrimSpace(typed))
	for _, n := range names {
		switch l := strings.ToLower(n); {
		case q == "" || l == q:
		case strings.HasPrefix(l, q):
			starts = append(starts, n)
		case strings.Contains(l, q):
			holds = append(holds, n)
		}
	}
	return append(starts, holds...)
}

// suggestVars shows the variables matching what is typed after a {{ in
// an input built just before, bound to value, and takes the keys that
// choose one. It reports that one was put in the text, which the input's
// Changed doesn't tell.
func (a *app) suggestVars(c *ui.Context, in ui.Element, value *string, label string) bool {
	return a.suggestIn(c, in, value, label, nil)
}

// suggestKey is suggestVars for a key input, listing after the variables
// those of names holding what was typed in it.
func (a *app) suggestKey(c *ui.Context, in ui.Element, value *string, label string, names []string) bool {
	return a.suggestIn(c, in, value, label, names)
}

func (a *app) suggestIn(c *ui.Context, in ui.Element, value *string, label string, names []string) bool {
	done := a.suggested == label
	if done {
		a.suggested = ""
	}
	if a.caretLabel == label {
		in.SetTextSelection(a.caretAt, a.caretAt)
		a.caretLabel = ""
	}
	s := &a.suggest
	hide := func() {
		if s.label == label {
			*s = suggestion{}
		}
	}
	// Names show once typed, as an autocomplete's, not as the input is
	// clicked. What was typed is kept rather than which input: the blank
	// row's input is another row's once typed in.
	if names != nil && in.Changed() {
		a.typedKey = *value
	}
	start, caret := in.TextSelection()
	text := []rune(*value)
	open, q, ok := varQuery(text, caret)
	if !ok && a.dismissed.label == label {
		a.dismissed = dismissal{}
	}
	var items []choice
	if in.Focused() && start == caret {
		if ok && a.dismissed != (dismissal{label, open}) {
			for _, name := range matchVars(a.ws.Env, q) {
				items = append(items, choice{name: name, variable: true})
			}
		}
		if names != nil && *value == a.typedKey {
			for _, name := range matchNames(names, *value) {
				items = append(items, choice{name: name})
			}
		}
	}
	if len(items) == 0 {
		hide()
		return done
	}
	if s.label != label || s.open != open || !slices.Equal(s.items, items) {
		sel := -1
		if items[0].variable {
			sel = 0
		}
		*s = suggestion{label: label, open: open, items: items, sel: sel}
	}
	s.caret, s.length = caret, len(text)

	in.OnTextKey(func(k ui.TextKey) bool {
		if s.label != label || k.Mods != 0 {
			return false
		}
		switch k.Key {
		case ui.KeyDown:
			s.sel = min(s.sel+1, len(s.items)-1)
		case ui.KeyUp:
			s.sel = max(s.sel-1, 0)
		case ui.KeyEnter, ui.KeyTab:
			if s.sel < 0 {
				return false
			}
			if it := s.items[s.sel]; !it.variable {
				k.SetText(it.name)
				a.caretLabel, a.caretAt, a.suggested = label, len([]rune(it.name)), label
				a.typedKey = ""
				*s = suggestion{}
				return true
			}
			// The text may be ahead of the frame that found the {{: what was
			// typed since went in at the caret.
			now := []rune(k.Text())
			caret := s.caret + len(now) - s.length
			if caret < s.open+2 || caret > len(now) || string(now[s.open:s.open+2]) != "{{" {
				return false
			}
			text, at := insertVar(now, s.open, caret, s.items[s.sel].name)
			k.SetText(text)
			a.caretLabel, a.caretAt, a.suggested = label, at, label
			*s = suggestion{}
		case ui.KeyEscape:
			if s.items[0].variable {
				a.dismissed = dismissal{label, s.open}
			}
			a.typedKey = ""
			*s = suggestion{}
		default:
			return false
		}
		return true
	})

	t := c.Theme()
	chosen := -1
	ui.Overlay(c, func() {
		ui.Column(c).AttachTo(in, ui.AnchorBottomLeft, ui.AnchorTopLeft).Margin(4, 0, 0, 0).
			MinWidth(240).MaxWidth(420).Padding(4).Gap(1).Radius(8).
			Background(t.Background).Border(1, t.Border).Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.18)).
			Children(func() {
				for i, it := range s.items {
					if i > 0 && !it.variable && s.items[i-1].variable {
						ui.Divider(c).Margin(3, 0)
					}
					label := "Variable " + it.name
					if !it.variable {
						label = "Name " + it.name
					}
					// KeepFocus leaves the keyboard in the input clicked from.
					row := ui.ButtonBase(c.Key(label)).KeepFocus().Gap(10).AlignItems(ui.Center).
						Padding(4, 8).Radius(5).Label(label)
					if i == s.sel {
						row.Background(t.Accent).TextColor(t.AccentText)
					} else if row.Hovered() {
						row.Background(t.SurfaceHover)
					}
					row.Children(func() {
						ui.Text(c, it.name).Font(mono).FontFeatures(monoFeatures).FontSize(12).SingleLine()
						ui.Spacer(c)
						if !it.variable {
							return
						}
						v := ui.Text(c, a.varPreview(it.name)).Font(mono).FontFeatures(monoFeatures).FontSize(11).SingleLine().MinWidth(0).Shrink(1)
						if i != s.sel {
							v.TextColor(t.TextMuted)
						}
					})
					if row.Clicked() {
						chosen = i
					}
				}
			})
	})
	if chosen >= 0 && !s.items[chosen].variable {
		*value = s.items[chosen].name
		a.caretLabel, a.caretAt = label, len([]rune(*value))
		a.typedKey = ""
		*s = suggestion{}
		return true
	}
	if chosen >= 0 {
		text, at := insertVar(text, open, caret, s.items[chosen].name)
		*value = text
		a.caretLabel, a.caretAt = label, at
		*s = suggestion{}
		return true
	}
	return done
}

// varPreview is a variable's value as the list shows it, hidden when
// secret.
func (a *app) varPreview(name string) string {
	for _, kv := range a.ws.Env {
		if kv.Key == name && kv.Enabled {
			if kv.Secret {
				return "••••••"
			}
			return kv.Value
		}
	}
	return ""
}
