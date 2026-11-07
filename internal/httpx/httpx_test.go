package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
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
