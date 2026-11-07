package model

import (
	"slices"
	"testing"
)

func TestQuerySync(t *testing.T) {
	r := &Request{URL: "https://a.dev/users?page=1&debug"}
	r.QueryFromURL()
	if len(r.Query) != 2 || r.Query[0] != (KV{Key: "page", Value: "1", Enabled: true}) || r.Query[1].Key != "debug" {
		t.Fatalf("query %+v", r.Query)
	}

	// Disabling a pair takes it out of the URL but keeps it in the table.
	r.Query[0].Enabled = false
	r.URLFromQuery()
	if r.URL != "https://a.dev/users?debug" {
		t.Fatalf("url %q", r.URL)
	}
	r.URL += "&limit={{n}}"
	r.QueryFromURL()
	if len(r.Query) != 3 || r.Query[0].Key != "page" || r.Query[0].Enabled || r.Query[2].Value != "{{n}}" {
		t.Fatalf("query after typing %+v", r.Query)
	}

	r.Query[1].Enabled, r.Query[2].Enabled = false, false
	r.URLFromQuery()
	if r.URL != "https://a.dev/users" {
		t.Errorf("all disabled: url %q", r.URL)
	}
}

func TestPathNames(t *testing.T) {
	for raw, want := range map[string][]string{
		"http://localhost:8080/users/:id/posts/:post": {"id", "post"},
		"localhost:8080/:id?x=:y":                     {"id"},
		"{{base}}/orgs/:org/:org":                     {"org"},
		"https://a.dev":                               nil,
		"https://a.dev/:":                             nil,
	} {
		if got := PathNames(raw); !slices.Equal(got, want) {
			t.Errorf("PathNames(%q) = %q, want %q", raw, got, want)
		}
	}
	r := &Request{URL: "/users/:id", Path: []KV{{Key: "id", Value: "7", Enabled: true}, {Key: "gone", Value: "x"}}}
	r.URL = "/orgs/:org/users/:id"
	r.PathFromURL()
	if len(r.Path) != 2 || r.Path[0].Key != "org" || r.Path[1].Value != "7" {
		t.Errorf("path %+v", r.Path)
	}
}
