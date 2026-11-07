package vars

import (
	"slices"
	"testing"

	"telepath/internal/model"
)

func TestRefs(t *testing.T) {
	got := Refs("é{{base}}/x/{{ id }}?{{}}&{{open")
	want := []Ref{{1, 9, "base"}, {12, 20, "id"}}
	if !slices.Equal(got, want) {
		t.Errorf("Refs = %+v, want %+v", got, want)
	}
}

func TestExpand(t *testing.T) {
	env := NewEnv([]model.KV{
		{Key: "base", Value: "https://a.dev", Enabled: true},
		{Key: "self", Value: "{{self}}", Enabled: true},
		{Key: "off", Value: "x"},
	})
	var missing []string
	got := env.Expand("{{base}}/{{ self }}/{{off}}/{{off}}/{{nope", &missing)
	if got != "https://a.dev/{{self}}/{{off}}/{{off}}/{{nope" || !slices.Equal(missing, []string{"off"}) {
		t.Errorf("Expand = %q, missing %q", got, missing)
	}
}

func TestRequest(t *testing.T) {
	env := NewEnv([]model.KV{{Key: "host", Value: "a.dev", Enabled: true}, {Key: "tok", Value: "t1", Enabled: true}})
	r := &model.Request{
		URL:     "{{host}}/users/:id?q={{q}}",
		Path:    []model.KV{{Key: "id", Value: "{{tok}}", Enabled: true}},
		Headers: []model.KV{{Key: "X-{{tok}}", Value: "{{tok}}", Enabled: true}},
		Body:    model.Body{Kind: model.BodyJSON, Text: `{"t":"{{tok}}"}`},
		Auth:    model.Auth{Kind: model.AuthBearer, Token: "{{tok}}"},
	}
	out, missing := env.Request(r)
	if out.URL != "a.dev/users/:id?q={{q}}" || out.Path[0].Value != "t1" || out.Headers[0].Key != "X-t1" ||
		out.Body.Text != `{"t":"t1"}` || out.Auth.Token != "t1" || !slices.Equal(missing, []string{"q"}) {
		t.Errorf("expanded %+v, missing %q", out, missing)
	}
	if r.Headers[0].Value != "{{tok}}" {
		t.Error("expanding changed the request itself")
	}
}
