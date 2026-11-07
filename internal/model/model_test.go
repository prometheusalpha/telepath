package model

import "testing"

func names(items []*Node) []string {
	var s []string
	for _, n := range items {
		s = append(s, n.Name)
	}
	return s
}

func TestTreeEdits(t *testing.T) {
	w := &Workspace{}
	users := NewFolder("Users")
	w.Add(users, "")
	list := NewRequest("list")
	w.Add(list, users.ID)
	w.Add(NewRequest("get"), list.ID) // beside a request, not inside it
	admin := NewFolder("Admin")
	w.Add(admin, users.ID)
	w.Add(NewRequest("ban"), admin.ID)

	if got := names(users.Folder.Children); len(got) != 3 || got[0] != "list" || got[1] != "get" || got[2] != "Admin" {
		t.Fatalf("children of Users %q", got)
	}
	if !users.Folder.Open {
		t.Error("adding into a folder leaves it closed")
	}

	cp := w.Duplicate(admin.ID)
	if cp == nil || cp.ID == admin.ID || cp.Folder.Children[0].ID == admin.Folder.Children[0].ID {
		t.Fatal("duplicate shares IDs with the original")
	}
	cp.Folder.Children[0].Request.URL = "changed"
	if admin.Folder.Children[0].Request.URL != "" {
		t.Error("duplicate shares its requests with the original")
	}

	w.Remove(admin.Folder.Children[0].ID)
	w.Remove(list.ID)
	if got := names(users.Folder.Children); len(got) != 3 || got[0] != "get" || got[2] != "Admin copy" {
		t.Errorf("children of Users after removing %q", got)
	}
	if len(admin.Folder.Children) != 0 {
		t.Errorf("nested remove left %q", names(admin.Folder.Children))
	}
}

func TestMove(t *testing.T) {
	w := &Workspace{}
	a, b := NewFolder("a"), NewFolder("b")
	w.Add(a, "")
	w.Add(b, "")
	x, y := NewRequest("x"), NewRequest("y")
	w.Add(x, a.ID)
	w.Add(y, a.ID)
	sub := NewFolder("sub")
	w.Add(sub, b.ID)

	if !w.Move([]string{y.ID}, x.ID, Before) || names(a.Folder.Children)[0] != "y" {
		t.Fatalf("before: %q", names(a.Folder.Children))
	}
	if !w.Move([]string{x.ID}, sub.ID, Into) || len(sub.Folder.Children) != 1 || len(a.Folder.Children) != 1 {
		t.Fatalf("into: a %q, sub %q", names(a.Folder.Children), names(sub.Folder.Children))
	}
	if !w.Move([]string{a.ID}, b.ID, After) || names(w.Items)[1] != "a" {
		t.Fatalf("after: %q", names(w.Items))
	}
	if !w.Move([]string{y.ID}, "", Before) || names(w.Items)[2] != "y" {
		t.Fatalf("to the top level's end: %q", names(w.Items))
	}

	// Not into itself, nor into what it holds, nor into a request.
	for _, c := range []struct {
		id, target string
		place      Place
	}{{b.ID, b.ID, Into}, {b.ID, sub.ID, Into}, {b.ID, x.ID, After}, {a.ID, y.ID, Into}} {
		if w.Move([]string{c.id}, c.target, c.place) {
			t.Errorf("moved %s to %s", c.id, c.target)
		}
	}
	if names(w.Items)[0] != "b" || len(sub.Folder.Children) != 1 {
		t.Errorf("a refused move changed the tree: %q", names(w.Items))
	}
}
