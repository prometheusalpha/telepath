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
