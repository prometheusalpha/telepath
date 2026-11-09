package httpx

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/text/encoding/japanese"

	"telepath/internal/model"
)

func compress(t *testing.T, coding string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w io.WriteCloser
	switch coding {
	case "gzip":
		w = gzip.NewWriter(&buf)
	case "deflate":
		w = zlib.NewWriter(&buf)
	case "raw-deflate":
		w, _ = flate.NewWriter(&buf, flate.DefaultCompression)
	case "br":
		w = brotli.NewWriter(&buf)
	case "zstd":
		w, _ = zstd.NewWriter(&buf)
	}
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func TestDecodeContent(t *testing.T) {
	text := []byte(strings.Repeat("hello, world ", 50))
	for _, coding := range []string{"gzip", "deflate", "raw-deflate", "br", "zstd"} {
		header := coding
		if coding == "raw-deflate" {
			header = "deflate"
		}
		got, cut, err := decodeContent(compress(t, coding, text), header)
		if err != nil || cut || !bytes.Equal(got, text) {
			t.Errorf("%s: %q, cut %v, %v", coding, got, cut, err)
		}
	}
	// Applied gzip, then br: br comes off first.
	both := compress(t, "br", compress(t, "gzip", text))
	if got, _, err := decodeContent(both, "gzip, br"); err != nil || !bytes.Equal(got, text) {
		t.Errorf("gzip, br: %q, %v", got, err)
	}
	if got, _, err := decodeContent(text, "identity"); err != nil || !bytes.Equal(got, text) {
		t.Errorf("identity: %q, %v", got, err)
	}
	if got, _, err := decodeContent(text, "compress"); err == nil || !bytes.Equal(got, text) {
		t.Errorf("an unknown coding: %q, %v", got, err)
	}
	if got, _, err := decodeContent(text, "gzip"); err == nil || !bytes.Equal(got, text) {
		t.Errorf("a body that isn't gzip: %q, %v", got, err)
	}
}

func TestDecodeContentStopsAtMaxBody(t *testing.T) {
	zeros := compress(t, "gzip", make([]byte, MaxBody+100))
	got, cut, err := decodeContent(zeros, "gzip")
	if err != nil || !cut || len(got) != MaxBody {
		t.Errorf("len %d, cut %v, %v", len(got), cut, err)
	}
}

// The transport decodes gzip it asked for itself, but leaves what a
// request asked for by its own Accept-Encoding.
func TestSendDecodesContentEncoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(compress(t, "br", []byte("squeezed")))
	}))
	defer srv.Close()
	resp, err := Send(context.Background(), srv.Client(), &model.Request{Method: "GET", URL: srv.URL,
		Headers: []model.KV{{Key: "Accept-Encoding", Value: "br", Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "squeezed" || resp.Encoding != "br" || resp.EncodingErr != nil {
		t.Errorf("body %q, encoding %q, %v", resp.Body, resp.Encoding, resp.EncodingErr)
	}
	if resp.Size == int64(len("squeezed")) {
		t.Error("the size is of the body decoded, not as received")
	}
}

func TestCharset(t *testing.T) {
	sjis, _ := japanese.ShiftJIS.NewEncoder().Bytes([]byte("こんにちは"))
	ascii := strings.Repeat("a", 2000)
	for _, c := range []struct {
		name, ctype string
		body        []byte
		want        string
	}{
		{"by its header", "text/plain; charset=Shift_JIS", sjis, "shift_jis"},
		{"by a meta of HTML", "text/html", append([]byte(`<meta charset="shift_jis">`), sjis...), "shift_jis"},
		{"a meta outside HTML counts not", "text/plain", []byte(`<meta charset="shift_jis">ok`), "utf-8"},
		{"JSON", "application/json", []byte(`{"a":"ờ"}`), "utf-8"},
		{"UTF-8 past the first KB", "text/html", []byte(ascii + "ờ"), "utf-8"},
		{"bytes that aren't UTF-8", "text/html", []byte(ascii + "\xe9"), "windows-1252"},
		{"a BOM", "text/plain; charset=latin1", []byte("\xef\xbb\xbfok"), "utf-8"},
	} {
		r := &Response{Headers: []Header{{"Content-Type", c.ctype}}, Body: c.body}
		if got := r.Charset(); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDecode(t *testing.T) {
	sjis, _ := japanese.ShiftJIS.NewEncoder().Bytes([]byte("こんにちは"))
	if got, err := Decode(sjis, "shift_jis"); err != nil || got != "こんにちは" {
		t.Errorf("shift_jis: %q, %v", got, err)
	}
	if got, err := Decode([]byte("\xef\xbb\xbfhi"), "utf-8"); err != nil || got != "hi" {
		t.Errorf("utf-8: %q, %v", got, err)
	}
	if got, err := Decode([]byte("caf\xe9"), "windows-1252"); err != nil || got != "café" {
		t.Errorf("windows-1252: %q, %v", got, err)
	}
	if _, err := Decode([]byte("x"), "nope"); err == nil {
		t.Error("an unknown charset decoded")
	}
}

func TestResponseKinds(t *testing.T) {
	for _, c := range []struct {
		ctype               string
		body                string
		html, binary, shown bool
	}{
		{"text/html; charset=utf-8", "<p>", true, false, true},
		{"", "<!DOCTYPE html><p>", true, false, true},
		{"image/png", "\x89PNG", false, true, true},
		{"image/svg+xml", "<svg/>", false, false, true},
		{"application/pdf", "%PDF", false, true, true},
		{"application/json", "{}", false, false, false},
		{"application/octet-stream", "\xff\xfe\x00", false, true, false},
	} {
		r := &Response{Body: []byte(c.body)}
		if c.ctype != "" {
			r.Headers = []Header{{"Content-Type", c.ctype}}
		}
		if r.HTML() != c.html || r.Binary() != c.binary || r.Previewable() != c.shown {
			t.Errorf("%q: html %v, binary %v, previewable %v", c.ctype, r.HTML(), r.Binary(), r.Previewable())
		}
	}
}
