package app

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
	"telepath/internal/store"
	"telepath/internal/vars"
)

const (
	titleBarHeight = 44
	saveDelay      = 500 * time.Millisecond
)

type app struct {
	ws          *model.Workspace
	path        string // "" keeps the workspace in memory, as tests do
	sessionPath string
	sealer      *store.Sealer // nil saves secrets as they are
	tree        tree
	env         vars.Env // ws.Env, looked up as the views build

	envOpen  bool
	envDraft model.KV

	settingsOpen bool
	settingsTab  int
	monoFamilies []string // the code fonts settings offer, once they opened
	recording    string   // the command Settings takes the next keys for
	keysNote     keysNote // what setting a command's keys did to another
	// menuKeys gives the menu items the commands' keys again, none while
	// recording; nil in tests.
	menuKeys func()

	suggest    suggestion // the variables listed under the input typed in
	dismissed  dismissal
	suggested  string // the input a variable was put in between frames
	caretLabel string // the input whose caret goes to caretAt next frame
	caretAt    int
	typedKey   string // what was last typed in a key input, which lists names for it
	tip        varTip // the variable the pointer is over, with its value

	searchOpen bool
	query      string
	hits       []hit // what the search shows, for its keys between frames
	searchSel  int
	recent     []string // requests opened, the last first

	curlOpen bool
	curlText string
	curlErr  string
	// toast is a message for the next frame to show, from work that ends
	// outside the view, as a file dialog's.
	toast string

	files    ui.OutlineState[string]
	depth    int  // of the row being built, from the outline's Prefix
	open     bool // likewise
	row      int
	selected string
	selectID string // a node to choose once the outline has its row
	followed string // the tab the sidebar last turned to

	press    press    // the row a press began on, to tell a click from a drag
	enterKey bool     // Enter went to the outline since the last frame
	dropHint dropHint // where a drag over the sidebar would drop, last frame
	dropNext dropHint // and this frame

	urlHandle ui.Handle // the URL input, for ⌘L

	renaming      string
	renameDraft   string
	renameFocused bool

	deleting  *model.Node
	askDelete bool

	dirty   bool
	saveAt  time.Time
	saveErr error

	tabs   []*tab
	active int     // -1 with no tab open
	split  float32 // the request's share above the response, in percent
	// responseRight puts the response beside the request, which takes
	// splitRight percent of the width; each layout keeps its own size.
	responseRight bool
	splitRight    float32
	// onLayout tells the menu the response moved, as the button moves it
	// too; nil in tests.
	onLayout func(right bool)
	client   *http.Client
	// update runs fn on the main thread and redraws: the window's Update,
	// or in tests whatever drives the Tester.
	update func(fn func())
	// setAppearance makes the app light, dark or follow the system, menus
	// and window materials too; nil in tests, which set the Tester's.
	setAppearance func(theme string)
	// pickFile asks for a file and gives its path to done, on the main
	// thread, unless canceled.
	pickFile func(done func(path string))
	// openFile asks for a file and gives its name and contents to done;
	// saveFile asks where to write data, writes it and reports to done.
	// Both call back on the main thread, and not at all when canceled.
	openFile func(done func(name string, data []byte))
	saveFile func(name string, data []byte, done func(error))

	sidebarWidth float32
}

func newApp(ws *model.Workspace, path string) *app {
	a := &app{ws: ws, path: path, row: -1, active: -1, split: 45, splitRight: 50, client: &http.Client{}, sidebarWidth: 240,
		// Without a window there are no dialogs: main and tests set these.
		pickFile: func(func(string)) {},
		openFile: func(func(string, []byte)) {},
		saveFile: func(string, []byte, func(error)) {},
	}
	if path != "" {
		a.sessionPath = filepath.Join(filepath.Dir(path), "session.json")
	}
	a.ws.Walk(func(n, _ *model.Node) bool {
		if n.Folder != nil && n.Folder.Open {
			a.files.Open.Add(n.ID)
		}
		return true
	})
	a.reindex()
	a.env = vars.NewEnv(a.ws.Env)
	setMono(ws)
	return a
}

// touch marks the session, as tabs opening and closing, for saving with
// the workspace.
func (a *app) touch() {
	a.dirty, a.saveAt = true, time.Time{}
}

func (a *app) session() store.Session {
	s := store.Session{Active: a.active, SidebarWidth: a.sidebarWidth, Split: a.split,
		ResponseRight: a.responseRight, SplitRight: a.splitRight}
	for _, tb := range a.tabs {
		s.Tabs = append(s.Tabs, tb.id)
	}
	return s
}

// restore reopens the tabs of a session whose requests are still there.
func (a *app) restore(s store.Session) {
	for _, id := range s.Tabs {
		a.openTab(id)
	}
	if s.Active >= 0 && s.Active < len(a.tabs) {
		a.active = s.Active
	}
	if s.SidebarWidth > 0 {
		a.sidebarWidth = s.SidebarWidth
	}
	if s.Split > 0 {
		a.split = s.Split
	}
	if s.SplitRight > 0 {
		a.splitRight = s.SplitRight
	}
	a.responseRight = s.ResponseRight
	a.dirty = false
}

// changed marks the workspace for saving and pushes the save back, so a
// burst of edits writes once.
func (a *app) changed() {
	a.reindex()
	a.env = vars.NewEnv(a.ws.Env)
	a.pruneTabs()
	a.dirty, a.saveAt = true, time.Time{}
}

func (a *app) flush() error {
	if a.path == "" {
		a.dirty = false
		return nil
	}
	// Folders open and close in the outline without an edit, so their
	// state is copied over at every save.
	a.ws.Walk(func(n, _ *model.Node) bool {
		if n.Folder != nil {
			n.Folder.Open = a.files.Open.Has(n.ID)
		}
		return true
	})
	if err := store.Save(a.path, a.ws, a.sealer); err != nil {
		return err
	}
	if err := store.SaveSession(a.sessionPath, a.session()); err != nil {
		return err
	}
	a.dirty = false
	return nil
}

func (a *app) autosave(c *ui.Context) {
	if !a.dirty {
		return
	}
	now := c.Now()
	if a.saveAt.IsZero() {
		a.saveAt = now.Add(saveDelay)
	}
	if now.Before(a.saveAt) {
		c.After(a.saveAt.Sub(now))
		return
	}
	if err := a.flush(); err != nil {
		// Keep dirty and retry with the next edit; one toast per failure
		// streak is enough.
		if a.saveErr == nil {
			c.Toast("Couldn't save: " + err.Error())
		}
		a.saveErr, a.dirty = err, false
		return
	}
	a.saveErr = nil
}

func (a *app) view(c *ui.Context) {
	applyTheme(c)
	t := c.Theme()
	// The sidebar follows the tab shown, however it came to show: a click,
	// the keys, a tab closing, a search.
	if tb := a.current(); tb == nil {
		a.followed = ""
	} else if tb.id != a.followed {
		a.followed = tb.id
		a.reveal(tb.id)
	}
	// The Split reads the theme as it is built; its panes get t back.
	if t.Dark {
		c.SetTheme(darkSplit)
	} else {
		c.SetTheme(lightSplit)
	}
	ui.Split(c, &a.sidebarWidth, func() {
		c.SetTheme(t)
		a.sidebar(c)
	}, func() { a.content(c) }).SplitReset(240).Fill()
	// After SetTheme, which paints the root in the theme's Background.
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(sidebarColor(t))
	}
	a.envModal(c)
	a.settingsModal(c)
	a.searchModal(c)
	a.curlModal(c)
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
	a.shortcuts(c)
	a.autosave(c)
}

// shortcuts are the window's keys. The menu has those of its commands
// too, and takes them first in the app; the view's are for windows without
// that menu, as tests'.
func (a *app) shortcuts(c *ui.Context) {
	for _, cmd := range commands {
		if cmd.run != nil && a.shortcut(c, cmd.id) {
			cmd.run(a)
		}
	}
	if a.shortcut(c, "focus-url") && a.current() != nil {
		// Only with a URL showing: Focus waits for one otherwise, and would
		// take the focus whenever a tab opens next.
		a.urlHandle.Focus(c)
	}
	for i := range 9 {
		if a.recording != "" || !c.Shortcut(ui.Cmd, ui.Key1+ui.Key(i)) || len(a.tabs) == 0 {
			continue
		}
		// ⌘9 is the last tab, however many there are, as in browsers.
		if i == 8 {
			i = len(a.tabs) - 1
		}
		if i < len(a.tabs) {
			a.active = i
			a.touch()
		}
	}
}

// content is the card right of the sidebar; its top strip is where the
// request tabs go, and drags the window meanwhile.
func (a *app) content(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	ui.Column(c).Fill().Padding(6, 6, 6, 0).Children(func() {
		card := ui.Column(c).Fill().Radius(10).Clip().Background(t.Background).Border(1, t.Border)
		if !t.Dark {
			card.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.06))
		}
		card.Children(func() {
			ui.Row(c).Height(max(bar.Height, titleBarHeight)-6).Shrink(0).AlignItems(ui.Center).
				Padding(0, bar.Right+10, 0, 8).DragWindow().Children(func() { a.tabBar(c) })
			ui.Divider(c)
			if tb := a.current(); tb != nil {
				a.requestView(c, tb)
			} else {
				hint := "Pick one from the sidebar"
				if k := a.keys("search"); !k.none() {
					hint += ", or press " + k.String() + " to search"
				}
				emptyState(c, "No request open", hint)
			}
		})
	})
}

func themeSource(theme string) mygo.ThemeSource {
	switch theme {
	case model.ThemeLight:
		return mygo.ThemeLight
	case model.ThemeDark:
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// Run opens the window and runs the app until it quits.
func Run() {
	mygo.App.WhenReady(func() {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(dir, "workspace.json")
		// Without the credential store, secrets stay as they are in the
		// file: those sealed before stay sealed, new ones go in the clear.
		sealer, sealErr := store.NewSealer(keychain{keyService})
		if sealErr != nil {
			log.Printf("secrets saved unencrypted: %v", sealErr)
		}
		ws, locked, err := store.Load(path, sealer)
		if err != nil {
			// Starting empty would overwrite the file on the first edit.
			log.Fatalf("reading %s: %v", path, err)
		}
		a := newApp(ws, path)
		a.sealer = sealer
		a.restore(store.LoadSession(a.sessionPath))
		switch {
		case locked > 0:
			a.toast = fmt.Sprintf("Couldn't unlock %d saved secrets: the Keychain didn't give Telepath its key", locked)
		case sealErr != nil:
			a.toast = "Secrets are saved unencrypted: the Keychain isn't available"
		}
		mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
			if err := a.flush(); err != nil {
				log.Print(err)
			}
		})
		var win *mygo.Window
		themeItem := func(id, label, theme string) *mygo.MenuItem {
			return &mygo.MenuItem{ID: id, Label: label, Type: mygo.MenuItemRadio, Checked: ws.Theme == theme,
				Click: func(*mygo.MenuItem, *mygo.Window) { win.Update(func() { a.setTheme(theme) }) }}
		}
		// A command's item, with the keys set for it.
		item := func(id string) *mygo.MenuItem {
			cmd := commandByID(id)
			return &mygo.MenuItem{ID: id, Label: cmd.label, Accelerator: a.keys(id).accel(),
				Click: func(*mygo.MenuItem, *mygo.Window) { win.Update(func() { cmd.run(a) }) }}
		}
		// Not RoleClose, which brings ⌘W along when it has no keys of its
		// own, and ⌘W closes a tab.
		closeWindow := item("close-window")
		closeWindow.Click = func(*mygo.MenuItem, *mygo.Window) { win.Close() }
		menu := mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout},
				mygo.Separator(),
				item("settings"),
				mygo.Separator(),
				{Role: mygo.RoleServices},
				mygo.Separator(),
				{Role: mygo.RoleHide},
				{Role: mygo.RoleHideOthers},
				{Role: mygo.RoleUnhide},
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			}},
			{Label: "File", Submenu: []*mygo.MenuItem{
				item("new-request"),
				item("new-folder"),
				mygo.Separator(),
				item("close-tab"),
				item("close-other-tabs"),
				closeWindow,
				mygo.Separator(),
				item("import"),
				item("import-curl"),
				item("export"),
				item("export-postman"),
				mygo.Separator(),
				item("search"),
				item("variables"),
			}},
			{Role: mygo.RoleEditMenu},
			{Label: "View", Submenu: []*mygo.MenuItem{
				{ID: "response-below", Label: "Response Below", Type: mygo.MenuItemRadio, Checked: !a.responseRight,
					Click: func(*mygo.MenuItem, *mygo.Window) { win.Update(func() { a.setResponseRight(false) }) }},
				{ID: "response-right", Label: "Response on the Right", Type: mygo.MenuItemRadio, Checked: a.responseRight,
					Click: func(*mygo.MenuItem, *mygo.Window) { win.Update(func() { a.setResponseRight(true) }) }},
				mygo.Separator(),
				{Label: "Appearance", Submenu: []*mygo.MenuItem{
					themeItem("theme-system", "System", model.ThemeSystem),
					themeItem("theme-light", "Light", model.ThemeLight),
					themeItem("theme-dark", "Dark", model.ThemeDark),
				}},
			}},
			{Role: mygo.RoleWindowMenu},
		})
		mygo.App.SetMenu(menu)
		a.setAppearance = func(theme string) {
			mygo.Theme.SetSource(themeSource(theme))
			// The sidebar's button changes it too, so the radio items are
			// set here rather than left to the menu.
			for id, t := range map[string]string{"theme-system": model.ThemeSystem, "theme-light": model.ThemeLight, "theme-dark": model.ThemeDark} {
				menu.ItemByID(id).SetChecked(t == theme)
			}
		}
		a.menuKeys = func() {
			for _, cmd := range commands {
				if it := menu.ItemByID(cmd.id); it != nil {
					acc := ""
					if a.recording == "" {
						acc = a.keys(cmd.id).accel()
					}
					it.SetAccelerator(acc)
				}
			}
		}
		a.onLayout = func(right bool) {
			menu.ItemByID("response-below").SetChecked(!right)
			menu.ItemByID("response-right").SetChecked(right)
		}
		// Before the window shows, so it opens in the saved appearance.
		mygo.Theme.SetSource(themeSource(ws.Theme))
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:          "telepath",
			Width:          1200,
			Height:         780,
			MinWidth:       900,
			MinHeight:      600,
			StateKey:       "main",
			TitleBarStyle:  mygo.TitleBarHidden,
			TitleBarHeight: titleBarHeight,
			Vibrancy:       mygo.VibrancySidebar,
			Content:        ui.View(a.view),
		})
		a.update = win.Update
		jsonFiles := []mygo.FileFilter{{Name: "Collections", Extensions: []string{"json"}}}
		a.openFile = func(done func(string, []byte)) {
			go func() {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Import", Filters: jsonFiles})
				if err != nil || len(paths) == 0 {
					return
				}
				data, err := os.ReadFile(paths[0])
				win.Update(func() {
					if err != nil {
						a.toast = "Couldn't read " + filepath.Base(paths[0]) + ": " + err.Error()
						return
					}
					done(filepath.Base(paths[0]), data)
				})
			}()
		}
		a.saveFile = func(name string, data []byte, done func(error)) {
			go func() {
				var filters []mygo.FileFilter
				if filepath.Ext(name) == ".json" {
					filters = jsonFiles
				}
				path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: "Save", DefaultPath: name, Filters: filters})
				if err != nil || path == "" {
					return
				}
				err = os.WriteFile(path, data, 0o644)
				win.Update(func() { done(err) })
			}()
		}
		a.pickFile = func(done func(string)) {
			// Open waits for the user, so it can't run on the main thread.
			go func() {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Choose a File"})
				if err == nil && len(paths) > 0 {
					win.Update(func() { done(paths[0]) })
				}
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
