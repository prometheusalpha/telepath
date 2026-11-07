package importx

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"telepath/internal/model"
)

// The parts of Postman's collection format v2.1 Telepath reads.
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
	Disabled bool   `json:"disabled"`
	Type     string `json:"type"` // "file" in form data
	Src      any    `json:"src"`  // the file's path, or paths
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
	Mode       string `json:"mode"`
	Raw        string `json:"raw"`
	URLEncoded []pmKV `json:"urlencoded"`
	FormData   []pmKV `json:"formdata"`
	Options    struct {
		Raw struct {
			Language string `json:"language"`
		} `json:"raw"`
	} `json:"options"`
}

// pmAuth keeps each kind's settings as a list of key/value pairs.
type pmAuth struct {
	Type   string `json:"type"`
	Basic  []pmKV `json:"basic"`
	Bearer []pmKV `json:"bearer"`
	APIKey []pmKV `json:"apikey"`
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
			if b.Options.Raw.Language == "json" || json.Valid([]byte(strings.TrimSpace(b.Raw))) && b.Raw != "" {
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
