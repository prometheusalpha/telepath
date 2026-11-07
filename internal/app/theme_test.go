package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"telepath/internal/model"
)

func TestAppearanceButtonCycles(t *testing.T) {
	a := emptyApp()
	var applied []string
	a.setAppearance = func(theme string) { applied = append(applied, theme) }
	tt := ui.NewTester(a.view, 1200, 780)
	for _, want := range []string{model.ThemeLight, model.ThemeDark, model.ThemeSystem} {
		if err := tt.Click("Appearance: " + themeOf(a.ws.Theme).label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.ws.Theme != want {
			t.Fatalf("theme %q, want %q", a.ws.Theme, want)
		}
	}
	if len(applied) != 3 || applied[0] != model.ThemeLight || !a.dirty {
		t.Errorf("applied %q, dirty %v", applied, a.dirty)
	}
}
