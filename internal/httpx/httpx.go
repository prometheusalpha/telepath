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
	"net/http"
	"net/url"
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

func Build(ctx context.Context, r *model.Request) (*http.Request, error) {
	u, err := url.Parse(NormalizeURL(r.URL))
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("the URL has no host")
	}
	return http.NewRequestWithContext(ctx, r.Method, u.String(), nil)
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
