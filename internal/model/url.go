package model

import (
	"slices"
	"strings"
)

// The query is kept as typed, unescaped and with {{variables}} in it:
// escaping waits for sending, so the URL and the Params table show the
// same text.

// SplitURL cuts a URL at its query.
func SplitURL(raw string) (base, query string) {
	base, query, _ = strings.Cut(raw, "?")
	return base, query
}

func ParseQuery(q string) []KV {
	var kvs []KV
	for _, part := range strings.Split(q, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		kvs = append(kvs, KV{Key: k, Value: v, Enabled: true})
	}
	return kvs
}

// EncodeQuery joins the enabled pairs; a key without a value goes as the
// bare key, as "?debug".
func EncodeQuery(kvs []KV) string {
	var parts []string
	for _, kv := range kvs {
		if !kv.Enabled || kv.Key == "" && kv.Value == "" {
			continue
		}
		if kv.Value == "" {
			parts = append(parts, kv.Key)
		} else {
			parts = append(parts, kv.Key+"="+kv.Value)
		}
	}
	return strings.Join(parts, "&")
}

// QueryFromURL makes Query follow an edited URL: its pairs are the URL's,
// and the disabled ones, which the URL can't show, stay where they were.
func (r *Request) QueryFromURL() {
	_, q := SplitURL(r.URL)
	next := ParseQuery(q)
	for i, kv := range r.Query {
		if !kv.Enabled {
			next = slices.Insert(next, min(i, len(next)), kv)
		}
	}
	r.Query = next
}

// URLFromQuery writes the enabled pairs of an edited Query into the URL.
func (r *Request) URLFromQuery() {
	base, _ := SplitURL(r.URL)
	if q := EncodeQuery(r.Query); q != "" {
		base += "?" + q
	}
	r.URL = base
}

// PathNames returns the names of the URL path's :name segments, in order.
// The host is skipped, so the port of "localhost:8080" isn't one.
func PathNames(raw string) []string {
	base, _ := SplitURL(raw)
	if _, rest, ok := strings.Cut(base, "://"); ok {
		base = rest
	}
	_, path, ok := strings.Cut(base, "/")
	if !ok {
		return nil
	}
	var names []string
	for _, seg := range strings.Split(path, "/") {
		if len(seg) > 1 && seg[0] == ':' && !slices.Contains(names, seg[1:]) {
			names = append(names, seg[1:])
		}
	}
	return names
}

// PathFromURL makes Path hold the URL's :name segments, keeping the values
// of names that stay.
func (r *Request) PathFromURL() {
	names := PathNames(r.URL)
	next := make([]KV, 0, len(names))
	for _, name := range names {
		kv := KV{Key: name, Enabled: true}
		if i := slices.IndexFunc(r.Path, func(p KV) bool { return p.Key == name }); i >= 0 {
			kv.Value = r.Path[i].Value
		}
		next = append(next, kv)
	}
	r.Path = next
}
