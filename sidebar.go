package main

import (
	"fmt"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

var chevronIcon = icon(`<path d="M6 3.5L10.5 8 6 12.5"/>`)

var folderIcon = icon(`<path d="M1.5 4.5a1 1 0 0 1 1-1h3.6l1.4 1.5h6a1 1 0 0 1 1 1v6.5a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1z"/>`)

// tree is what the outline reads; rebuilt after every edit of the
// workspace rather than walked each frame.
type tree struct {
	byID  map[string]*model.Node
	roots []string
	kids  map[string][]string // nil for requests, empty for empty folders
}

func (a *app) reindex() {
	t := tree{byID: map[string]*model.Node{}, kids: map[string][]string{}}
	for _, n := range a.ws.Items {
		t.roots = append(t.roots, n.ID)
	}
	a.ws.Walk(func(n, _ *model.Node) bool {
		t.byID[n.ID] = n
		if n.Folder != nil {
			ids := make([]string, 0, len(n.Folder.Children))
			for _, k := range n.Folder.Children {
				ids = append(ids, k.ID)
			}
			t.kids[n.ID] = ids
		}
		return true
	})
	a.tree = t
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(max(bar.Height, titleBarHeight)).Shrink(0).AlignItems(ui.Center).
			Padding(0, 10, 0, bar.Left+10).Gap(2).DragWindow().Children(func() {
			ui.Spacer(c)
			iconButton(c, searchIcon, "Search")
			iconButton(c, plusIcon, "New").Menu(func(m *ui.Menu) {
				if m.Item("New Request").Chosen() {
					a.create(model.NewRequest("New Request"), a.selected)
				}
				if m.Item("New Folder").Chosen() {
					a.create(model.NewFolder("New Folder"), a.selected)
				}
			})
		})
		ui.Text(c, "COLLECTIONS").FontSize(11).FontWeight(600).LetterSpacing(0.6).
			TextColor(t.TextMuted).Padding(8, 16, 6)
		a.outline(c)
	})

	n := a.deleting
	msg := "You can't undo this."
	if n != nil && n.Folder != nil {
		msg = "Everything inside it goes too. You can't undo this."
	}
	if n != nil && ui.AlertDialog(c, &a.askDelete, fmt.Sprintf("Delete “%s”?", n.Name), msg, "Cancel", "Delete") == 1 {
		a.ws.Remove(n.ID)
		a.changed()
	}
	if !a.askDelete {
		a.deleting = nil
	}
}

func (a *app) outline(c *ui.Context) {
	if a.selectID != "" {
		for i := range a.files.Rows() {
			if a.files.Item(i) == a.selectID {
				a.row, a.selectID = i, ""
				break
			}
		}
	}
	a.files.List.Selected = &a.row
	// The prefix only notes where the row sits: node draws the indent and
	// the arrow inside its own row, so a press anywhere on it lands there.
	a.files.Prefix = func(r ui.OutlineRow) { a.depth, a.open = r.Depth, r.Open }
	// Row menus and keys only note what to do: changing the tree while the
	// outline builds its rows would pull them from under it.
	var later func()
	out := ui.Outline(c, &a.files, a.tree.roots, func(id string) []string { return a.tree.kids[id] }, func(id string) {
		if f := a.node(c, a.tree.byID[id]); f != nil {
			later = f
		}
	}).Grow(1).Padding(0, 8).Label("Collections")
	if c.Vibrancy() {
		out.Background(ui.Transparent)
	}
	out.Children(func() {
		if len(a.tree.roots) == 0 {
			emptyState(c, "No requests yet", "Create one with +")
		}
	})

	a.selected = ""
	if a.row >= 0 && a.row < a.files.Rows() {
		a.selected = a.files.Item(a.row)
	}
	if n := a.tree.byID[a.selected]; n != nil && a.renaming == "" {
		switch {
		case out.Submitted() && n.Request != nil:
			a.openTab(n.ID)
		case a.files.List.Shortcut(c, 0, ui.KeyF2):
			a.startRename(n)
		case a.files.List.Shortcut(c, ui.Cmd, ui.KeyBackspace):
			a.confirmDelete(n)
		}
	}
	if later != nil {
		later()
	}
}

// node builds one row and returns what its menu asked for, if anything.
func (a *app) node(c *ui.Context, n *model.Node) (later func()) {
	t := c.Theme()
	row := ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).
		Padding(5, 8, 5, 8+14*float32(a.depth))
	if a.renaming != n.ID {
		// A press opens a request, and toggles a folder every time, quick
		// presses too, as the arrow would. Observing leaves the press to
		// the outline, which chooses the row with it, where Clicked would
		// take it; the right button and Control-click open the menu
		// instead.
		id, folder := n.ID, n.Folder != nil
		row.ObserveInput(func(ev ui.InputEvent) {
			if ev.Kind != ui.InputPointerDown || ev.Button != 0 || ev.Mods&ui.Ctrl != 0 {
				return
			}
			if folder {
				a.toggle(id)
			} else {
				a.openTab(id)
			}
		})
	}
	row.ContextMenu(func(m *ui.Menu) {
		if n.Folder != nil {
			if m.Item("New Request").Chosen() {
				later = func() { a.create(model.NewRequest("New Request"), n.ID) }
			}
			if m.Item("New Folder").Chosen() {
				later = func() { a.create(model.NewFolder("New Folder"), n.ID) }
			}
			m.Separator()
		}
		if m.Item("Rename").Shortcut(0, ui.KeyF2).Chosen() {
			later = func() { a.startRename(n) }
		}
		if m.Item("Duplicate").Chosen() {
			later = func() {
				if cp := a.ws.Duplicate(n.ID); cp != nil {
					a.changed()
					a.selectID = cp.ID
				}
			}
		}
		m.Separator()
		if m.Item("Delete").Shortcut(ui.Cmd, ui.KeyBackspace).Chosen() {
			later = func() { a.confirmDelete(n) }
		}
	})
	row.Children(func() {
		// Muted grays and method hues both wash out on the accent fill of
		// the chosen row.
		// Requests keep the arrow's room so names line up with folders'.
		open := a.open
		ui.Box(c).Size(12, 12).Shrink(0).Center().Children(func() {
			if n.Folder == nil {
				return
			}
			chev := ui.Icon(c, chevronIcon).Size(12, 12).TextColor(t.TextMuted)
			if open {
				chev.Rotate(90)
			}
			if n.ID == a.selected {
				chev.TextColor(t.AccentText)
			}
		})
		var lead ui.Element
		if n.Folder != nil {
			lead = ui.Icon(c, folderIcon).Size(14, 14).TextColor(t.TextMuted)
		} else {
			lead = methodBadge(c, n.Request.Method)
		}
		if n.ID == a.selected {
			lead.TextColor(t.AccentText)
		}
		if a.renaming == n.ID {
			a.renameInput(c, n)
			return
		}
		ui.Text(c, n.Name).SingleLine().Grow(1).MinWidth(0)
	})
	return later
}

func (a *app) renameInput(c *ui.Context, n *model.Node) {
	t := c.Theme()
	in := ui.TextInputBase(c, &a.renameDraft).AutoFocus().Grow(1).MinWidth(0).
		Padding(1, 4).Margin(-2, -5).Radius(4).Background(t.Background).Border(1, t.Accent).
		TextColor(t.Text).Label("Name")
	focused := in.Focused()
	if !a.renameFocused {
		// Typing replaces the old name, as Finder's rename does.
		in.SetTextSelection(0, utf8.RuneCountInString(a.renameDraft))
	}
	switch {
	case in.Submitted():
		a.finishRename(c, n, true)
	case in.Shortcut(0, ui.KeyEscape):
		a.finishRename(c, n, false)
	case a.renameFocused && !focused:
		// Clicking elsewhere keeps what was typed, as Finder does.
		a.finishRename(c, n, true)
	}
	a.renameFocused = a.renameFocused || focused
}

func (a *app) startRename(n *model.Node) {
	a.renaming, a.renameDraft, a.renameFocused = n.ID, n.Name, false
}

func (a *app) finishRename(c *ui.Context, n *model.Node, keep bool) {
	if keep && a.renameDraft != "" && a.renameDraft != n.Name {
		n.Name = a.renameDraft
		a.changed()
	}
	a.renaming = ""
	a.files.List.Focus(c)
}

func (a *app) confirmDelete(n *model.Node) {
	a.deleting, a.askDelete = n, true
}

// create adds n inside the folder at, or beside the request at, and lets
// the user name it straight away.
func (a *app) create(n *model.Node, at string) {
	a.ws.Add(n, at)
	if p := a.tree.byID[at]; p != nil && p.Folder != nil {
		a.files.Open.Add(at)
	}
	a.changed()
	a.selectID = n.ID
	a.startRename(n)
}

func (a *app) toggle(id string) {
	if a.files.Open.Has(id) {
		a.files.Open.Remove(id)
	} else {
		a.files.Open.Add(id)
	}
}
