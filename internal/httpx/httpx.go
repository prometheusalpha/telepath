// Package httpx sends a model.Request and reads back what the response
// panel shows.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"telepath/internal/model"
)

const Timeout = 30 * time.Second

// MaxBody caps what is read into memory; the panel says when it cut.
const MaxBody = 20 << 20

type Header struct{ Key, Value string }

type Response struct {
	StatusCode int
	Status     string // "200 OK"
	Headers    []Header
	Body       []byte
	Truncated  bool
	Duration   time.Duration
	Size       int64 // bytes of body received, before truncating
	URL        string
}

// JSON reports whether the body is JSON, by its type or, for servers that
// send text/plain, by parsing.
func (r *Response) JSON() bool {
	for _, h := range r.Headers {
		if strings.EqualFold(h.Key, "Content-Type") {
			if mt, _, _ := mime.ParseMediaType(h.Value); mt == "application/json" || strings.HasSuffix(mt, "+json") {
				return true
			}
		}
	}
	return json.Valid(bytes.TrimSpace(r.Body)) && len(bytes.TrimSpace(r.Body)) > 0
}

// Pretty returns the body indented when it is JSON, else as it came.
func (r *Response) Pretty() string {
	if r.JSON() {
		var out bytes.Buffer
		if json.Indent(&out, r.Body, "", "  ") == nil {
			return out.String()
		}
	}
	return string(r.Body)
}

// NormalizeURL adds http:// to a URL typed without a scheme, as
// "localhost:8080/x", which net/url would read as scheme "localhost".
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "://") {
		return raw
	}
	return "http://" + raw
}

// ResolveURL fills the :name segments and escapes the query, which the
// app keeps as typed.
func ResolveURL(r *model.Request) string {
	base, q := model.SplitURL(NormalizeURL(r.URL))
	// Whole segments of the path only, so neither ":idx" for :id nor the
	// port after the host is taken for one.
	if i := strings.Index(base, "://"); i >= 0 {
		if j := strings.IndexByte(base[i+3:], '/'); j >= 0 {
			host, segs := base[:i+3+j], strings.Split(base[i+3+j:], "/")
			for k, seg := range segs {
				if p, ok := pathValue(r.Path, seg); ok {
					segs[k] = url.PathEscape(p)
				}
			}
			base = host + strings.Join(segs, "/")
		}
	}
	var parts []string
	for _, kv := range model.ParseQuery(q) {
		part := url.QueryEscape(unescape(kv.Key))
		if kv.Value != "" {
			part += "=" + url.QueryEscape(unescape(kv.Value))
		}
		parts = append(parts, part)
	}
	if len(parts) > 0 {
		base += "?" + strings.Join(parts, "&")
	}
	return base
}

func pathValue(path []model.KV, seg string) (string, bool) {
	if len(seg) < 2 || seg[0] != ':' {
		return "", false
	}
	for _, p := range path {
		if p.Key == seg[1:] {
			return p.Value, true
		}
	}
	return "", false
}

// unescape takes back what the user escaped by hand, so it isn't escaped
// twice; text that isn't valid escaping goes as it is.
func unescape(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return s
}

func Build(ctx context.Context, r *model.Request) (*http.Request, error) {
	u, err := url.Parse(ResolveURL(r))
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("the URL has no host")
	}
	body, contentType, err := encodeBody(&r.Body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), body)
	if err != nil {
		return nil, err
	}
	for _, h := range r.Headers {
		if h.Enabled && h.Key != "" {
			req.Header.Add(h.Key, h.Value)
		}
	}
	// A Content-Type the user set wins, even over a multipart boundary.
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

func encodeBody(b *model.Body) (io.Reader, string, error) {
	switch b.Kind {
	case model.BodyJSON:
		return strings.NewReader(b.Text), "application/json", nil
	case model.BodyText:
		return strings.NewReader(b.Text), "text/plain; charset=utf-8", nil
	case model.BodyForm:
		var parts []string
		for _, kv := range b.Form {
			if kv.Enabled && kv.Key != "" {
				parts = append(parts, url.QueryEscape(kv.Key)+"="+url.QueryEscape(kv.Value))
			}
		}
		return strings.NewReader(strings.Join(parts, "&")), "application/x-www-form-urlencoded", nil
	case model.BodyMultipart:
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for _, kv := range b.Form {
			if !kv.Enabled || kv.Key == "" {
				continue
			}
			if !kv.File {
				if err := w.WriteField(kv.Key, kv.Value); err != nil {
					return nil, "", err
				}
				continue
			}
			if err := writeFile(w, kv.Key, kv.Value); err != nil {
				return nil, "", err
			}
		}
		if err := w.Close(); err != nil {
			return nil, "", err
		}
		return &buf, w.FormDataContentType(), nil
	}
	return nil, "", nil
}

func writeFile(w *multipart.Writer, field, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("field %q: %w", field, err)
	}
	defer f.Close()
	part, err := w.CreateFormFile(field, filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}

func Send(ctx context.Context, client *http.Client, r *model.Request) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := Build(ctx, r)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return nil, err
	}
	out := &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Duration:   time.Since(start),
		Size:       int64(len(body)),
		URL:        req.URL.String(),
	}
	if len(body) > MaxBody {
		body, out.Truncated = body[:MaxBody], true
	}
	out.Body = body
	for k, vs := range resp.Header {
		for _, v := range vs {
			out.Headers = append(out.Headers, Header{k, v})
		}
	}
	sort.SliceStable(out.Headers, func(i, j int) bool { return out.Headers[i].Key < out.Headers[j].Key })
	return out, nil
}

func FormatSize(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2f s", d.Seconds())
}
