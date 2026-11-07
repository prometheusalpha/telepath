package main

import (
	"log"
	"net/http"
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
	ws   *model.Workspace
	path string // "" keeps the workspace in memory, as tests do
	tree tree
	env  vars.Env // ws.Env, looked up as the views build

	envOpen  bool
	envDraft model.KV

	searchOpen bool
	query      string
	hits       []hit // what the search shows, for its keys between frames
	searchSel  int
	recent     []string // requests opened, the last first

	files    ui.OutlineState[string]
	depth    int  // of the row being built, from the outline's Prefix
	open     bool // likewise
	row      int
	selected string
	selectID string // a node to choose once the outline has its row

	renaming      string
	renameDraft   string
	renameFocused bool

	deleting  *model.Node
	askDelete bool

	dirty   bool
	saveAt  time.Time
	saveErr error

	tabs   []*tab
	active int // -1 with no tab open
	split  float32
	client *http.Client
	// update runs fn on the main thread and redraws: the window's Update,
	// or in tests whatever drives the Tester.
	update func(fn func())
	// setAppearance makes the app light, dark or follow the system, menus
	// and window materials too; nil in tests, which set the Tester's.
	setAppearance func(theme string)
	// pickFile asks for a file and gives its path to done, on the main
	// thread, unless canceled.
	pickFile func(done func(path string))

	sidebarWidth float32
}

func newApp(ws *model.Workspace, path string) *app {
	a := &app{ws: ws, path: path, row: -1, active: -1, split: 45, client: &http.Client{}, sidebarWidth: 240}
	a.ws.Walk(func(n, _ *model.Node) bool {
		if n.Folder != nil && n.Folder.Open {
			a.files.Open.Add(n.ID)
		}
		return true
	})
	a.reindex()
	a.env = vars.NewEnv(a.ws.Env)
	return a
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
	if err := store.Save(a.path, a.ws); err != nil {
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
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(sidebarColor(t))
	}
	ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c) }, func() { a.content(c) }).
		SplitReset(240).Fill()
	a.envModal(c)
	a.searchModal(c)
	a.autosave(c)
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
				emptyState(c, "No request open", "Pick one from the sidebar, or press ⌘K to search")
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

func main() {
	mygo.App.WhenReady(func() {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(dir, "workspace.json")
		ws, err := store.Load(path)
		if err != nil {
			// Starting empty would overwrite the file on the first edit.
			log.Fatalf("reading %s: %v", path, err)
		}
		a := newApp(ws, path)
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
		// The default menu's Close Window would take ⌘W from the tabs.
		menu := mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Label: "File", Submenu: []*mygo.MenuItem{
				{Label: "Close Tab", Accelerator: "CmdOrCtrl+W", Click: func(*mygo.MenuItem, *mygo.Window) {
					win.Update(func() { a.closeTab(a.active) })
				}},
				{Label: "Close Window", Accelerator: "CmdOrCtrl+Shift+W", Role: mygo.RoleClose},
				mygo.Separator(),
				{Label: "Search…", Accelerator: "CmdOrCtrl+K", Click: func(*mygo.MenuItem, *mygo.Window) {
					win.Update(a.openSearch)
				}},
				{Label: "Variables…", Accelerator: "CmdOrCtrl+E", Click: func(*mygo.MenuItem, *mygo.Window) {
					win.Update(func() { a.envOpen = true })
				}},
			}},
			{Role: mygo.RoleEditMenu},
			{Label: "View", Submenu: []*mygo.MenuItem{
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
