package app

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

const maxHits = 50

type hit struct {
	id     string
	crumbs string // the folders holding it, "Users / Admin"
	score  int
}

// fuzzy scores how well q's runes appear in order in s, case aside: runes
// next to each other and at the start of words count more, so "gu" ranks
// "get user" above "logout".
func fuzzy(q, s string) (int, bool) {
	q, s = strings.ToLower(q), strings.ToLower(s)
	score, run, si := 0, 0, 0
	prev := ' '
	for _, qr := range q {
		found := false
		for si < len(s) {
			r, n := utf8.DecodeRuneInString(s[si:])
			si += n
			if r == qr {
				score += 1 + run*4
				if !unicode.IsLetter(prev) && !unicode.IsDigit(prev) {
					score += 6
				}
				if si == n {
					score += 4 // the very start: "user log" over "ban user"
				}
				run++
				prev, found = r, true
				break
			}
			run, prev = 0, r
		}
		if !found {
			return 0, false
		}
	}
	return score, true
}

// matchNode scores a node for a query of words, each of which must match
// its name, its folders or its method and URL, as typed or with its
// variables filled in (url), so a host matches {{base}}; the name weighs
// most.
func matchNode(words []string, n *model.Node, crumbs, url string) (int, bool) {
	total := 0
	for _, w := range words {
		best, ok := 0, false
		try := func(s string, weight int) {
			if sc, hit := fuzzy(w, s); hit && sc*weight > best {
				best, ok = sc*weight, true
			}
		}
		try(n.Name, 3)
		try(crumbs, 1)
		if n.Request != nil {
			try(n.Request.Method, 2)
			try(n.Request.URL, 1)
			try(url, 1)
		}
		if !ok {
			return 0, false
		}
		total += best
	}
	return total, true
}

func (a *app) crumbs(id string) string {
	var names []string
	for p := a.tree.parent[id]; p != ""; p = a.tree.parent[p] {
		names = append(names, a.tree.byID[p].Name)
	}
	slices.Reverse(names)
	return strings.Join(names, " / ")
}

// searchHits are the best matches of q, or with no query the requests
// opened last.
func (a *app) searchHits(q string) []hit {
	words := strings.Fields(q)
	var hits []hit
	if len(words) == 0 {
		for _, id := range a.recent {
			if a.tree.byID[id] != nil {
				hits = append(hits, hit{id: id, crumbs: a.crumbs(id)})
			}
		}
		return hits
	}
	a.ws.Walk(func(n, _ *model.Node) bool {
		crumbs, url := a.crumbs(n.ID), ""
		if n.Request != nil {
			url = a.env.Expand(n.Request.URL, nil)
		}
		if sc, ok := matchNode(words, n, crumbs, url); ok {
			hits = append(hits, hit{id: n.ID, crumbs: crumbs, score: sc})
		}
		return true
	})
	// Stable, so equal scores keep the sidebar's order.
	slices.SortStableFunc(hits, func(x, y hit) int { return y.score - x.score })
	return hits[:min(len(hits), maxHits)]
}

func (a *app) openSearch() {
	a.searchOpen, a.query, a.searchSel = true, "", 0
	a.hits = a.searchHits("")
}

// reveal shows a node in the sidebar, its folders opened, and chooses it.
func (a *app) reveal(id string) {
	for p := a.tree.parent[id]; p != ""; p = a.tree.parent[p] {
		a.files.Open.Add(p)
	}
	a.selectID = id
}

func (a *app) choose(h hit) {
	n := a.tree.byID[h.id]
	if n == nil {
		return
	}
	a.searchOpen = false
	a.reveal(n.ID)
	if n.Folder != nil {
		a.files.Open.Add(n.ID)
	} else {
		a.openTab(n.ID)
	}
}

func (a *app) searchModal(c *ui.Context) {
	t := c.Theme()
	w, h := c.Size()
	ui.Modal(c, &a.searchOpen, func() {
		a.hits = slices.DeleteFunc(a.hits, func(h hit) bool { return a.tree.byID[h.id] == nil })
		a.searchSel = min(a.searchSel, max(len(a.hits)-1, 0))
		ui.Column(c).Width(min(600, w-80)).Gap(8).Children(func() {
			// A plain input rather than SearchField, whose OnTextKey doesn't
			// reach the text it holds, so the arrows would move the caret.
			var in ui.Element
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 10).Radius(8).Background(t.Surface).Children(func() {
				ui.Icon(c, searchIcon).Size(15, 15).TextColor(t.TextMuted)
				in = ui.TextInputBase(c, &a.query).Grow(1).FontSize(15).AutoFocus().Search().
					Placeholder("Search requests and folders").Label("Search")
			})
			// Keys may come between frames: they read the hits the last
			// frame showed.
			in.OnTextKey(func(k ui.TextKey) bool {
				switch {
				case k.Key == ui.KeyDown && k.Mods == 0:
					a.searchSel = min(a.searchSel+1, len(a.hits)-1)
				case k.Key == ui.KeyUp && k.Mods == 0:
					a.searchSel = max(a.searchSel-1, 0)
				default:
					return false
				}
				return true
			})
			if in.Changed() {
				a.hits, a.searchSel = a.searchHits(a.query), 0
			}
			// Enter is the field's submit, which OnTextKey doesn't see.
			if in.Submitted() && a.searchSel < len(a.hits) {
				a.choose(a.hits[a.searchSel])
			}
			title := "Recent"
			if strings.TrimSpace(a.query) != "" {
				title = "Results"
			}
			if len(a.hits) == 0 {
				msg := "No matches"
				if title == "Recent" {
					msg = "Requests you open show here"
				}
				ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted).Padding(12, 4)
				return
			}
			ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(t.TextMuted).Padding(0, 4)
			chosen := -1
			ui.Scroll(c).MaxHeight(h * 0.5).Children(func() {
				ui.Column(c).Gap(1).Children(func() {
					for i, hh := range a.hits {
						if a.searchRow(c, hh, i == a.searchSel).Clicked() {
							chosen = i
						}
					}
				})
			})
			if chosen >= 0 {
				a.choose(a.hits[chosen])
			}
		})
	})
}

func (a *app) searchRow(c *ui.Context, h hit, sel bool) ui.Element {
	t := c.Theme()
	n := a.tree.byID[h.id]
	c.Key(h.id)
	row := ui.ButtonBase(c).Gap(8).AlignItems(ui.Center).Padding(6, 8).Radius(6).Label(n.Name)
	if sel {
		row.Background(t.Accent).TextColor(t.AccentText)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		var lead ui.Element
		if n.Folder != nil {
			lead = ui.Icon(c, folderIcon).Size(14, 14).TextColor(t.TextMuted)
		} else {
			lead = methodBadge(c, n.Request.Method)
		}
		if sel {
			lead.TextColor(t.AccentText)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, n.Name).SingleLine()
			if n.Request != nil && n.Request.URL != "" {
				url := ui.Text(c, n.Request.URL).Font(mono).FontFeatures(monoFeatures).FontSize(11).SingleLine()
				if !sel {
					url.TextColor(t.TextMuted)
				}
			}
		})
		if h.crumbs != "" {
			crumbs := ui.Text(c, h.crumbs).FontSize(12).SingleLine().MinWidth(0).Shrink(1)
			if !sel {
				crumbs.TextColor(t.TextMuted)
			}
		}
	})
	if sel {
		row.ScrollIntoView()
	}
	return row
}
