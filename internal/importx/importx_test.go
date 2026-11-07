package importx

import (
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
	if len(r.Headers) != 2 || r.Headers[1].Value != "Bearer abc" {
		t.Errorf("headers %+v", r.Headers)
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
	if err != nil || out.Method != "POST" || out.Body.Text != `{"a":1}` || header(out.Headers, "X-Note") != "it's" {
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
