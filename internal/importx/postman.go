package importx

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"telepath/internal/model"
)

// The parts of Postman's collection format v2.1 Telepath reads. The ones
// shared with export leave out what's empty, as Postman's own files do.
type pmCollection struct {
	Info struct {
		Name string `json:"name"`
	} `json:"info"`
	Item     []pmItem `json:"item"`
	Variable []pmKV   `json:"variable"`
	Auth     *pmAuth  `json:"auth"`
	Event    []any    `json:"event"`
}

type pmItem struct {
	Name    string     `json:"name"`
	Item    []pmItem   `json:"item"` // set for folders
	Request *pmRequest `json:"request"`
	Auth    *pmAuth    `json:"auth"`
	Event   []any      `json:"event"`
}

type pmRequest struct {
	Method string  `json:"method"`
	Header []pmKV  `json:"header"`
	URL    pmURL   `json:"url"`
	Body   *pmBody `json:"body"`
	Auth   *pmAuth `json:"auth"`
}

type pmKV struct {
	Key      string `json:"key"`
	Value    any    `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
	Type     string `json:"type,omitempty"` // "file" in form data
	Src      any    `json:"src,omitempty"`  // the file's path, or paths
}

func (kv pmKV) value() string {
	switch v := kv.Value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// pmURL is a string, or an object with the raw string and its parts.
type pmURL struct {
	Raw      string `json:"raw"`
	Query    []pmKV `json:"query"`
	Variable []pmKV `json:"variable"`
}

func (u *pmURL) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		u.Raw = s
		return nil
	}
	type plain pmURL
	return json.Unmarshal(b, (*plain)(u))
}

type pmBody struct {
	Mode       string     `json:"mode"`
	Raw        string     `json:"raw,omitempty"`
	URLEncoded []pmKV     `json:"urlencoded,omitempty"`
	FormData   []pmKV     `json:"formdata,omitempty"`
	Options    *pmOptions `json:"options,omitempty"`
}

type pmOptions struct {
	Raw struct {
		Language string `json:"language"`
	} `json:"raw"`
}

// pmAuth keeps each kind's settings as a list of key/value pairs.
type pmAuth struct {
	Type   string `json:"type"`
	Basic  []pmKV `json:"basic,omitempty"`
	Bearer []pmKV `json:"bearer,omitempty"`
	APIKey []pmKV `json:"apikey,omitempty"`
}

func (a *pmAuth) get(list []pmKV, key string) string {
	for _, kv := range list {
		if kv.Key == key {
			return kv.value()
		}
	}
	return ""
}

func parsePostman(data []byte) (*Result, error) {
	var c pmCollection
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("reading the Postman collection: %w", err)
	}
	p := &pmParser{}
	r := &Result{Name: c.Info.Name}
	p.scripts += len(c.Event)
	for _, it := range c.Item {
		r.Items = append(r.Items, p.item(it, c.Auth))
	}
	for _, v := range c.Variable {
		r.Env = append(r.Env, model.KV{Key: v.Key, Value: v.value(), Enabled: !v.Disabled})
	}
	if p.scripts > 0 {
		r.Skipped = append(r.Skipped, plural(p.scripts, "script"))
	}
	for _, kind := range slices.Sorted(maps.Keys(p.auths)) {
		r.Skipped = append(r.Skipped, plural(p.auths[kind], kind+" auth"))
	}
	return r, nil
}

type pmParser struct {
	scripts int
	auths   map[string]int // kinds of auth Telepath lacks, as "oauth2"
}

// item converts a folder or a request; auth is the nearest folder's, which
// Postman requests inherit and Telepath's, having no inheritance, copy.
func (p *pmParser) item(it pmItem, auth *pmAuth) *model.Node {
	p.scripts += len(it.Event)
	if it.Auth != nil {
		auth = it.Auth
	}
	if it.Request == nil {
		n := model.NewFolder(it.Name)
		for _, k := range it.Item {
			n.Folder.Children = append(n.Folder.Children, p.item(k, auth))
		}
		return n
	}
	n := model.NewRequest(it.Name)
	n.Request = p.request(it.Request, auth)
	return n
}

func (p *pmParser) request(pr *pmRequest, inherited *pmAuth) *model.Request {
	r := &model.Request{Method: strings.ToUpper(pr.Method)}
	if r.Method == "" {
		r.Method = "GET"
	}
	r.URL = pr.URL.Raw
	// Disabled query parameters aren't in the raw URL; the rest come from
	// it, as typing it would make them.
	r.QueryFromURL()
	for _, q := range pr.URL.Query {
		if q.Disabled {
			r.Query = append(r.Query, model.KV{Key: q.Key, Value: q.value()})
		}
	}
	r.PathFromURL()
	for _, v := range pr.URL.Variable {
		for i := range r.Path {
			if r.Path[i].Key == v.Key {
				r.Path[i].Value = v.value()
			}
		}
	}
	for _, h := range pr.Header {
		r.Headers = append(r.Headers, model.KV{Key: h.Key, Value: h.value(), Enabled: !h.Disabled})
	}
	if b := pr.Body; b != nil {
		switch b.Mode {
		case "raw":
			r.Body.Text = b.Raw
			r.Body.Kind = model.BodyText
			if b.Options != nil && b.Options.Raw.Language == "json" || json.Valid([]byte(strings.TrimSpace(b.Raw))) && b.Raw != "" {
				r.Body.Kind = model.BodyJSON
			}
		case "urlencoded":
			r.Body.Kind = model.BodyForm
			for _, kv := range b.URLEncoded {
				r.Body.Form = append(r.Body.Form, model.KV{Key: kv.Key, Value: kv.value(), Enabled: !kv.Disabled})
			}
		case "formdata":
			r.Body.Kind = model.BodyMultipart
			for _, kv := range b.FormData {
				f := model.KV{Key: kv.Key, Value: kv.value(), Enabled: !kv.Disabled}
				if kv.Type == "file" {
					f.File, f.Value = true, firstPath(kv.Src)
				}
				r.Body.Form = append(r.Body.Form, f)
			}
		}
	}
	auth := pr.Auth
	if auth == nil {
		auth = inherited
	}
	r.Auth = p.auth(auth)
	return r
}

func (p *pmParser) auth(a *pmAuth) model.Auth {
	if a == nil {
		return model.Auth{}
	}
	switch a.Type {
	case "", "noauth":
		return model.Auth{}
	case "basic":
		return model.Auth{Kind: model.AuthBasic, Username: a.get(a.Basic, "username"), Password: a.get(a.Basic, "password")}
	case "bearer":
		return model.Auth{Kind: model.AuthBearer, Token: a.get(a.Bearer, "token")}
	case "apikey":
		return model.Auth{Kind: model.AuthAPIKey, Key: a.get(a.APIKey, "key"), Value: a.get(a.APIKey, "value"),
			InQuery: a.get(a.APIKey, "in") == "query"}
	}
	if p.auths == nil {
		p.auths = map[string]int{}
	}
	p.auths[a.Type]++
	return model.Auth{}
}

func firstPath(src any) string {
	switch v := src.(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			s, _ := v[0].(string)
			return s
		}
	}
	return ""
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

const pmSchema = "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"

// What Telepath writes. Folders, even empty ones, have an item list, as
// that is what tells Postman an item is a folder.
type pmOutCollection struct {
	Info struct {
		Name   string `json:"name"`
		Schema string `json:"schema"`
	} `json:"info"`
	Item     []pmOutItem `json:"item"`
	Variable []pmKV      `json:"variable,omitempty"`
}

type pmOutItem struct {
	Name    string        `json:"name"`
	Item    *[]pmOutItem  `json:"item,omitempty"`
	Request *pmOutRequest `json:"request,omitempty"`
}

type pmOutRequest struct {
	Method string   `json:"method"`
	Header []pmKV   `json:"header"`
	Body   *pmBody  `json:"body,omitempty"`
	URL    pmOutURL `json:"url"`
	Auth   *pmAuth  `json:"auth,omitempty"`
}

// pmOutURL has the URL's parts as well as its text: Postman builds the
// request from the parts.
type pmOutURL struct {
	Raw      string   `json:"raw"`
	Protocol string   `json:"protocol,omitempty"`
	Host     []string `json:"host,omitempty"`
	Port     string   `json:"port,omitempty"`
	Path     []string `json:"path,omitempty"`
	Query    []pmKV   `json:"query,omitempty"`
	Hash     string   `json:"hash,omitempty"`
	Variable []pmKV   `json:"variable,omitempty"`
}

// ExportPostman writes items and the variables as a Postman collection
// v2.1, the variables as the collection's.
func ExportPostman(name string, items []*model.Node, env []model.KV) ([]byte, error) {
	var c pmOutCollection
	c.Info.Name, c.Info.Schema = name, pmSchema
	c.Item = pmItems(items)
	for _, kv := range env {
		c.Variable = append(c.Variable, pmKV{Key: kv.Key, Value: kv.Value, Disabled: !kv.Enabled})
	}
	return json.MarshalIndent(c, "", "  ")
}

func pmItems(nodes []*model.Node) []pmOutItem {
	items := []pmOutItem{}
	for _, n := range nodes {
		it := pmOutItem{Name: n.Name}
		if n.Folder != nil {
			kids := pmItems(n.Folder.Children)
			it.Item = &kids
		} else {
			it.Request = pmOut(n.Request)
		}
		items = append(items, it)
	}
	return items
}

func pmOut(r *model.Request) *pmOutRequest {
	out := &pmOutRequest{Method: r.Method, Header: pmKVs(r.Headers), URL: pmOutURLOf(r)}
	switch r.Body.Kind {
	case model.BodyJSON, model.BodyText:
		out.Body = &pmBody{Mode: "raw", Raw: r.Body.Text, Options: &pmOptions{}}
		out.Body.Options.Raw.Language = map[string]string{model.BodyJSON: "json", model.BodyText: "text"}[r.Body.Kind]
	case model.BodyForm:
		out.Body = &pmBody{Mode: "urlencoded", URLEncoded: pmKVs(r.Body.Form)}
	case model.BodyMultipart:
		out.Body = &pmBody{Mode: "formdata", FormData: []pmKV{}}
		for _, kv := range r.Body.Form {
			f := pmKV{Key: kv.Key, Value: kv.Value, Disabled: !kv.Enabled, Type: "text"}
			if kv.File {
				f.Value, f.Type, f.Src = nil, "file", kv.Value
			}
			out.Body.FormData = append(out.Body.FormData, f)
		}
	}
	str := func(k, v string) pmKV { return pmKV{Key: k, Value: v, Type: "string"} }
	switch a := r.Auth; a.Kind {
	case model.AuthBasic:
		out.Auth = &pmAuth{Type: "basic", Basic: []pmKV{str("username", a.Username), str("password", a.Password)}}
	case model.AuthBearer:
		out.Auth = &pmAuth{Type: "bearer", Bearer: []pmKV{str("token", a.Token)}}
	case model.AuthAPIKey:
		in := "header"
		if a.InQuery {
			in = "query"
		}
		out.Auth = &pmAuth{Type: "apikey", APIKey: []pmKV{str("key", a.Key), str("value", a.Value), str("in", in)}}
	}
	return out
}

func pmKVs(list []model.KV) []pmKV {
	out := []pmKV{}
	for _, kv := range list {
		out = append(out, pmKV{Key: kv.Key, Value: kv.Value, Disabled: !kv.Enabled})
	}
	return out
}

// pmOutURLOf splits the URL as Postman does: the host on dots, the path on
// slashes. The query comes from the request's list, which also has the
// disabled parameters the URL leaves out.
func pmOutURLOf(r *model.Request) pmOutURL {
	u := pmOutURL{Raw: r.URL}
	rest := r.URL
	if i := strings.Index(rest, "#"); i >= 0 {
		rest, u.Hash = rest[:i], rest[i+1:]
	}
	rest, _, _ = strings.Cut(rest, "?")
	if p, after, ok := strings.Cut(rest, "://"); ok {
		u.Protocol, rest = p, after
	}
	host, path, hasPath := strings.Cut(rest, "/")
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i:], "]") {
		host, u.Port = host[:i], host[i+1:]
	}
	if host != "" {
		u.Host = strings.Split(host, ".")
	}
	if hasPath {
		u.Path = strings.Split(path, "/")
	}
	for _, q := range r.Query {
		u.Query = append(u.Query, pmKV{Key: q.Key, Value: q.Value, Disabled: !q.Enabled})
	}
	for _, p := range r.Path {
		u.Variable = append(u.Variable, pmKV{Key: p.Key, Value: p.Value})
	}
	return u
}
