package main

import (
	"log"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
	"telepath/internal/store"
)

const (
	titleBarHeight = 44
	saveDelay      = 500 * time.Millisecond
)

type app struct {
	ws   *model.Workspace
	path string // "" keeps the workspace in memory, as tests do
	tree tree

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

	sidebarWidth float32
}

func newApp(ws *model.Workspace, path string) *app {
	a := &app{ws: ws, path: path, row: -1, sidebarWidth: 240}
	a.ws.Walk(func(n, _ *model.Node) bool {
		if n.Folder != nil && n.Folder.Open {
			a.files.Open.Add(n.ID)
		}
		return true
	})
	a.reindex()
	return a
}

// changed marks the workspace for saving and pushes the save back, so a
// burst of edits writes once.
func (a *app) changed() {
	a.reindex()
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
				Padding(0, bar.Right+10, 0, 10).DragWindow()
			ui.Divider(c)
			emptyState(c, "No request open", "Pick one from the sidebar, or press ⌘K to search")
		})
	})
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
		mygo.NewWindow(mygo.WindowOptions{
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
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
