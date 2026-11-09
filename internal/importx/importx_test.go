package importx

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"telepath/internal/model"
)

func TestSplitShell(t *testing.T) {
	got, err := splitShell("curl 'a b' \"c \\\"d\\\"\" $'e\\nf\\'g' h\\ i \\\n  -X")
	want := []string{"curl", "a b", `c "d"`, "e\nf'g", "h i", "-X"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("splitShell = %q, %v", got, err)
	}
	if _, err := splitShell("curl 'open"); err == nil {
		t.Error("an open quote")
	}
}

func TestParseCurlFromChrome(t *testing.T) {
	r, err := ParseCurl(`curl 'https://api.acme.dev/users/:id?expand=roles' \
  -H 'accept: application/json' \
  -H 'authorization: Bearer abc' \
  --data-raw $'{"name":"Ada\'s"}' \
  --compressed -s -o /dev/null`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || r.URL != "https://api.acme.dev/users/:id?expand=roles" || len(r.Query) != 1 || len(r.Path) != 1 {
		t.Errorf("method %q, url %q, query %+v, path %+v", r.Method, r.URL, r.Query, r.Path)
	}
	if len(r.Headers) != 1 || r.Auth.Kind != model.AuthBearer || r.Auth.Token != "abc" {
		t.Errorf("headers %+v, auth %+v", r.Headers, r.Auth)
	}
	if r.Body.Kind != model.BodyJSON || r.Body.Text != `{"name":"Ada's"}` {
		t.Errorf("body %+v", r.Body)
	}
}

func TestParseCurlBodies(t *testing.T) {
	r, _ := ParseCurl(`curl -XPUT localhost:8080/x -d 'a=1' -d 'b=x%20y' -u ada:pw`)
	if r.Method != "PUT" || r.Body.Kind != model.BodyForm || len(r.Body.Form) != 2 || r.Body.Form[1].Value != "x y" ||
		r.Auth.Kind != model.AuthBasic || r.Auth.Password != "pw" {
		t.Errorf("form: %+v", r)
	}
	r, _ = ParseCurl(`curl https://a.dev/up -F title=hi -F 'doc=@/tmp/a.pdf;type=application/pdf' -H 'Content-Type: multipart/form-data'`)
	if r.Method != "POST" || r.Body.Kind != model.BodyMultipart || !r.Body.Form[1].File || r.Body.Form[1].Value != "/tmp/a.pdf" || len(r.Headers) != 0 {
		t.Errorf("multipart: %+v", r)
	}
	r, _ = ParseCurl(`curl -G https://a.dev/s -d q=go`)
	if r.Method != "GET" || r.URL != "https://a.dev/s?q=go" || r.Body.Kind != model.BodyNone {
		t.Errorf("-G: %+v", r)
	}
	r, _ = ParseCurl(`curl https://a.dev -H 'Content-Type: text/plain' -d 'hello'`)
	if r.Body.Kind != model.BodyText {
		t.Errorf("text: %+v", r.Body)
	}
	if _, err := ParseCurl("wget https://a.dev"); err == nil {
		t.Error("not curl")
	}
}

func TestParseCurlAuthHeader(t *testing.T) {
	// YWRhOnA6dw== is ada:p:w; the header wins over -u.
	r, _ := ParseCurl(`curl a.dev -u x:y -H 'Authorization: basic YWRhOnA6dw=='`)
	if len(r.Headers) != 0 || r.Auth != (model.Auth{Kind: model.AuthBasic, Username: "ada", Password: "p:w"}) {
		t.Errorf("basic: %+v, %+v", r.Headers, r.Auth)
	}
	for _, v := range []string{"Basic not-base64", "Digest x=1", "Bearer "} {
		r, _ = ParseCurl(`curl a.dev -H 'Authorization: ` + v + `'`)
		if len(r.Headers) != 1 || r.Auth.Kind != "" {
			t.Errorf("%q: %+v, %+v", v, r.Headers, r.Auth)
		}
	}
	// The first that converts wins; every Authorization header goes.
	r, _ = ParseCurl(`curl a.dev -H 'Authorization: Digest x=1' -H 'Authorization: Bearer a' -H 'authorization: Bearer b' -H 'X: 1'`)
	if len(r.Headers) != 1 || r.Auth != (model.Auth{Kind: model.AuthBearer, Token: "a"}) {
		t.Errorf("several: %+v, %+v", r.Headers, r.Auth)
	}
}

func TestPostmanAuthHeader(t *testing.T) {
	const doc = `{"info": {"name": "A", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
  "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "coll"}]},
  "item": [
    {"name": "over", "request": {"method": "GET", "url": "a.dev",
      "header": [{"key": "Authorization", "value": "Basic YWRhOnB3"}, {"key": "authorization", "value": "Bearer x"}]}},
    {"name": "only", "request": {"method": "GET", "url": "a.dev", "auth": {"type": "noauth"},
      "header": [{"key": "Authorization", "value": "Bearer {{t}}"}]}},
    {"name": "off", "request": {"method": "GET", "url": "a.dev",
      "header": [{"key": "Authorization", "value": "Bearer off", "disabled": true}]}},
    {"name": "digest", "request": {"method": "GET", "url": "a.dev", "auth": {"type": "noauth"},
      "header": [{"key": "Authorization", "value": "Digest x=1"}]}}
  ]}`
	r, err := Parse([]byte(doc), "a.json")
	if err != nil {
		t.Fatal(err)
	}
	req := func(i int) *model.Request { return r.Items[i].Request }
	if h, a := req(0).Headers, req(0).Auth; len(h) != 0 || a != (model.Auth{Kind: model.AuthBasic, Username: "ada", Password: "pw"}) {
		t.Errorf("the header doesn't win over auth: %+v, %+v", h, a)
	}
	if h, a := req(1).Headers, req(1).Auth; len(h) != 0 || a != (model.Auth{Kind: model.AuthBearer, Token: "{{t}}"}) {
		t.Errorf("header only: %+v, %+v", h, a)
	}
	if h, a := req(2).Headers, req(2).Auth; len(h) != 1 || a.Token != "coll" {
		t.Errorf("a disabled header: %+v, %+v", h, a)
	}
	if h, a := req(3).Headers, req(3).Auth; len(h) != 1 || a.Kind != model.AuthNone {
		t.Errorf("digest: %+v, %+v", h, a)
	}
}

func TestCurlRoundTrip(t *testing.T) {
	in := &model.Request{Method: "POST", URL: "https://a.dev/users/:id?x=a b",
		Path:    []model.KV{{Key: "id", Value: "7", Enabled: true}},
		Headers: []model.KV{{Key: "X-Note", Value: "it's", Enabled: true}},
		Body:    model.Body{Kind: model.BodyJSON, Text: `{"a":1}`},
		Auth:    model.Auth{Kind: model.AuthBearer, Token: "t"}}
	cmd, err := Curl(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-X POST", "'https://a.dev/users/7?x=a+b'", `'X-Note: it'\''s'`, "'Authorization: Bearer t'", "'Content-Type: application/json'", `--data-raw '{"a":1}'`} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %s in\n%s", want, cmd)
		}
	}
	out, err := ParseCurl(cmd)
	if err != nil || out.Method != "POST" || out.Body.Text != `{"a":1}` || header(out.Headers, "X-Note") != "it's" || out.Auth != in.Auth {
		t.Errorf("parsed back %+v, %v", out, err)
	}

	cmd, _ = Curl(&model.Request{Method: "POST", URL: "a.dev", Body: model.Body{Kind: model.BodyMultipart,
		Form: []model.KV{{Key: "f", Value: "/no/such", Enabled: true, File: true}}}})
	if !strings.Contains(cmd, "-F 'f=@/no/such'") || strings.Contains(cmd, "Content-Type") {
		t.Errorf("multipart:\n%s", cmd)
	}
}

func readPostman(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/acme.postman_collection.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPostman(t *testing.T) {
	r, err := Parse(readPostman(t), "acme.postman_collection.json")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Acme" || len(r.Items) != 2 || len(r.Env) != 1 || r.Env[0].Value != "https://api.acme.dev" {
		t.Fatalf("result %+v", r)
	}
	if !slices.Equal(r.Skipped, []string{"2 scripts", "1 oauth2 auth"}) {
		t.Errorf("skipped %q", r.Skipped)
	}
	users := r.Items[0]
	get := users.Folder.Children[0].Request
	if get.Method != "GET" || len(get.Query) != 2 || get.Query[1].Enabled || get.Path[0].Value != "7" ||
		len(get.Headers) != 2 || get.Headers[1].Enabled {
		t.Errorf("get user %+v", get)
	}
	if get.Auth.Kind != model.AuthBearer || get.Auth.Token != "{{token}}" {
		t.Errorf("the collection's auth isn't inherited: %+v", get.Auth)
	}
	up := users.Folder.Children[1].Request
	if up.Body.Kind != model.BodyMultipart || !up.Body.Form[1].File || up.Body.Form[1].Value != "/tmp/a.pdf" || up.Auth.Kind != model.AuthNone {
		t.Errorf("upload %+v", up)
	}
	login := r.Items[1].Request
	if login.Body.Kind != model.BodyJSON || login.Auth.Kind != model.AuthBasic || login.Auth.Password != "pw" {
		t.Errorf("login %+v", login)
	}
}

func TestNativeRoundTrip(t *testing.T) {
	f := model.NewFolder("Users")
	r := model.NewRequest("list")
	f.Folder.Children = append(f.Folder.Children, r)
	data, err := Export("Users", []*model.Node{f}, []model.KV{{Key: "base", Value: "x", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data, "whatever.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Users" || len(got.Items) != 1 || got.Items[0].ID == f.ID ||
		got.Items[0].Folder.Children[0].ID == r.ID || got.Env[0].Key != "base" {
		t.Errorf("round trip %+v", got)
	}
	if _, err := Parse([]byte(`{"hello": 1}`), "x.json"); err == nil {
		t.Error("an unknown file")
	}
	if _, err := Parse([]byte(`{"telepath": 99, "items": []}`), "x.json"); err == nil {
		t.Error("a newer format")
	}
}

func TestPostmanRoundTrip(t *testing.T) {
	first, err := Parse(readPostman(t), "acme.postman_collection.json")
	if err != nil {
		t.Fatal(err)
	}
	empty := model.NewFolder("Empty")
	items := append(first.Items, empty)
	data, err := ExportPostman("Acme", items, first.Env)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(data, "x.json")
	if err != nil {
		t.Fatal(err)
	}
	// IDs are new on each import; everything else comes back.
	strip := func(nodes []*model.Node) string {
		w := &model.Workspace{Items: nodes}
		w.Walk(func(n, _ *model.Node) bool { n.ID = ""; return true })
		b, _ := json.Marshal(w)
		return string(b)
	}
	if a, b := strip(items), strip(again.Items); a != b {
		t.Errorf("round trip\n%s\n%s", a, b)
	}
	if !slices.Equal(again.Env, first.Env) || len(again.Skipped) != 0 {
		t.Errorf("env %+v, skipped %q", again.Env, again.Skipped)
	}

	var c struct {
		Info struct{ Schema string }
		Item []map[string]json.RawMessage
	}
	json.Unmarshal(data, &c)
	if c.Info.Schema != pmSchema || string(c.Item[2]["item"]) != "[]" {
		t.Errorf("schema %q, empty folder %s", c.Info.Schema, c.Item[2]["item"])
	}
}

func TestPostmanURLParts(t *testing.T) {
	r := &model.Request{Method: "GET", URL: "https://api.acme.dev:8443/users/:id?a=1#top",
		Query: []model.KV{{Key: "a", Value: "1", Enabled: true}, {Key: "b", Value: "2"}},
		Path:  []model.KV{{Key: "id", Value: "7"}}}
	u := pmOutURLOf(r)
	if u.Protocol != "https" || !slices.Equal(u.Host, []string{"api", "acme", "dev"}) || u.Port != "8443" ||
		!slices.Equal(u.Path, []string{"users", ":id"}) || u.Hash != "top" ||
		len(u.Query) != 2 || !u.Query[1].Disabled || u.Variable[0].Value != "7" {
		t.Errorf("parts %+v", u)
	}
	u = pmOutURLOf(&model.Request{URL: "{{base}}/users"})
	if u.Protocol != "" || !slices.Equal(u.Host, []string{"{{base}}"}) || !slices.Equal(u.Path, []string{"users"}) {
		t.Errorf("a variable host %+v", u)
	}
}
