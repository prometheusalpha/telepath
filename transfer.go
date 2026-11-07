package main

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"telepath/internal/importx"
	"telepath/internal/model"
	"telepath/internal/vars"
)

// importFile adds a Telepath export or a Postman collection, in a folder
// of its own at the top, so nothing there is replaced.
func (a *app) importFile() {
	a.openFile(func(name string, data []byte) {
		res, err := importx.Parse(data, name)
		if err != nil {
			a.toast = "Couldn't import " + name + ": " + err.Error()
			return
		}
		a.addImport(res)
	})
}

func (a *app) addImport(res *importx.Result) {
	folder := model.NewFolder(res.Name)
	folder.Folder.Children = append(folder.Folder.Children, res.Items...)
	a.ws.Add(folder, "")
	// Variables the workspace has keep their values: the file's would
	// change what other requests send.
	kept := 0
	for _, kv := range res.Env {
		if slices.ContainsFunc(a.ws.Env, func(e model.KV) bool { return e.Key == kv.Key }) {
			kept++
			continue
		}
		a.ws.Env = append(a.ws.Env, kv)
	}
	a.changed()
	a.files.Open.Add(folder.ID)
	a.reveal(folder.ID)

	requests := 0
	(&model.Workspace{Items: res.Items}).Walk(func(n, _ *model.Node) bool {
		if n.Request != nil {
			requests++
		}
		return true
	})
	msg := fmt.Sprintf("Imported %s", count(requests, "request"))
	if added := len(res.Env) - kept; added > 0 {
		msg += ", " + count(added, "variable")
	}
	if kept > 0 {
		msg += fmt.Sprintf("; kept your value of %s", count(kept, "variable"))
	}
	if len(res.Skipped) > 0 {
		msg += "; skipped " + strings.Join(res.Skipped, ", ")
	}
	a.toast = msg
}

func count(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

func (a *app) exportWorkspace() {
	a.export("Telepath", a.ws.Items, a.ws.Env)
}

// exportFolder writes a folder with the variables its requests use, so
// the file works on its own without carrying every secret along.
func (a *app) exportFolder(n *model.Node) {
	b, _ := json.Marshal(n)
	used := map[string]bool{}
	for _, r := range vars.Refs(string(b)) {
		used[r.Name] = true
	}
	var env []model.KV
	for _, kv := range a.ws.Env {
		if used[kv.Key] {
			env = append(env, kv)
		}
	}
	a.export(n.Name, []*model.Node{n}, env)
}

func (a *app) export(name string, items []*model.Node, env []model.KV) {
	// Marshaled here, on the main thread, before the dialog lets the
	// workspace change.
	data, err := importx.Export(name, items, env)
	if err != nil {
		a.toast = "Couldn't export: " + err.Error()
		return
	}
	a.saveFile(name+".json", data, func(err error) {
		if err != nil {
			a.toast = "Couldn't export: " + err.Error()
		} else {
			a.toast = "Exported " + name
		}
	})
}

func (a *app) copyCurl(c *ui.Context, n *model.Node) {
	r, _ := a.env.Request(n.Request)
	cmd, err := importx.Curl(r)
	if err != nil {
		c.Toast("Couldn't make a cURL command: " + err.Error())
		return
	}
	c.Services().WriteClipboard(cmd)
	c.Toast("Copied as cURL")
}

// curlName names a request pasted as cURL by its path's last part, as
// "users" for https://a.dev/v1/users?x=1.
func curlName(r *model.Request) string {
	base, _ := model.SplitURL(r.URL)
	if _, rest, ok := strings.Cut(base, "://"); ok {
		base = rest
	}
	name := path.Base(strings.TrimRight(base, "/"))
	if _, p, ok := strings.Cut(base, "/"); !ok || p == "" {
		name = base // just the host
	}
	return r.Method + " " + name
}

func (a *app) curlModal(c *ui.Context) {
	t := c.Theme()
	w, _ := c.Size()
	ui.Modal(c, &a.curlOpen, func() {
		ui.Column(c).Width(min(640, w-80)).Gap(8).Children(func() {
			ui.Text(c, "Import cURL").FontSize(16).FontWeight(600)
			ui.Text(c, "Paste a curl command, as a browser's Copy as cURL makes.").FontSize(12).TextColor(t.TextMuted)
			ui.TextArea(c, &a.curlText).Lines(6, 14).Font(mono).FontFeatures(monoFeatures).FontSize(12).AutoFocus().
				Placeholder("curl https://api.example.com/users -H 'Accept: application/json'").Label("cURL command")
			if a.curlErr != "" {
				ui.Text(c, a.curlErr).FontSize(12).TextColor(t.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.curlOpen = false
				}
				if ui.PrimaryButton(c, "Import").Clicked() || c.Shortcut(ui.Cmd, ui.KeyEnter) {
					a.importCurl()
				}
			})
		})
	})
}

func (a *app) importCurl() {
	r, err := importx.ParseCurl(a.curlText)
	if err != nil {
		a.curlErr = err.Error()
		return
	}
	n := model.NewRequest(curlName(r))
	n.Request = r
	a.ws.Add(n, a.selected)
	a.changed()
	a.reveal(n.ID)
	a.openTab(n.ID)
	a.curlOpen, a.curlText, a.curlErr = false, "", ""
}

func (a *app) openCurl() {
	a.curlOpen, a.curlErr = true, ""
}
