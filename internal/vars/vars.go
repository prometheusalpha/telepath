// Package vars fills {{name}} references with the workspace's variables.
package vars

import (
	"slices"
	"strings"
	"unicode/utf8"

	"telepath/internal/model"
)

// Ref is a {{name}} in a text, Start and End in runes, as inputs color
// ranges.
type Ref struct {
	Start, End int
	Name       string
}

// scan calls fn for each reference of s, with the bytes it spans; spaces
// inside the braces are allowed, as {{ base }}.
func scan(s string, fn func(open, end int, name string)) {
	for i := 0; ; {
		open := strings.Index(s[i:], "{{")
		if open < 0 {
			return
		}
		open += i
		close := strings.Index(s[open+2:], "}}")
		if close < 0 {
			return
		}
		end := open + 2 + close + 2
		if name := strings.TrimSpace(s[open+2 : end-2]); name != "" && !strings.Contains(name, "{{") {
			fn(open, end, name)
		}
		i = end
	}
}

func Refs(s string) []Ref {
	var refs []Ref
	scan(s, func(open, end int, name string) {
		start := utf8.RuneCountInString(s[:open])
		refs = append(refs, Ref{Start: start, End: start + utf8.RuneCountInString(s[open:end]), Name: name})
	})
	return refs
}

// Env looks variables up; disabled and unnamed ones don't count.
type Env map[string]string

func NewEnv(kvs []model.KV) Env {
	env := Env{}
	for _, kv := range kvs {
		if kv.Enabled && kv.Key != "" {
			env[kv.Key] = kv.Value
		}
	}
	return env
}

// Expand fills the references of s; unknown ones stay as typed and are
// added to missing. Values are not expanded again, so a variable can't
// loop into itself.
func (e Env) Expand(s string, missing *[]string) string {
	var b strings.Builder
	last := 0
	scan(s, func(open, end int, name string) {
		v, ok := e[name]
		if !ok {
			if missing != nil && !slices.Contains(*missing, name) {
				*missing = append(*missing, name)
			}
			return
		}
		b.WriteString(s[last:open])
		b.WriteString(v)
		last = end
	})
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// Request returns a copy of r with every text that is sent expanded, and
// the names it found no variable for.
func (e Env) Request(r *model.Request) (*model.Request, []string) {
	var missing []string
	x := func(s *string) { *s = e.Expand(*s, &missing) }
	kvs := func(list []model.KV) {
		for i := range list {
			x(&list[i].Key)
			x(&list[i].Value)
		}
	}
	cp := r.Clone()
	x(&cp.URL)
	kvs(cp.Path)
	kvs(cp.Headers)
	x(&cp.Body.Text)
	kvs(cp.Body.Form)
	for _, s := range []*string{&cp.Auth.Username, &cp.Auth.Password, &cp.Auth.Token, &cp.Auth.Key, &cp.Auth.Value} {
		x(s)
	}
	// Query is the URL's: sending reads the URL, already expanded.
	return cp, missing
}
