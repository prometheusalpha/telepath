package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"telepath/internal/httpx"
)

func TestResponses(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "responses")
	if r, err := LoadResponse(dir, "a"); r != nil || err != nil {
		t.Fatalf("none kept: %v, %v", r, err)
	}
	at := time.Date(2026, 11, 18, 16, 30, 0, 0, time.UTC)
	want := &httpx.Response{StatusCode: 200, Status: "200 OK", Headers: []httpx.Header{{Key: "Content-Type", Value: "image/png"}},
		Body: []byte{0x89, 'P', 'N', 'G', 0}, Duration: 42 * time.Millisecond, Size: 3, URL: "http://x/y",
		Encoding: "gzip", EncodingErr: errors.New("bad"), Received: at}
	if err := SaveResponse(dir, "a", want); err != nil {
		t.Fatal(err)
	}
	SaveResponse(dir, "b", want)
	got, err := LoadResponse(dir, "a")
	if err != nil || got == nil {
		t.Fatalf("loaded %v, %v", got, err)
	}
	if string(got.Body) != string(want.Body) || got.Status != "200 OK" || got.Headers[0].Value != "image/png" ||
		got.Duration != want.Duration || got.Size != 3 || got.EncodingErr.Error() != "bad" || !got.Received.Equal(at) {
		t.Errorf("loaded %+v", got)
	}
	if r, _ := LoadResponse(dir, "../a"); r != nil {
		t.Error("read outside the directory")
	}

	// A body from another save than its metadata's is no response.
	os.WriteFile(filepath.Join(dir, "a.body"), []byte("another body"), 0o644)
	if r, err := LoadResponse(dir, "a"); r != nil || err != nil {
		t.Errorf("mismatched body: %v, %v", r, err)
	}

	if err := PruneResponses(dir, func(id string) bool { return id == "b" }); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(dir); len(left) != 2 {
		t.Errorf("left %d files", len(left))
	}
	if r, _ := LoadResponse(dir, "b"); r == nil {
		t.Error("pruned a kept request's response")
	}
}
