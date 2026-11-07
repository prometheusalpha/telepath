package main

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

var closeIcon = icon(`<path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/>`)

// openTab shows the request with id, in its tab if it has one.
func (a *app) openTab(id string) {
	if n := a.tree.byID[id]; n == nil || n.Request == nil {
		return
	}
	a.recent = slices.Insert(slices.DeleteFunc(a.recent, func(r string) bool { return r == id }), 0, id)
	a.recent = a.recent[:min(len(a.recent), 10)]
	if i := slices.IndexFunc(a.tabs, func(t *tab) bool { return t.id == id }); i >= 0 {
		a.active = i
		return
	}
	// Requests saved before Query and Path existed get them from the URL.
	r := a.tree.byID[id].Request
	r.QueryFromURL()
	r.PathFromURL()
	a.tabs = append(a.tabs, &tab{id: id})
	a.active = len(a.tabs) - 1
}

func (a *app) closeTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	a.tabs[i].stop()
	a.tabs = slices.Delete(a.tabs, i, i+1)
	if a.active >= i && a.active > 0 {
		a.active--
	}
	if len(a.tabs) == 0 {
		a.active = -1
	}
}

// pruneTabs closes the tabs of requests that were deleted, directly or
// with their folder.
func (a *app) pruneTabs() {
	for i := len(a.tabs) - 1; i >= 0; i-- {
		if a.tree.byID[a.tabs[i].id] == nil {
			a.closeTab(i)
		}
	}
}

func (a *app) current() *tab {
	if a.active < 0 || a.active >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.active]
}

func (a *app) tabBar(c *ui.Context) {
	t := c.Theme()
	closing := -1
	ui.Row(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Center).Clip().Children(func() {
		for i, tb := range a.tabs {
			n := a.tree.byID[tb.id]
			c.Key(tb.id)
			pill := ui.ButtonBase(c).Gap(6).AlignItems(ui.Center).Padding(4, 6, 4, 10).Radius(7).
				MinWidth(80).MaxWidth(200).Shrink(1).Label(n.Name).Tooltip(n.Name)
			switch {
			case i == a.active:
				pill.Background(t.Surface)
			case pill.Hovered():
				pill.Background(t.SurfaceHover.Alpha(0.6))
			}
			pill.Children(func() {
				methodBadge(c, n.Request.Method)
				name := ui.Text(c, n.Name).SingleLine().Grow(1).MinWidth(0).FontSize(12)
				if i != a.active {
					name.TextColor(t.TextMuted)
				}
				x := ui.ButtonBase(c).Size(16, 16).Radius(4).Center().TextColor(t.TextMuted).Label("Close " + n.Name)
				if x.Hovered() {
					x.Background(t.SurfaceHover).TextColor(t.Text)
				}
				x.Children(func() { ui.Icon(c, closeIcon).Size(10, 10) })
				if x.Clicked() {
					closing = i
				}
			})
			if pill.Clicked() {
				a.active = i
			}
		}
	})
	if closing >= 0 {
		a.closeTab(closing)
	}
}
