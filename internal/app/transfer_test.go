package app

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/importx"
	"telepath/internal/model"
)

func TestImportPostmanKeepsVariables(t *testing.T) {
	data, err := os.ReadFile("../importx/testdata/acme.postman_collection.json")
	if err != nil {
		t.Fatal(err)
	}
	a := emptyApp()
	a.ws.Env = []model.KV{{Key: "base", Value: "http://localhost", Enabled: true}}
	a.changed()
	a.openFile = func(done func(string, []byte)) { done("acme.postman_collection.json", data) }
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("More")
	if err := tt.ChooseMenuItem("Import Collection…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.ws.Items) != 1 || a.ws.Items[0].Name != "Acme" || !tt.HasText("Users") {
		t.Fatalf("items %q, texts %q", names(a.ws.Items), tt.Texts())
	}
	if a.env["base"] != "http://localhost" || len(a.ws.Env) != 1 {
		t.Errorf("env %+v", a.ws.Env)
	}
	want := "Imported 3 requests; kept your value of 1 variable; skipped 2 scripts, 1 oauth2 auth"
	if !tt.HasText(want) {
		t.Errorf("no toast %q, texts %q", want, tt.Texts())
	}
}

func TestExportFolderTakesItsVariables(t *testing.T) {
	ws := &model.Workspace{Env: []model.KV{
		{Key: "base", Value: "b", Enabled: true}, {Key: "secret", Value: "s", Enabled: true},
	}}
	f := model.NewFolder("Users")
	ws.Add(f, "")
	r := model.NewRequest("list")
	r.Request.URL = "{{base}}/users"
	ws.Add(r, f.ID)
	a := newApp(ws, "")
	var saved []byte
	var savedAs string
	a.saveFile = func(name string, data []byte, done func(error)) { savedAs, saved = name, data; done(nil) }
	tt := ui.NewTester(a.view, 1200, 780)
	tt.RightClick("Users")
	if err := tt.ChooseMenuItem("Export…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	res, err := importx.Parse(saved, savedAs)
	if err != nil || savedAs != "Users.json" || len(res.Items) != 1 || len(res.Env) != 1 || res.Env[0].Key != "base" {
		t.Fatalf("saved %s: %+v, %v", savedAs, res, err)
	}
	if !tt.HasText("Exported Users") {
		t.Errorf("texts %q", tt.Texts())
	}

	tt.RightClick("Users")
	if err := tt.ChooseMenuItem("Export as Postman…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	res, err = importx.Parse(saved, savedAs)
	if err != nil || savedAs != "Users.postman_collection.json" || !strings.Contains(string(saved), `"schema"`) ||
		len(res.Items) != 1 || len(res.Env) != 1 || res.Env[0].Key != "base" {
		t.Errorf("saved %s: %+v, %v", savedAs, res, err)
	}
}

func TestCurlInAndOut(t *testing.T) {
	a := emptyApp()
	a.ws.Env = []model.KV{{Key: "tok", Value: "t1", Enabled: true}}
	a.changed()
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("More")
	if err := tt.ChooseMenuItem("Import cURL…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type(`curl -X PATCH 'https://a.dev/v1/users?x=1' -H 'Authorization: Bearer {{tok}}' --data-raw '{"a":1}'`)
	tt.Click("Import")
	tt.Frame()
	if len(a.ws.Items) != 1 || a.ws.Items[0].Name != "PATCH users" || len(a.tabs) != 1 {
		t.Fatalf("items %q, tabs %d, err %q", names(a.ws.Items), len(a.tabs), a.curlErr)
	}
	r := a.ws.Items[0].Request
	if r.Body.Kind != model.BodyJSON || len(r.Query) != 1 {
		t.Errorf("request %+v", r)
	}

	tt.RightClick("PATCH users")
	if err := tt.ChooseMenuItem("Copy as cURL"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); !strings.Contains(got, "'Authorization: Bearer t1'") || !strings.Contains(got, "-X PATCH") {
		t.Errorf("clipboard %q", got)
	}

	tt.Click("More")
	tt.ChooseMenuItem("Import cURL…")
	tt.Frame()
	tt.Type("wget x")
	tt.Click("Import")
	tt.Frame()
	if !a.curlOpen || !tt.HasText("not a curl command") {
		t.Errorf("a bad command: open %v, texts %q", a.curlOpen, tt.Texts())
	}
}

func TestExportCollectionFromMore(t *testing.T) {
	a := emptyApp()
	a.ws.Add(model.NewRequest("ping"), "")
	a.changed()
	var saved []byte
	a.saveFile = func(name string, data []byte, done func(error)) { saved = data; done(nil) }
	tt := ui.NewTester(a.view, 1200, 780)
	tt.Click("More")
	if err := tt.ChooseMenuItem("Export Collection…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if res, err := importx.Parse(saved, "x.json"); err != nil || len(res.Items) != 1 {
		t.Errorf("saved %s, %v", saved, err)
	}
}
