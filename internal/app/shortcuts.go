package app

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

// keys is a shortcut, a key with the modifiers held; the zero value is
// none.
type keys struct {
	mods ui.Modifiers
	key  ui.Key
}

// keyName is a key a shortcut may use, by its name in menus and the
// workspace file, and as macOS shows it when that differs.
type keyName struct {
	key        ui.Key
	name, show string
}

var keyNames = func() []keyName {
	ks := []keyName{
		{ui.KeyMinus, "-", ""}, {ui.KeyEqual, "=", ""}, {ui.KeyComma, ",", ""}, {ui.KeyPeriod, ".", ""},
		{ui.KeySlash, "/", ""}, {ui.KeySemicolon, ";", ""}, {ui.KeyQuote, "'", ""}, {ui.KeyBracketLeft, "[", ""},
		{ui.KeyBracketRight, "]", ""}, {ui.KeyBackslash, `\`, ""}, {ui.KeyBackquote, "`", ""},
		{ui.KeyEnter, "Enter", "↩"}, {ui.KeyTab, "Tab", "⇥"}, {ui.KeySpace, "Space", ""},
		{ui.KeyBackspace, "Backspace", "⌫"}, {ui.KeyDelete, "Delete", "⌦"},
		{ui.KeyUp, "Up", "↑"}, {ui.KeyDown, "Down", "↓"}, {ui.KeyLeft, "Left", "←"}, {ui.KeyRight, "Right", "→"},
		{ui.KeyHome, "Home", "↖"}, {ui.KeyEnd, "End", "↘"}, {ui.KeyPageUp, "PageUp", "⇞"}, {ui.KeyPageDown, "PageDown", "⇟"},
	}
	for k := ui.KeyA; k <= ui.KeyZ; k++ {
		ks = append(ks, keyName{k, string(rune('A' + k - ui.KeyA)), ""})
	}
	for k := ui.Key0; k <= ui.Key9; k++ {
		ks = append(ks, keyName{k, string(rune('0' + k - ui.Key0)), ""})
	}
	for k := ui.KeyF1; k <= ui.KeyF12; k++ {
		ks = append(ks, keyName{k, "F" + strconv.Itoa(int(k-ui.KeyF1)+1), ""})
	}
	return ks
}()

type modName struct {
	mod        ui.Modifiers
	name, show string
}

// In the order of macOS's menus.
var modNames = []modName{{ui.Ctrl, "Ctrl", "⌃"}, {ui.Alt, "Alt", "⌥"}, {ui.Shift, "Shift", "⇧"}, {ui.Super, "Cmd", "⌘"}}

func (k keys) none() bool { return k.key == 0 }

// accel writes k as menus take it and the workspace keeps it, as
// "Shift+Cmd+N"; "" for none.
func (k keys) accel() string { return k.format(false) }

// String shows k as macOS does, as "⇧⌘N".
func (k keys) String() string { return k.format(true) }

func (k keys) format(shown bool) string {
	i := slices.IndexFunc(keyNames, func(n keyName) bool { return n.key == k.key })
	if i < 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range modNames {
		switch {
		case k.mods&m.mod == 0:
		case shown:
			b.WriteString(m.show)
		default:
			b.WriteString(m.name + "+")
		}
	}
	if n := keyNames[i]; shown && n.show != "" {
		return b.String() + n.show
	}
	return b.String() + keyNames[i].name
}

// parseKeys reads what accel writes.
func parseKeys(s string) (keys, bool) {
	var k keys
	parts := strings.Split(s, "+")
	for _, p := range parts[:len(parts)-1] {
		i := slices.IndexFunc(modNames, func(m modName) bool { return m.name == p })
		if i < 0 {
			return keys{}, false
		}
		k.mods |= modNames[i].mod
	}
	i := slices.IndexFunc(keyNames, func(n keyName) bool { return n.name == parts[len(parts)-1] })
	if i < 0 {
		return keys{}, false
	}
	k.key = keyNames[i].key
	return k, true
}

// command is something the app does that a shortcut can be set for, in
// the order Settings lists them.
type command struct {
	id, label string
	def       keys
	// run does it from the menu or the keys; nil for what only the view
	// can do, which it checks the keys for itself.
	run func(a *app)
}

var commands = []command{
	{"new-request", "New Request", keys{ui.Cmd, ui.KeyN}, func(a *app) { a.newNode(model.NewRequest("New Request")) }},
	{"new-folder", "New Folder", keys{ui.Cmd | ui.Shift, ui.KeyN}, func(a *app) { a.newNode(model.NewFolder("New Folder")) }},
	{"close-tab", "Close Tab", keys{ui.Cmd, ui.KeyW}, func(a *app) { a.closeTab(a.active) }},
	{"close-other-tabs", "Close Other Tabs", keys{ui.Cmd | ui.Shift, ui.KeyW}, func(a *app) {
		if a.current() != nil {
			a.closeOthers(a.active)
		}
	}},
	// The menu closes the window, as only it has the window.
	{"close-window", "Close Window", keys{}, nil},
	{"import", "Import Collection…", keys{ui.Cmd, ui.KeyO}, (*app).importFile},
	{"import-curl", "Import cURL…", keys{ui.Cmd | ui.Shift, ui.KeyO}, (*app).openCurl},
	{"export", "Export Collection…", keys{}, func(a *app) { a.exportWorkspace(false) }},
	{"export-postman", "Export as Postman…", keys{}, func(a *app) { a.exportWorkspace(true) }},
	{"search", "Search…", keys{ui.Cmd, ui.KeyK}, (*app).openSearch},
	{"variables", "Variables…", keys{ui.Cmd, ui.KeyE}, func(a *app) { a.envOpen = true }},
	{"settings", "Settings…", keys{ui.Cmd, ui.KeyComma}, (*app).openSettings},
	{"focus-url", "Focus URL", keys{ui.Cmd, ui.KeyL}, nil},
	{"send", "Send Request", keys{ui.Cmd, ui.KeyEnter}, nil},
}

func commandByID(id string) *command {
	return &commands[slices.IndexFunc(commands, func(c command) bool { return c.id == id })]
}

// keys is the shortcut of the command id: the one set in Settings, or its
// default.
func (a *app) keys(id string) keys {
	if s, ok := a.ws.Shortcuts[id]; ok {
		k, _ := parseKeys(s)
		return k
	}
	return commandByID(id).def
}

// shortcut reports whether the command's keys were pressed. None are while
// Settings records new keys, which go to the recorder then.
func (a *app) shortcut(c *ui.Context, id string) bool {
	k := a.keys(id)
	return a.recording == "" && !k.none() && c.Shortcut(k.mods, k.key)
}

// hint is text followed by the command's keys, as "Search ⌘K".
func (a *app) hint(text, id string) string {
	if k := a.keys(id); !k.none() {
		return strings.TrimSpace(text + " " + k.String())
	}
	return text
}

// menuItem is the command's item in a menu of the view, showing its keys.
func (a *app) menuItem(m *ui.Menu, id string) *ui.MenuItem {
	k := a.keys(id)
	return m.Item(commandByID(id).label).Shortcut(k.mods, k.key)
}

// setKeys gives cmd the keys k, taking them from the command that had
// them, which the note under cmd names. Defaults aren't written down.
func (a *app) setKeys(cmd command, k keys) {
	store := func(id string, k keys) {
		if k == commandByID(id).def {
			delete(a.ws.Shortcuts, id)
			return
		}
		if a.ws.Shortcuts == nil {
			a.ws.Shortcuts = map[string]string{}
		}
		a.ws.Shortcuts[id] = k.accel()
	}
	for _, o := range commands {
		if o.id != cmd.id && !k.none() && a.keys(o.id) == k {
			store(o.id, keys{})
			a.keysNote = keysNote{cmd.id, "Removed from " + strings.TrimSuffix(o.label, "…") + "."}
		}
	}
	store(cmd.id, k)
	a.keysChanged()
}

func (a *app) keysChanged() {
	a.changed()
	if a.menuKeys != nil {
		a.menuKeys()
	}
}

// record starts or, with "", stops taking the next keys pressed for the
// command id. The menu lets go of its keys meanwhile, as it would take
// them before the window sees them.
func (a *app) record(id string) {
	if a.recording != id {
		a.recording = id
		if a.menuKeys != nil {
			a.menuKeys()
		}
	}
}

// keysNote is a word under a command's keys in Settings, as what setting
// them did.
type keysNote struct{ id, text string }

// reservedKeys never reach a recorder in the app: the system's menus take
// them first, or the tabs.
var reservedKeys = func() map[keys]string {
	m := map[keys]string{{ui.Super | ui.Shift, ui.KeyZ}: "macOS", {ui.Super | ui.Alt, ui.KeyH}: "macOS"}
	for _, k := range []ui.Key{ui.KeyQ, ui.KeyH, ui.KeyM, ui.KeyC, ui.KeyV, ui.KeyX, ui.KeyA, ui.KeyZ} {
		m[keys{ui.Super, k}] = "macOS"
	}
	for k := ui.Key1; k <= ui.Key9; k++ {
		m[keys{ui.Super, k}] = "switching tabs"
	}
	return m
}()

var resetIcon = icon(`<path d="M3 8a5 5 0 1 0 1.5-3.5"/><path d="M3 2.5V5h2.5"/>`)

// shortcutSettings is the Shortcuts tab of Settings.
func (a *app) shortcutSettings(c *ui.Context) {
	t := c.Theme()
	for _, cmd := range commands {
		c.Key(cmd.id)
		ui.Row(c).AlignItems(ui.Center).Gap(6).MinHeight(30).Children(func() {
			ui.Text(c, strings.TrimSuffix(cmd.label, "…")).Grow(1)
			a.keysButton(c, cmd)
			// Kept in place while hidden, so the keys line up.
			reset := iconButton(c, resetIcon, "Reset "+cmd.label)
			if _, set := a.ws.Shortcuts[cmd.id]; !set {
				reset.Opacity(0).IgnoreInput(true)
			} else if reset.Clicked() {
				a.keysNote = keysNote{}
				a.setKeys(cmd, cmd.def)
			}
		})
		if a.keysNote.id == cmd.id {
			ui.Text(c, a.keysNote.text).FontSize(11).TextColor(t.TextMuted).Padding(0, 0, 4)
		}
	}
	ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(10, 0, 0).Children(func() {
		ui.Text(c, "Click a shortcut, then press the new keys. ⌫ removes it, Esc cancels.").
			FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0)
		if ui.Button(c, "Restore Defaults").Disabled(len(a.ws.Shortcuts) == 0).Clicked() {
			a.ws.Shortcuts, a.keysNote = nil, keysNote{}
			a.keysChanged()
		}
	})
}

// keysButton shows the command's keys and, clicked, takes the next ones
// pressed.
func (a *app) keysButton(c *ui.Context, cmd command) {
	t := c.Theme()
	recording := a.recording == cmd.id
	text, color := a.keys(cmd.id).String(), t.Text
	switch {
	case recording:
		text, color = "Press keys…", t.TextMuted
	case text == "":
		text, color = "None", t.TextMuted
	}
	b := ui.ButtonBase(c).Width(140).Padding(4, 8).Radius(6).Center().Border(1, t.Border).
		Label(cmd.label + " shortcut")
	if recording {
		b.Border(1, accentInk(t))
	} else if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Text(c, text).FontSize(12).TextColor(color) })
	b.HandleInput(func(ev ui.InputEvent) bool {
		if a.recording != cmd.id || ev.Kind != ui.InputKeyDown {
			return false
		}
		a.pressKeys(cmd, keys{ev.Mods & (ui.Ctrl | ui.Alt | ui.Shift | ui.Super), ev.Key})
		return true
	})
	switch {
	case b.Clicked():
		a.keysNote = keysNote{}
		a.record(cmd.id)
	case recording && !b.Focused():
		a.record("")
	}
}

// pressKeys takes the keys pressed while recording for cmd.
func (a *app) pressKeys(cmd command, k keys) {
	fn := k.key >= ui.KeyF1 && k.key <= ui.KeyF12
	switch {
	case k.key == ui.KeyUnknown: // a modifier on its own, on the way to the key
	case k.mods == 0 && k.key == ui.KeyEscape:
		a.record("")
	case k.mods == 0 && (k.key == ui.KeyBackspace || k.key == ui.KeyDelete):
		a.record("")
		a.setKeys(cmd, keys{})
	case k.accel() == "":
		a.keysNote = keysNote{cmd.id, "That key can't be part of a shortcut."}
	case k.mods&(ui.Super|ui.Ctrl) == 0 && !fn:
		// Without ⌘ or ⌃ the key would type in the inputs instead.
		a.keysNote = keysNote{cmd.id, k.String() + " needs ⌘ or ⌃ with it."}
	case reservedKeys[k] != "":
		a.keysNote = keysNote{cmd.id, k.String() + " is kept for " + reservedKeys[k] + "."}
	default:
		a.record("")
		a.setKeys(cmd, k)
	}
}
