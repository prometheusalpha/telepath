// Package model is the workspace the app edits and saves: a tree of
// folders and requests, and the one set of variables.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
)

const Version = 1

type Workspace struct {
	Version int     `json:"version"`
	Items   []*Node `json:"items"`
	Env     []KV    `json:"env,omitempty"`   // the one set of variables
	Theme   string  `json:"theme,omitempty"` // ThemeSystem, ThemeLight or ThemeDark
	// MonoFont is the family of code, URLs and key/value text, "" the one
	// the app brings; MonoSize the size of the request and response
	// bodies, 0 the default.
	MonoFont string  `json:"monoFont,omitempty"`
	MonoSize float32 `json:"monoSize,omitempty"`
	// Shortcuts are the keys set in Settings by command, as "Shift+Cmd+N",
	// "" for none; a command not in it has its default.
	Shortcuts map[string]string `json:"shortcuts,omitempty"`
}

const (
	ThemeSystem = ""
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// Node is a folder or a request: exactly one of Folder and Request is set.
type Node struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Folder  *Folder  `json:"folder,omitempty"`
	Request *Request `json:"request,omitempty"`
}

type Folder struct {
	Children []*Node `json:"children"`
	Open     bool    `json:"open,omitempty"`
}

type Request struct {
	Method string `json:"method"`
	// URL is as typed, with the enabled query parameters in it; Query also
	// keeps those disabled, which the URL leaves out.
	URL     string `json:"url"`
	Query   []KV   `json:"query,omitempty"`
	Path    []KV   `json:"path,omitempty"` // the values of the URL's :name segments
	Headers []KV   `json:"headers,omitempty"`
	Body    Body   `json:"body"`
	Auth    Auth   `json:"auth"`
}

type KV struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
	// File makes Value a path whose file a multipart body sends.
	File bool `json:"file,omitempty"`
	// Secret hides Value in the variables' table.
	Secret bool `json:"secret,omitempty"`
}

const (
	BodyNone      = ""
	BodyJSON      = "json"
	BodyText      = "text"
	BodyForm      = "form"
	BodyMultipart = "multipart"
)

type Body struct {
	Kind string `json:"kind,omitempty"`
	Text string `json:"text,omitempty"` // JSON and text bodies
	Form []KV   `json:"form,omitempty"` // form and multipart bodies
}

const (
	AuthNone   = ""
	AuthBasic  = "basic"
	AuthBearer = "bearer"
	AuthAPIKey = "apikey"
)

// Auth fields are kept for every kind, so switching kinds and back loses
// nothing typed.
type Auth struct {
	Kind     string `json:"kind,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	Key      string `json:"key,omitempty"`   // API key's header or query name
	Value    string `json:"value,omitempty"` // and its value
	InQuery  bool   `json:"inQuery,omitempty"`
}

func (r *Request) Clone() *Request {
	cp := *r
	cp.Query = slices.Clone(r.Query)
	cp.Path = slices.Clone(r.Path)
	cp.Headers = slices.Clone(r.Headers)
	cp.Body.Form = slices.Clone(r.Body.Form)
	return &cp
}

func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func NewFolder(name string) *Node {
	return &Node{ID: NewID(), Name: name, Folder: &Folder{Children: []*Node{}}}
}

func NewRequest(name string) *Node {
	return &Node{ID: NewID(), Name: name, Request: &Request{Method: "GET"}}
}

func (n *Node) IsFolder() bool { return n.Folder != nil }

// Walk visits every node depth-first, with the folder holding it (nil at
// the top); fn returning false skips a folder's children.
func (w *Workspace) Walk(fn func(n, parent *Node) bool) {
	var walk func(items []*Node, parent *Node)
	walk = func(items []*Node, parent *Node) {
		for _, n := range items {
			if fn(n, parent) && n.Folder != nil {
				walk(n.Folder.Children, n)
			}
		}
	}
	walk(w.Items, nil)
}

// siblings returns the slice holding the node with id, and where.
func (w *Workspace) siblings(id string) (*[]*Node, int) {
	if i := slices.IndexFunc(w.Items, func(n *Node) bool { return n.ID == id }); i >= 0 {
		return &w.Items, i
	}
	var list *[]*Node
	idx := -1
	w.Walk(func(n, _ *Node) bool {
		if list != nil || n.Folder == nil {
			return list == nil
		}
		if i := slices.IndexFunc(n.Folder.Children, func(k *Node) bool { return k.ID == id }); i >= 0 {
			list, idx = &n.Folder.Children, i
			return false
		}
		return true
	})
	return list, idx
}

// Add puts n inside the folder at, or after the request at; an empty or
// unknown at adds it at the end of the top level.
func (w *Workspace) Add(n *Node, at string) {
	list, i := w.siblings(at)
	switch {
	case list == nil:
		w.Items = append(w.Items, n)
	case (*list)[i].Folder != nil:
		f := (*list)[i].Folder
		f.Children = append(f.Children, n)
		f.Open = true
	default:
		*list = slices.Insert(*list, i+1, n)
	}
}

func (w *Workspace) Remove(id string) {
	if list, i := w.siblings(id); list != nil {
		*list = slices.Delete(*list, i, i+1)
	}
}

// Duplicate copies the node with id, its children too, right after it.
func (w *Workspace) Duplicate(id string) *Node {
	list, i := w.siblings(id)
	if list == nil {
		return nil
	}
	cp := clone((*list)[i])
	cp.Name += " copy"
	*list = slices.Insert(*list, i+1, cp)
	return cp
}

func clone(n *Node) *Node {
	cp := &Node{ID: NewID(), Name: n.Name}
	if n.Request != nil {
		cp.Request = n.Request.Clone()
	}
	if n.Folder != nil {
		cp.Folder = &Folder{Children: make([]*Node, 0, len(n.Folder.Children)), Open: n.Folder.Open}
		for _, k := range n.Folder.Children {
			cp.Folder.Children = append(cp.Folder.Children, clone(k))
		}
	}
	return cp
}

// Place says where Move puts nodes: beside the target, or inside it.
type Place int

const (
	Before Place = iota
	After
	Into // the end of a folder's children
)

// Move puts the nodes with ids by target, keeping their order in the tree;
// an empty target is the end of the top level. It moves nothing and
// returns false when the target is one of the nodes or inside one, as a
// folder dropped into itself would be.
func (w *Workspace) Move(ids []string, target string, place Place) bool {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var nodes []*Node
	inMoved := false // the walk is inside a node that moves
	var walk func(items []*Node) bool
	walk = func(items []*Node) bool {
		for _, n := range items {
			if n.ID == target && (inMoved || want[n.ID]) {
				return false
			}
			top := want[n.ID] && !inMoved
			if top {
				nodes = append(nodes, n)
				inMoved = true
			}
			if n.Folder != nil && !walk(n.Folder.Children) {
				return false
			}
			if top {
				inMoved = false
			}
		}
		return true
	}
	if !walk(w.Items) || len(nodes) == 0 {
		return false
	}
	if target != "" {
		if list, i := w.siblings(target); list == nil || place == Into && (*list)[i].Folder == nil {
			return false
		}
	}
	for _, n := range nodes {
		w.Remove(n.ID)
	}
	if target == "" {
		w.Items = append(w.Items, nodes...)
		return true
	}
	list, i := w.siblings(target)
	switch place {
	case Into:
		f := (*list)[i].Folder
		f.Children = append(f.Children, nodes...)
	case After:
		*list = slices.Insert(*list, i+1, nodes...)
	default:
		*list = slices.Insert(*list, i, nodes...)
	}
	return true
}
