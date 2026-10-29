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

// SetSafeArea tells the game how many pixels along each screen edge are
// covered by system bars or display cutouts. Called from MainActivity.
func SetSafeArea(left, top, right, bottom int) {
	game.SetSafeArea(float64(left), float64(top), float64(right), float64(bottom))
}

// Dummy is exported because gomobile won't bind a package without exported functions.
func Dummy() {}
