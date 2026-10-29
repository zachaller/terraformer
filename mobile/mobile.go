// Package mobile is the entry point ebitenmobile binds into an Android library.
package mobile

import (
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2/mobile"
	"github.com/zachaller/terraformer/game"
)

var g *game.Game

func init() {
	var err error
	g, err = game.New(game.Options{Seed: uint64(time.Now().UnixNano()), GridSize: 25})
	if err != nil {
		panic(err)
	}
	mobile.SetGame(g)
}

// SetDataDir tells the game where to keep its save file: the app's private
// files directory. Called from MainActivity before the game starts.
func SetDataDir(dir string) { game.SetSaveDir(dir) }

// Save saves the game now. Called from MainActivity when the app goes into
// the background.
func Save() {
	if err := g.SaveNow(); err != nil {
		log.Printf("saving: %v", err)
	}
}

// SetSafeArea tells the game how many pixels along each screen edge are
// covered by system bars or display cutouts. Called from MainActivity.
func SetSafeArea(left, top, right, bottom int) {
	game.SetSafeArea(float64(left), float64(top), float64(right), float64(bottom))
}

// Dummy is exported because gomobile won't bind a package without exported functions.
func Dummy() {}
