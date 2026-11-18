package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"telepath/internal/httpx"
)

// The last response of each request is two files in a directory beside
// the workspace: <id>.json for what isn't the body, and <id>.body for the
// body as received, which JSON would grow by a third in base64.

// responses serializes the writes, as a send may finish while the
// previous one's files are still being written.
var responses sync.Mutex

type savedResponse struct {
	Received    time.Time      `json:"received"`
	StatusCode  int            `json:"statusCode"`
	Status      string         `json:"status"`
	Headers     []httpx.Header `json:"headers"`
	Truncated   bool           `json:"truncated,omitempty"`
	Duration    time.Duration  `json:"duration"`
	Size        int64          `json:"size"`
	URL         string         `json:"url"`
	Encoding    string         `json:"encoding,omitempty"`
	EncodingErr string         `json:"encodingErr,omitempty"`
	// BodyLen tells a body file left from another save, as after a crash
	// between the two writes.
	BodyLen int `json:"bodyLen"`
}

// fileID refuses IDs that would reach outside dir; the app's are hex.
func fileID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\`) && id != "." && id != ".."
}

// SaveResponse keeps r as the last response of the request id.
func SaveResponse(dir, id string, r *httpx.Response) error {
	if !fileID(id) {
		return nil
	}
	s := savedResponse{Received: r.Received, StatusCode: r.StatusCode, Status: r.Status, Headers: r.Headers,
		Truncated: r.Truncated, Duration: r.Duration, Size: r.Size, URL: r.URL, Encoding: r.Encoding, BodyLen: len(r.Body)}
	if r.EncodingErr != nil {
		s.EncodingErr = r.EncodingErr.Error()
	}
	meta, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	responses.Lock()
	defer responses.Unlock()
	base := filepath.Join(dir, id)
	if err := write(base+".body", r.Body); err != nil {
		return err
	}
	return write(base+".json", meta)
}

// LoadResponse returns the last response kept for id, or nil when there
// is none or its files don't match.
func LoadResponse(dir, id string) (*httpx.Response, error) {
	if !fileID(id) {
		return nil, nil
	}
	responses.Lock()
	defer responses.Unlock()
	base := filepath.Join(dir, id)
	meta, err := os.ReadFile(base + ".json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s savedResponse
	if err := json.Unmarshal(meta, &s); err != nil {
		return nil, err
	}
	body, err := os.ReadFile(base + ".body")
	if errors.Is(err, fs.ErrNotExist) || err == nil && len(body) != s.BodyLen {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r := &httpx.Response{Received: s.Received, StatusCode: s.StatusCode, Status: s.Status, Headers: s.Headers, Body: body,
		Truncated: s.Truncated, Duration: s.Duration, Size: s.Size, URL: s.URL, Encoding: s.Encoding}
	if s.EncodingErr != "" {
		r.EncodingErr = errors.New(s.EncodingErr)
	}
	return r, nil
}

// PruneResponses removes the responses of requests keep doesn't know,
// deleted since they were saved.
func PruneResponses(dir string, keep func(id string) bool) error {
	responses.Lock()
	defer responses.Unlock()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		id := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".tmp"), ".json"), ".body")
		if !keep(id) {
			errs = append(errs, os.Remove(filepath.Join(dir, e.Name())))
		}
	}
	return errors.Join(errs...)
}
