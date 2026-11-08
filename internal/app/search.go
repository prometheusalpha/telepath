package app

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/unicode/norm"

	"telepath/internal/model"
)

const maxHits = 50

type hit struct {
	id     string
	crumbs string // the folders holding it, "Users / Admin"
	score  int
}

// fuzzy scores how well q matches s, by the kind of match first, as dbx's
// quick open does: exact, the initials of s's words ("gu" for "get user"),
// a prefix, prefixes of its words, a substring, then q's runes scattered
// in order. Within a kind, a tighter match scores more. Case and marks
// aside: Vietnamese puts the tone on either vowel ("hoá", "hóa"), and
// people type it without any, so runes match by their base letter and
// score a point more when all the marks agree.
func fuzzy(q, s string) (int, bool) {
	qt, st := prep(q), prep(s)
	if len(qt.fold) == 0 {
		return 0, false
	}
	kind, tight, at := match(qt.fold, st)
	if kind == 0 {
		return 0, false
	}
	agree := 1
	for i, p := range at {
		if qt.orig[i] != st.orig[p] {
			agree = 0
			break
		}
	}
	return kind*1000 + tight*2 + agree, true
}

// text is a string ready to match: its runes in lower case, as they are
// and folded (what is not a letter or digit to a space), and its words, split at what is not a letter or digit and
// at camelCase.
type text struct {
	orig, fold []rune
	words      [][2]int // start, end
}

func prep(s string) text {
	var t text
	prev := ' '
	start := -1
	for _, r := range norm.NFC.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		i := len(t.orig)
		sep := !unicode.IsLetter(r) && !unicode.IsDigit(r)
		if start >= 0 && (sep || unicode.IsLower(prev) && unicode.IsUpper(r)) {
			t.words = append(t.words, [2]int{start, i})
			start = -1
		}
		if start < 0 && !sep {
			start = i
		}
		lr := unicode.ToLower(r)
		t.orig = append(t.orig, lr)
		if sep {
			// "get list" is "get-list" and "get_list" too.
			t.fold = append(t.fold, ' ')
		} else {
			t.fold = append(t.fold, fold(lr))
		}
		prev = r
	}
	if start >= 0 {
		t.words = append(t.words, [2]int{start, len(t.orig)})
	}
	return t
}

// match returns the kind of match of q in s (0 for none, 6 for exact),
// how tight it is within that kind (0 to 99) and where q's runes are.
func match(q []rune, s text) (kind, tight int, at []int) {
	n, m := len(s.fold), len(q)
	if m > n {
		return 0, 0, nil
	}
	loose := func(d int) int { return 99 - min(d, 99) }
	run := func(from int) []int {
		at := make([]int, m)
		for i := range at {
			at[i] = from + i
		}
		return at
	}
	if slices.Equal(q, s.fold) {
		return 6, 99, run(0)
	}
	if len(s.words) >= 2 && len(s.words) == m {
		at := make([]int, m)
		for i, w := range s.words {
			if s.fold[w[0]] != q[i] {
				at = nil
				break
			}
			at[i] = w[0]
		}
		if at != nil {
			return 5, loose(m), at
		}
	}
	if slices.Equal(q, s.fold[:m]) {
		return 4, loose(n - m), run(0)
	}
	if at, gap := wordPrefixes(q, s); at != nil {
		return 3, loose(gap), at
	}
	for i := 0; i+m <= n; i++ {
		if slices.Equal(q, s.fold[i:i+m]) {
			return 2, loose(i), run(i)
		}
	}
	// The tightest run of q's runes in order: taking the first rune that
	// fits misses "hóa" in "danh sách hóa", as its h is in "danh".
	best, span := []int(nil), 0
	for i := range n {
		if s.fold[i] != q[0] {
			continue
		}
		at := []int{i}
		for j := i + 1; j < n && len(at) < m; j++ {
			if s.fold[j] == q[len(at)] {
				at = append(at, j)
			}
		}
		if len(at) < m {
			break // no later start fits either
		}
		if sp := at[m-1] - i + 1; best == nil || sp < span {
			best, span = at, sp
		}
	}
	if best != nil {
		return 1, loose(span - m), best
	}
	return 0, 0, nil
}

// wordPrefixes matches q as prefixes of s's words in order, "usli" in
// "user list", one word at least, and returns where q's runes are and how
// far apart the words are: the words skipped between them count most.
func wordPrefixes(q []rune, s text) (at []int, gap int) {
	type state struct {
		first, last, used int
		at                []int
	}
	cost := func(st *state) int { return (st.last-st.first-st.used+1)*10 + st.used }
	m, nw := len(q), len(s.words)
	// states[qi][used] is the best way to match q[:qi] in used words.
	states := make([][]*state, m+1)
	for i := range states {
		states[i] = make([]*state, nw+1)
	}
	states[0][0] = &state{first: -1, last: -1}
	for k, w := range s.words {
		// Down from the end, so a state made from word k is not grown by
		// word k again.
		for qi := m - 1; qi >= 0; qi-- {
			for used := min(k, nw-1); used >= 0; used-- {
				st := states[qi][used]
				if st == nil {
					continue
				}
				for l := 1; l <= min(w[1]-w[0], m-qi); l++ {
					if s.fold[w[0]+l-1] != q[qi+l-1] {
						break
					}
					c := &state{first: st.first, last: k, used: used + 1, at: slices.Clone(st.at)}
					if used == 0 {
						c.first = k
					}
					for i := range l {
						c.at = append(c.at, w[0]+i)
					}
					if old := states[qi+l][used+1]; old == nil || cost(c) < cost(old) {
						states[qi+l][used+1] = c
					}
				}
			}
		}
	}
	var best *state
	for _, st := range states[m][1:] {
		if st != nil && (best == nil || cost(st) < cost(best)) {
			best = st
		}
	}
	if best == nil {
		return nil, 0
	}
	return best.at, cost(best)
}

// fold is r without its marks: "ố" is "o". đ is a letter of its own, not
// d with a mark, so it is folded by hand.
func fold(r rune) rune {
	if r < utf8.RuneSelf {
		return r
	}
	if r == 'đ' {
		return 'd'
	}
	b, _ := utf8.DecodeRuneInString(norm.NFD.String(string(r)))
	return b
}

// matchNode scores a node for a query of words, each of which must match
// its name, its folders or its method and URL, as typed or with its
// variables filled in (url), so a host matches {{base}}. The kind of match
// counts before where it is, so "hóa" whole in a folder's name beats its
// letters strewn over a request named for a long URL; for one kind, the
// name weighs most. A name holding the whole query beats them all.
func matchNode(words []string, n *model.Node, crumbs, url string) (int, bool) {
	total := 0
	for _, w := range words {
		best, ok := 0, false
		try := func(s string, weight int) {
			if sc, hit := fuzzy(w, s); hit && sc+weight > best {
				best, ok = sc+weight, true
			}
		}
		try(n.Name, 400)
		try(crumbs, 0)
		if n.Request != nil {
			try(n.Request.Method, 200)
			try(n.Request.URL, 0)
			try(url, 0)
		}
		if !ok {
			return 0, false
		}
		total += best
	}
	if len(words) > 1 {
		if sc, ok := fuzzy(strings.Join(words, " "), n.Name); ok && sc >= 2000 {
			total += 10000
		}
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
