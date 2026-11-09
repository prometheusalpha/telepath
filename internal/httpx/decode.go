package httpx

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/htmlindex"
)

// decodeContent undoes the Content-Encoding the transport left in place,
// as it does when the request sets its own Accept-Encoding. Codings are
// listed in the order applied, so they come off from the last. What
// decodes past MaxBody is cut, as a body read past it is.
func decodeContent(body []byte, codings string) ([]byte, bool, error) {
	list := strings.Split(codings, ",")
	for i := len(list) - 1; i >= 0; i-- {
		var r io.Reader
		var err error
		switch coding := strings.ToLower(strings.TrimSpace(list[i])); coding {
		case "", "identity":
			continue
		case "gzip", "x-gzip":
			r, err = gzip.NewReader(bytes.NewReader(body))
		case "deflate":
			// The spec's deflate is zlib-wrapped, but some servers send
			// it bare.
			if r, err = zlib.NewReader(bytes.NewReader(body)); err != nil {
				r, err = flate.NewReader(bytes.NewReader(body)), nil
			}
		case "br":
			r = brotli.NewReader(bytes.NewReader(body))
		case "zstd":
			var d *zstd.Decoder
			d, err = zstd.NewReader(bytes.NewReader(body), zstd.WithDecoderConcurrency(1))
			if err == nil {
				defer d.Close()
				r = d
			}
		default:
			return body, false, fmt.Errorf("unsupported Content-Encoding %q", coding)
		}
		if err != nil {
			return body, false, fmt.Errorf("%s: %w", list[i], err)
		}
		out, err := io.ReadAll(io.LimitReader(r, MaxBody+1))
		// A body cut at MaxBody ends mid-stream: what came out of it
		// still shows.
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return body, false, fmt.Errorf("%s: %w", strings.TrimSpace(list[i]), err)
		}
		if len(out) > MaxBody {
			return out[:MaxBody], true, nil
		}
		body = out
	}
	return body, false, nil
}

// MediaType is the body's type, from Content-Type or, without one, as
// sniffed.
func (r *Response) MediaType() string {
	if mt, _, err := mime.ParseMediaType(r.Header("Content-Type")); err == nil {
		return mt
	}
	mt, _, _ := mime.ParseMediaType(http.DetectContentType(r.Body))
	return mt
}

// Header returns the first value of the header named key.
func (r *Response) Header(key string) string {
	for _, h := range r.Headers {
		if strings.EqualFold(h.Key, key) {
			return h.Value
		}
	}
	return ""
}

// HTML reports whether the body is an HTML page.
func (r *Response) HTML() bool {
	mt := r.MediaType()
	return mt == "text/html" || mt == "application/xhtml+xml"
}

// Previewable reports whether the panel can show the body as a page, an
// image or a document.
func (r *Response) Previewable() bool {
	mt := r.MediaType()
	return r.HTML() || strings.HasPrefix(mt, "image/") || mt == "application/pdf"
}

// Binary reports whether the body is something other than text, which
// has no characters to show.
func (r *Response) Binary() bool {
	mt := r.MediaType()
	switch {
	case mt == "image/svg+xml":
		return false
	case strings.HasPrefix(mt, "image/"), strings.HasPrefix(mt, "audio/"), strings.HasPrefix(mt, "video/"),
		strings.HasPrefix(mt, "font/"), mt == "application/pdf", mt == "application/zip",
		mt == "application/octet-stream" && !utf8.Valid(r.Body):
		return true
	}
	return false
}

// Charset names the encoding the body's text is in, as a browser finds
// it: a byte order mark, the Content-Type, a <meta> of an HTML page, else
// UTF-8 when the bytes are, else windows-1252.
func (r *Response) Charset() string {
	ct := r.Header("Content-Type")
	// Prescanning for <meta charset> is for HTML: other text, JSON
	// first, is UTF-8 unless its bytes can't be.
	if mt, params, err := mime.ParseMediaType(ct); err == nil && params["charset"] == "" && !r.HTML() && utf8.Valid(r.Body) {
		params["charset"] = "utf-8"
		ct = mime.FormatMediaType(mt, params)
	}
	_, name, certain := charset.DetermineEncoding(r.Body, ct)
	// Its guess reads the first KB alone, and falls back to windows-1252
	// for one without a byte past ASCII.
	if name == "" || !certain && name == "windows-1252" && utf8.Valid(r.Body) {
		return "utf-8"
	}
	return name
}

// Decode returns body read as text in the named charset. UTF-8 comes as
// it is, invalid bytes and all.
func Decode(body []byte, name string) (string, error) {
	enc, err := htmlindex.Get(name)
	if err != nil {
		return string(body), err
	}
	if n, _ := htmlindex.Name(enc); n == "utf-8" {
		return string(bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))), nil
	}
	out, err := enc.NewDecoder().Bytes(body)
	if err != nil {
		return string(body), err
	}
	return string(out), nil
}

// PrettyText returns text indented when it is JSON, else as it is.
func PrettyText(text string, isJSON bool) string {
	if isJSON {
		var out bytes.Buffer
		if json.Indent(&out, []byte(text), "", "  ") == nil {
			return out.String()
		}
	}
	return text
}
