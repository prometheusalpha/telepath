package importx

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"telepath/internal/httpx"
	"telepath/internal/model"
)

// splitShell splits a command as a POSIX shell would, which is how
// browsers and docs write cURL commands: quotes, backslashes, lines
// continued with a backslash, and bash's $'…' strings, which Chrome's
// "Copy as cURL" uses.
func splitShell(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\\' && i+1 < len(rs):
			i++
			if rs[i] != '\n' && rs[i] != '\r' {
				cur.WriteRune(rs[i])
				inArg = true
			}
		case r == '\'':
			end := slices.Index(rs[i+1:], '\'')
			if end < 0 {
				return nil, errors.New("a quote ' isn't closed")
			}
			cur.WriteString(string(rs[i+1 : i+1+end]))
			i += end + 1
			inArg = true
		case r == '$' && i+1 < len(rs) && rs[i+1] == '\'':
			j := i + 2
			for ; j < len(rs) && rs[j] != '\''; j++ {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
					switch rs[j] {
					case 'n':
						cur.WriteRune('\n')
					case 't':
						cur.WriteRune('\t')
					case 'r':
						cur.WriteRune('\r')
					default: // \', \\, \"
						cur.WriteRune(rs[j])
					}
					continue
				}
				cur.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, errors.New("a quote $' isn't closed")
			}
			i, inArg = j, true
		case r == '"':
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				if rs[j] == '\\' && j+1 < len(rs) && strings.ContainsRune("\"\\$`\n", rs[j+1]) {
					j++
					if rs[j] == '\n' {
						continue
					}
				}
				cur.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, errors.New(`a quote " isn't closed`)
			}
			i, inArg = j, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}

// Flags of curl that take a value but don't change the request.
var ignoredWithValue = map[string]bool{
	"-o": true, "--output": true, "-m": true, "--max-time": true, "--connect-timeout": true,
	"-w": true, "--write-out": true, "--retry": true, "-x": true, "--proxy": true, "-c": true,
	"--cookie-jar": true, "--cacert": true, "--cert": true, "--key": true, "-E": true,
	"--resolve": true, "--limit-rate": true, "-r": true, "--range": true,
}

// ParseCurl makes a request of a curl command. Flags that don't change
// what is sent, as -s, -L or -k, are ignored.
func ParseCurl(cmd string) (*model.Request, error) {
	args, err := splitShell(strings.TrimSpace(cmd))
	if err != nil {
		return nil, err
	}
	if len(args) == 0 || args[0] != "curl" {
		return nil, errors.New("not a curl command")
	}
	r := &model.Request{}
	var data []string
	var form []model.KV
	get := false
	for i := 1; i < len(args); i++ {
		flag, val, hasVal := args[i], "", false
		// Short flags may hold their value: -XPOST, -H'Accept: x'.
		if len(flag) > 2 && flag[0] == '-' && flag[1] != '-' && strings.ContainsRune("XHduFAbe", rune(flag[1])) {
			flag, val, hasVal = flag[:2], flag[2:], true
		}
		next := func() string {
			if hasVal {
				return val
			}
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch flag {
		case "-X", "--request":
			r.Method = strings.ToUpper(next())
		case "-H", "--header":
			if k, v, ok := strings.Cut(next(), ":"); ok {
				r.Headers = append(r.Headers, model.KV{Key: strings.TrimSpace(k), Value: strings.TrimSpace(v), Enabled: true})
			}
		case "-d", "--data", "--data-raw", "--data-binary", "--data-ascii", "--data-urlencode":
			data = append(data, next())
		case "--json":
			data = append(data, next())
			r.Headers = append(r.Headers, model.KV{Key: "Content-Type", Value: "application/json", Enabled: true},
				model.KV{Key: "Accept", Value: "application/json", Enabled: true})
		case "-F", "--form":
			k, v, _ := strings.Cut(next(), "=")
			kv := model.KV{Key: k, Value: v, Enabled: true}
			if strings.HasPrefix(v, "@") {
				path, _, _ := strings.Cut(v[1:], ";") // ;type=… after the path
				kv.Value, kv.File = path, true
			}
			form = append(form, kv)
		case "-u", "--user":
			u, p, _ := strings.Cut(next(), ":")
			r.Auth = model.Auth{Kind: model.AuthBasic, Username: u, Password: p}
		case "-A", "--user-agent":
			r.Headers = append(r.Headers, model.KV{Key: "User-Agent", Value: next(), Enabled: true})
		case "-b", "--cookie":
			r.Headers = append(r.Headers, model.KV{Key: "Cookie", Value: next(), Enabled: true})
		case "-e", "--referer":
			r.Headers = append(r.Headers, model.KV{Key: "Referer", Value: next(), Enabled: true})
		case "--url":
			r.URL = next()
		case "-G", "--get":
			get = true
		default:
			switch {
			case ignoredWithValue[flag]:
				next()
			case strings.HasPrefix(flag, "-"):
			case r.URL == "":
				r.URL = flag
			}
		}
	}
	if r.URL == "" {
		return nil, errors.New("the command has no URL")
	}
	authFromHeader(r)
	joined := strings.Join(data, "&")
	switch {
	case get && joined != "":
		sep := "?"
		if strings.Contains(r.URL, "?") {
			sep = "&"
		}
		r.URL += sep + joined
	case len(form) > 0:
		r.Body = model.Body{Kind: model.BodyMultipart, Form: form}
		// curl writes the boundary into it; one set by hand has none.
		r.Headers = slices.DeleteFunc(r.Headers, func(h model.KV) bool { return strings.EqualFold(h.Key, "Content-Type") })
	case len(data) > 0:
		r.Body = dataBody(joined, header(r.Headers, "Content-Type"))
	}
	if r.Method == "" {
		r.Method = "GET"
		if r.Body.Kind != model.BodyNone {
			r.Method = "POST"
		}
	}
	r.QueryFromURL()
	r.PathFromURL()
	return r, nil
}

// authFromHeader moves a Basic or Bearer Authorization header into Auth,
// over any Auth already there: the header is what curl or Postman sends.
// The first that converts wins and every other enabled Authorization
// header goes with it. Other schemes, Basic that doesn't decode to
// user:password, and disabled headers stay as headers.
func authFromHeader(r *model.Request) {
	isAuth := func(h model.KV) bool { return h.Enabled && strings.EqualFold(h.Key, "Authorization") }
	for _, h := range r.Headers {
		if !isAuth(h) {
			continue
		}
		if a, ok := authOf(h.Value); ok {
			r.Auth = a
			r.Headers = slices.DeleteFunc(r.Headers, isAuth)
			return
		}
	}
}

func authOf(value string) (model.Auth, bool) {
	scheme, cred, _ := strings.Cut(value, " ")
	cred = strings.TrimSpace(cred)
	switch {
	case cred == "":
	case strings.EqualFold(scheme, "Bearer"):
		return model.Auth{Kind: model.AuthBearer, Token: cred}, true
	case strings.EqualFold(scheme, "Basic"):
		b, err := base64.StdEncoding.DecodeString(cred)
		if u, p, ok := strings.Cut(string(b), ":"); err == nil && ok {
			return model.Auth{Kind: model.AuthBasic, Username: u, Password: p}, true
		}
	}
	return model.Auth{}, false
}

func header(hs []model.KV, key string) string {
	for _, h := range hs {
		if strings.EqualFold(h.Key, key) {
			return h.Value
		}
	}
	return ""
}

// dataBody reads -d data: JSON when it is, a form when curl would send it
// as one (its default type), else text.
func dataBody(data, contentType string) model.Body {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "json") || ct == "" && json.Valid([]byte(strings.TrimSpace(data))) {
		return model.Body{Kind: model.BodyJSON, Text: data}
	}
	if ct == "" || strings.Contains(ct, "x-www-form-urlencoded") {
		if form, ok := parseForm(data); ok {
			return model.Body{Kind: model.BodyForm, Form: form}
		}
	}
	return model.Body{Kind: model.BodyText, Text: data}
}

func parseForm(data string) ([]model.KV, bool) {
	var form []model.KV
	for _, part := range strings.Split(data, "&") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil, false
		}
		uk, err1 := url.QueryUnescape(k)
		uv, err2 := url.QueryUnescape(v)
		if err1 != nil || err2 != nil {
			return nil, false
		}
		form = append(form, model.KV{Key: uk, Value: uv, Enabled: true})
	}
	return form, len(form) > 0
}

// Curl writes the command that sends r, which the caller has filled the
// variables of: its URL, the headers sending would add (auth, the body's
// type) and its body.
func Curl(r *model.Request) (string, error) {
	built := r.Clone()
	multipart := built.Body.Kind == model.BodyMultipart
	if multipart {
		// -F writes those; building them would read the files.
		built.Body = model.Body{}
	}
	req, err := httpx.Build(context.Background(), built)
	if err != nil {
		return "", err
	}
	parts := []string{"curl"}
	if r.Method != "GET" {
		parts = append(parts, "-X "+r.Method)
	}
	parts = append(parts, quote(req.URL.String()))
	for _, k := range slices.Sorted(maps.Keys(req.Header)) {
		for _, v := range req.Header[k] {
			parts = append(parts, "-H "+quote(k+": "+v))
		}
	}
	switch {
	case multipart:
		for _, f := range r.Body.Form {
			if !f.Enabled || f.Key == "" {
				continue
			}
			v := f.Value
			if f.File {
				v = "@" + v
			}
			parts = append(parts, "-F "+quote(f.Key+"="+v))
		}
	case req.Body != nil && req.Body != http.NoBody:
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return "", err
		}
		if len(b) > 0 {
			parts = append(parts, "--data-raw "+quote(string(b)))
		}
	}
	return strings.Join(parts, " \\\n  "), nil
}

// quote makes s one shell word, in single quotes.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
