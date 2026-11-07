package main

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

// The Mono variant keeps the Nerd Font icons one cell wide, and has the
// ligatures (the NL variants are the ones without).
var (
	//go:embed fonts/JetBrainsMonoNerdFontMono-Regular.ttf
	monoRegular []byte
	//go:embed fonts/JetBrainsMonoNerdFontMono-Bold.ttf
	monoBold []byte
)

// builtinMono is the family the app brings, under a name of its own so a
// copy installed on the system doesn't mix its weights in.
const builtinMono = "Telepath Mono"

func init() {
	for _, f := range [][]byte{monoRegular, monoBold} {
		if err := ui.RegisterFont(f, builtinMono); err != nil {
			panic(err)
		}
	}
}
