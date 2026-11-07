package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

// drag presses the row showing from and lets go over the row showing to,
// frac of the way down it.
func drag(t *testing.T, tt *ui.Tester, from, to string, frac float32) {
	t.Helper()
	src, ok1 := tt.Find(from)
	dst, ok2 := tt.Find(to)
	if !ok1 || !ok2 {
		t.Fatalf("no row %q or %q in %q", from, to, tt.Texts())
	}
	// The label's box is shorter than its row: aim from the row's middle,
	// which the label's middle is.
	x, y := src.X+src.W/2, src.Y+src.H/2
	tt.Press(x, y)
	tt.Move(x, y+8)
	tt.Frame()
	ty := dst.Y + dst.H/2 + (frac-0.5)*28 // rows are about 28 high
	tt.Move(dst.X+dst.W/2, ty)
	tt.Frame()
	tt.Release(dst.X+dst.W/2, ty)
	tt.Frame()
	tt.Frame()
}

func dragTree() (*model.Workspace, *model.Node, *model.Node) {
	ws := &model.Workspace{}
	a, b := model.NewFolder("alpha"), model.NewFolder("beta")
	ws.Add(a, "")
	ws.Add(b, "")
	ws.Add(model.NewRequest("one"), a.ID)
	ws.Add(model.NewRequest("two"), a.ID)
	b.Folder.Open = false
	return ws, a, b
}

func TestDragIntoAFolder(t *testing.T) {
	ws, a, b := dragTree()
	app := newApp(ws, "")
	tt := ui.NewTester(app.view, 1200, 780)
	drag(t, tt, "two", "beta", 0.5)
	if len(a.Folder.Children) != 1 || len(b.Folder.Children) != 1 || b.Folder.Children[0].Name != "two" {
		t.Fatalf("alpha %q, beta %q", names(a.Folder.Children), names(b.Folder.Children))
	}
	if !app.files.Open.Has(b.ID) || !app.dirty {
		t.Error("dropping into beta leaves it closed or unsaved")
	}
}

func TestDragBeforeARow(t *testing.T) {
	ws, a, _ := dragTree()
	app := newApp(ws, "")
	tt := ui.NewTester(app.view, 1200, 780)
	drag(t, tt, "two", "one", 0.1)
	if got := names(a.Folder.Children); len(got) != 2 || got[0] != "two" {
		t.Errorf("alpha %q", got)
	}
}

func TestDraggingAFolderDoesNotToggleIt(t *testing.T) {
	ws, a, b := dragTree()
	app := newApp(ws, "")
	tt := ui.NewTester(app.view, 1200, 780)
	drag(t, tt, "alpha", "beta", 0.95) // after beta, which is closed
	if !app.files.Open.Has(a.ID) {
		t.Error("dragging alpha closed it")
	}
	if got := names(ws.Items); len(got) != 2 || got[0] != "beta" {
		t.Errorf("top level %q", got)
	}
	// Into itself does nothing.
	drag(t, tt, "beta", "beta", 0.5)
	if len(b.Folder.Children) != 0 || len(ws.Items) != 2 {
		t.Errorf("beta dropped into itself: %q", names(ws.Items))
	}
}
