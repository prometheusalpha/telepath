package main

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

var gearIcon = icon(`<circle cx="8" cy="8" r="2"/><path d="M8 1.5v1.7M8 12.8v1.7M1.5 8h1.7M12.8 8h1.7M3.4 3.4l1.2 1.2M11.4 11.4l1.2 1.2M3.4 12.6l1.2-1.2M11.4 4.6l1.2-1.2"/><circle cx="8" cy="8" r="4.3"/>`)

const builtinLabel = "JetBrains Mono (built in)"

func (a *app) openSettings() {
	if a.monoFamilies == nil {
		// The system's list takes a while with many fonts installed, so it
		// is asked for once, when settings first open.
		a.monoFamilies = []string{builtinLabel}
		for _, f := range ui.FontFamilies() {
			if f.Monospaced && !f.Registered {
				a.monoFamilies = append(a.monoFamilies, f.Name)
			}
		}
	}
	// A family chosen before and since uninstalled still shows as chosen;
	// the text falls back to the built-in one meanwhile.
	if f := a.ws.MonoFont; f != "" && !slices.Contains(a.monoFamilies, f) {
		a.monoFamilies = append(a.monoFamilies, f)
	}
	a.settingsOpen = true
}

func (a *app) settingsModal(c *ui.Context) {
	ui.Modal(c, &a.settingsOpen, func() {
		ui.Column(c).Width(460).Gap(4).Children(func() {
			ui.Text(c, "Settings").FontSize(16).FontWeight(600).Padding(0, 0, 8)
			ui.Form(c, func() {
				ui.Field(c, "Appearance", func() {
					ui.Row(c).Children(func() {
						i := slices.IndexFunc(themes, func(th themeChoice) bool { return th.theme == a.ws.Theme })
						labels := make([]string, len(themes))
						for j, th := range themes {
							labels[j] = th.label
						}
						if ui.Segmented(c, &i, labels...).Changed() {
							a.setTheme(themes[i].theme)
						}
					})
				})
				font := a.ws.MonoFont
				if font == "" {
					font = builtinLabel
				}
				ui.Field(c, "Code font", func() {
					if ui.Select(c, &font, a.monoFamilies).Changed() {
						if font == builtinLabel {
							font = ""
						}
						a.ws.MonoFont = font
						a.fontChanged()
					}
				}).Description("URLs, key/value tables, and the request and response bodies.")
				size := float64(codeSize)
				ui.Field(c, "Code size", func() {
					ui.Row(c).Children(func() {
						if ui.NumberInput(c, &size, 9, 24, 1).Width(110).Changed() {
							a.ws.MonoSize = float32(size)
							a.fontChanged()
						}
					})
				}).Description("The request and response bodies.")
			})
			ui.Row(c).Justify(ui.End).Padding(12, 0, 0).Children(func() {
				if ui.PrimaryButton(c, "Done").Clicked() {
					a.settingsOpen = false
				}
			})
		})
	})
}

func (a *app) fontChanged() {
	setMono(a.ws)
	a.changed()
}
