package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

func TestSettings(t *testing.T) {
	a := emptyApp()
	tt := ui.NewTester(a.view, 1200, 780)
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Dark"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.ws.Theme != model.ThemeDark || !a.dirty {
		t.Errorf("theme %q, dirty %v", a.ws.Theme, a.dirty)
	}

	if err := tt.Click("Code size"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if a.ws.MonoSize != defaultCodeSize+1 || codeSize != defaultCodeSize+1 {
		t.Errorf("size saved %v, used %v", a.ws.MonoSize, codeSize)
	}

	a.ws.MonoFont = "Menlo"
	a.fontChanged()
	if mono != "Menlo, "+builtinMono+", monospace" {
		t.Errorf("family %q", mono)
	}
	// Back to the defaults, for the other tests.
	setMono(&model.Workspace{})
}
