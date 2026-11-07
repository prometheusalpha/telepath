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
}

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
	URL    string `json:"url"`
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
		r := *n.Request
		cp.Request = &r
	}
	if n.Folder != nil {
		cp.Folder = &Folder{Children: make([]*Node, 0, len(n.Folder.Children)), Open: n.Folder.Open}
		for _, k := range n.Folder.Children {
			cp.Folder.Children = append(cp.Folder.Children, clone(k))
		}
	}
	return cp
}
