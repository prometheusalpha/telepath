// Package importx reads collections from files, Telepath's own and
// Postman's, and from cURL commands, and writes them back out.
package importx

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"telepath/internal/model"
)

// Result is what a file brings: requests to put under a folder of Name,
// variables to add, and what was left out, for telling the user.
type Result struct {
	Name    string
	Items   []*model.Node
	Env     []model.KV
	Skipped []string
}

const nativeVersion = 1

// native is Telepath's export: a workspace's items, or one folder's, and
// the variables they use.
type native struct {
	Telepath int           `json:"telepath"`
	Name     string        `json:"name"`
	Items    []*model.Node `json:"items"`
	Env      []model.KV    `json:"env,omitempty"`
}

func Export(name string, items []*model.Node, env []model.KV) ([]byte, error) {
	return json.MarshalIndent(native{Telepath: nativeVersion, Name: name, Items: items, Env: env}, "", "  ")
}

// Parse reads a Telepath export or a Postman collection, by what it holds
// rather than its name; filename names the result when the file doesn't.
func Parse(data []byte, filename string) (*Result, error) {
	var probe struct {
		Telepath int `json:"telepath"`
		Info     *struct {
			Schema string `json:"schema"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("not a JSON file: %w", err)
	}
	var r *Result
	var err error
	switch {
	case probe.Telepath > 0:
		r, err = parseNative(data)
	case probe.Info != nil && strings.Contains(probe.Info.Schema, "collection"):
		r, err = parsePostman(data)
	default:
		return nil, errors.New("neither a Telepath export nor a Postman collection")
	}
	if err != nil {
		return nil, err
	}
	if r.Name == "" {
		r.Name = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	return r, nil
}

func parseNative(data []byte) (*Result, error) {
	var f native
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Telepath > nativeVersion {
		return nil, fmt.Errorf("made by a newer Telepath (format %d)", f.Telepath)
	}
	r := &Result{Name: f.Name, Env: f.Env}
	for _, n := range f.Items {
		if n = fresh(n); n != nil {
			r.Items = append(r.Items, n)
		}
	}
	return r, nil
}

// fresh gives a node from a file new IDs, so importing the same file
// twice, or into the workspace it came from, makes no duplicates of IDs;
// nodes neither folder nor request are dropped.
func fresh(n *model.Node) *model.Node {
	if n == nil || (n.Folder == nil) == (n.Request == nil) {
		return nil
	}
	n.ID = model.NewID()
	if n.Folder != nil {
		kids := n.Folder.Children[:0]
		for _, k := range n.Folder.Children {
			if k = fresh(k); k != nil {
				kids = append(kids, k)
			}
		}
		n.Folder.Children = kids
		if n.Folder.Children == nil {
			n.Folder.Children = []*model.Node{}
		}
	}
	return n
}
