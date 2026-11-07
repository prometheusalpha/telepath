package main

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

var commonHeaders = []string{
	"Accept", "Accept-Encoding", "Accept-Language", "Authorization", "Cache-Control",
	"Content-Type", "Cookie", "If-Match", "If-None-Match", "Origin", "Referer",
	"User-Agent", "X-API-Key", "X-Request-ID", "X-Requested-With",
}

type kvOptions struct {
	name string // for the inputs' labels: "Query key 1"
	// fixedKeys shows keys that come from elsewhere, as the URL's :names:
	// no checkboxes, no adding or deleting rows.
	fixedKeys   bool
	suggestions []string // for keys
	files       bool     // rows may send a file (multipart)
}

// kvTable edits rows, with a blank row last for adding one (none with
// fixedKeys, whose draft is nil), and reports an edit. draft holds what is typed into the blank row: as soon as it holds
// anything it becomes a row, keyed as the blank row was, so the input
// being typed in keeps the focus.
func (a *app) kvTable(c *ui.Context, rows *[]model.KV, draft *model.KV, o kvOptions) bool {
	t := c.Theme()
	changed, del := false, -1
	ui.Column(c).Padding(6, 0).Children(func() {
		for i := range *rows {
			kv := &(*rows)[i]
			c.Key(i)
			r := ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(2, 12)
			r.Children(func() {
				if o.fixedKeys {
					// Columns line up with the other tables': the checkbox's
					// room, then the key where its input would be.
					ui.Box(c).Size(t.Space(4), t.Space(4)).Shrink(0)
					ui.Text(c, kv.Key).Font(mono).FontSize(12).Grow(1).MinWidth(0).Padding(0, 11).SingleLine()
				} else {
					if ui.Checkbox(c, &kv.Enabled, "").Label(fmt.Sprintf("%s %d enabled", o.name, i+1)).Changed() {
						changed = true
					}
					changed = a.keyInput(c, &kv.Key, fmt.Sprintf("%s key %d", o.name, i+1), !kv.Enabled, o) || changed
				}
				// The rows may change while the file dialog is up: the path
				// lands only if the row is still where it was.
				idx, key := i, kv.Key
				setFile := func(path string) {
					if idx < len(*rows) && (*rows)[idx].Key == key {
						(*rows)[idx].Value = path
						a.changed()
					}
				}
				changed = a.valueInput(c, kv, fmt.Sprintf("%s value %d", o.name, i+1), !kv.Enabled && !o.fixedKeys, o, setFile) || changed
				if o.fixedKeys {
					ui.Box(c).Size(26, 26).Shrink(0)
				} else if iconButton(c, closeIcon, fmt.Sprintf("Delete %s %d", o.name, i+1)).Clicked() {
					del = i
				}
			})
		}
		if o.fixedKeys {
			if len(*rows) == 0 {
				ui.Text(c, "Path parameters come from :name segments of the URL, as /users/:id.").
					FontSize(12).TextColor(t.TextMuted).Padding(10, 12)
			}
			return
		}
		c.Key(len(*rows))
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(2, 12).Children(func() {
			ui.Box(c).Size(t.Space(4), t.Space(4)).Shrink(0) // where the checkbox goes
			a.keyInput(c, &draft.Key, o.name+" new key", false, o)
			// The draft becomes a row in the frame the path lands in.
			a.valueInput(c, draft, o.name+" new value", false, o, func(path string) { draft.Value = path })
			ui.Box(c).Size(26, 26).Shrink(0)
		})
	})
	if draft != nil && (draft.Key != "" || draft.Value != "") {
		*rows = append(*rows, model.KV{Key: draft.Key, Value: draft.Value, Enabled: true, File: draft.File})
		*draft = model.KV{}
		changed = true
	}
	if del >= 0 {
		*rows = slices.Delete(*rows, del, del+1)
		changed = true
	}
	return changed
}

func (a *app) keyInput(c *ui.Context, key *string, label string, muted bool, o kvOptions) bool {
	var in ui.Element
	if o.suggestions != nil {
		in = ui.Autocomplete(c, key, o.suggestions)
	} else {
		in = ui.TextInput(c, key)
	}
	in.Grow(1).MinWidth(0).Font(mono).FontSize(12).Placeholder("Key").Label(label)
	if muted {
		in.TextColor(c.Theme().TextMuted)
	}
	return in.Changed()
}

func (a *app) valueInput(c *ui.Context, kv *model.KV, label string, muted bool, o kvOptions, setFile func(string)) bool {
	changed := false
	if o.files {
		kind := "Text"
		if kv.File {
			kind = "File"
		}
		if ui.Select(c, &kind, []string{"Text", "File"}).Width(76).FontSize(12).Label(label + " kind").Changed() {
			kv.File, kv.Value, changed = kind == "File", "", true
		}
	}
	if kv.File {
		name := "Choose file…"
		if kv.Value != "" {
			name = filepath.Base(kv.Value)
		}
		b := ui.Button(c, name).Grow(2).MinWidth(0).FontSize(12).Label(label).Tooltip(kv.Value)
		if b.Clicked() {
			a.pickFile(setFile)
		}
		return changed
	}
	in := ui.TextInput(c, &kv.Value).Grow(2).MinWidth(0).Font(mono).FontSize(12).Placeholder("Value").Label(label)
	if muted {
		in.TextColor(c.Theme().TextMuted)
	}
	return in.Changed() || changed
}
