package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"telepath/internal/model"
)

func TestSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write([]byte(`{"method":"` + r.Method + `"}`))
	}))
	defer srv.Close()
	resp, err := Send(context.Background(), srv.Client(), &model.Request{Method: "POST", URL: srv.URL + "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 || resp.Status != "201 Created" || !resp.JSON() {
		t.Errorf("status %q, json %v", resp.Status, resp.JSON())
	}
	if want := "{\n  \"method\": \"POST\"\n}"; resp.Pretty() != want {
		t.Errorf("pretty %q", resp.Pretty())
	}
}

func TestCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Send(ctx, srv.Client(), &model.Request{Method: "GET", URL: srv.URL}); err == nil {
		t.Error("a canceled request succeeded")
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"localhost:8080/x":  "http://localhost:8080/x",
		" https://a.dev ":   "https://a.dev",
		"example.com/a?b=1": "http://example.com/a?b=1",
		"":                  "",
	} {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := Build(context.Background(), &model.Request{Method: "GET", URL: "http://"}); err == nil || !strings.Contains(err.Error(), "host") {
		t.Errorf("no host: %v", err)
	}
}

func TestResolveURL(t *testing.T) {
	r := &model.Request{
		URL:  "localhost:8080/users/:id/:idx?q=a b&tag=x%2Fy&flag",
		Path: []model.KV{{Key: "id", Value: "a/b"}, {Key: "idx", Value: "2"}},
	}
	want := "http://localhost:8080/users/a%2Fb/2?q=a+b&tag=x%2Fy&flag"
	if got := ResolveURL(r); got != want {
		t.Errorf("ResolveURL = %q, want %q", got, want)
	}
}

func TestBodies(t *testing.T) {
	var got struct{ ct, body, hdr string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.ct, got.body, got.hdr = r.Header.Get("Content-Type"), string(b), r.Header.Get("X-Token")
	}))
	defer srv.Close()
	send := func(r *model.Request) {
		t.Helper()
		r.Method, r.URL = "POST", srv.URL
		if _, err := Send(context.Background(), srv.Client(), r); err != nil {
			t.Fatal(err)
		}
	}

	send(&model.Request{
		Headers: []model.KV{{Key: "X-Token", Value: "t", Enabled: true}, {Key: "X-Off", Value: "x"}},
		Body:    model.Body{Kind: model.BodyJSON, Text: `{"a":1}`},
	})
	if got.ct != "application/json" || got.body != `{"a":1}` || got.hdr != "t" {
		t.Errorf("json: %+v", got)
	}

	send(&model.Request{
		Headers: []model.KV{{Key: "Content-Type", Value: "application/vnd.api+json", Enabled: true}},
		Body:    model.Body{Kind: model.BodyJSON, Text: `{}`},
	})
	if got.ct != "application/vnd.api+json" {
		t.Errorf("the user's Content-Type was replaced by %q", got.ct)
	}

	send(&model.Request{Body: model.Body{Kind: model.BodyForm, Form: []model.KV{
		{Key: "name", Value: "Ada L", Enabled: true}, {Key: "off", Value: "x"},
	}}})
	if got.ct != "application/x-www-form-urlencoded" || got.body != "name=Ada+L" {
		t.Errorf("form: %+v", got)
	}

	path := filepath.Join(t.TempDir(), "note.txt")
	os.WriteFile(path, []byte("hello file"), 0o644)
	send(&model.Request{Body: model.Body{Kind: model.BodyMultipart, Form: []model.KV{
		{Key: "title", Value: "x", Enabled: true}, {Key: "doc", Value: path, Enabled: true, File: true},
	}}})
	if !strings.HasPrefix(got.ct, "multipart/form-data; boundary=") ||
		!strings.Contains(got.body, `filename="note.txt"`) || !strings.Contains(got.body, "hello file") {
		t.Errorf("multipart: %+v", got)
	}

	_, err := Send(context.Background(), srv.Client(), &model.Request{Method: "POST", URL: srv.URL,
		Body: model.Body{Kind: model.BodyMultipart, Form: []model.KV{{Key: "doc", Value: "/nope", Enabled: true, File: true}}}})
	if err == nil || !strings.Contains(err.Error(), `"doc"`) {
		t.Errorf("missing file: %v", err)
	}
}

func TestAuth(t *testing.T) {
	build := func(r *model.Request) *http.Request {
		t.Helper()
		r.Method = "GET"
		req, err := Build(context.Background(), r)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}

	req := build(&model.Request{URL: "a.dev", Auth: model.Auth{Kind: model.AuthBasic, Username: "ada", Password: "pw"}})
	if u, p, ok := req.BasicAuth(); !ok || u != "ada" || p != "pw" {
		t.Errorf("basic: %q", req.Header.Get("Authorization"))
	}

	req = build(&model.Request{URL: "a.dev", Auth: model.Auth{Kind: model.AuthBearer, Token: "tok"}})
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("bearer: %q", got)
	}

	req = build(&model.Request{URL: "a.dev", Auth: model.Auth{Kind: model.AuthBearer, Token: "tok"},
		Headers: []model.KV{{Key: "Authorization", Value: "Custom x", Enabled: true}}})
	if got := req.Header.Get("Authorization"); got != "Custom x" {
		t.Errorf("a hand-set Authorization was replaced by %q", got)
	}

	req = build(&model.Request{URL: "a.dev", Auth: model.Auth{Kind: model.AuthAPIKey, Key: "X-API-Key", Value: "k"}})
	if got := req.Header.Get("X-API-Key"); got != "k" {
		t.Errorf("api key header: %q", got)
	}

	req = build(&model.Request{URL: "a.dev/x?b=2&a=1", Auth: model.Auth{Kind: model.AuthAPIKey, Key: "api key", Value: "k&v", InQuery: true}})
	if got := req.URL.RawQuery; got != "b=2&a=1&api+key=k%26v" {
		t.Errorf("api key query: %q", got)
	}

	// Kinds keep what was typed for others, but send only their own.
	req = build(&model.Request{URL: "a.dev", Auth: model.Auth{Kind: model.AuthNone, Token: "tok"}})
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("none sent %q", got)
	}
}
