// Package mobile is the entry point ebitenmobile binds into an Android library.
package mobile

import (
	"github.com/hajimehoshi/ebiten/v2/mobile"
	"github.com/zachaller/terraformer/game"
)

func init() {
	g, err := game.New(game.Options{Seed: 1, GridSize: 32})
	if err != nil {
		panic(err)
	}
	mobile.SetGame(g)
}

// Dummy is exported because gomobile won't bind a package without exported functions.
func Dummy() {}
