// Command desktop runs Terraformer in a desktop window, the main development loop.
package main

import (
	"flag"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/zachaller/terraformer/game"
)

func main() {
	var opts game.Options
	flag.Uint64Var(&opts.Seed, "seed", 1, "planet seed")
	flag.IntVar(&opts.GridSize, "size", 32, "tiles per cube face edge")
	flag.StringVar(&opts.ScreenshotPath, "screenshot", "", "save a PNG of the first frames to this path and exit")
	flag.Parse()

	g, err := game.New(opts)
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowTitle("Terraformer")
	ebiten.SetWindowSize(1024, 720)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
