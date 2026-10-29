// Command desktop runs Terraformer in a desktop window, the main development loop.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/zachaller/terraformer/game"
)

func main() {
	var opts game.Options
	flag.Uint64Var(&opts.Seed, "seed", 1, "planet seed")
	flag.IntVar(&opts.GridSize, "size", 32, "tiles per cube face edge")
	flag.BoolVar(&opts.DemoHeaters, "demo-heaters", false, "put fixed warmth on a ring of tiles around the equator")
	flag.BoolVar(&opts.DemoColony, "demo-colony", false, "land and build a small working colony")
	flag.IntVar(&opts.DemoSteps, "demo-steps", 0, "fast-forward this many simulation steps (10 = one day)")
	flag.Float64Var(&opts.Zoom, "zoom", 1, "starting zoom")
	flag.StringVar(&opts.View, "view", "surface", "starting view: surface, temp, water, life or industry")
	flag.StringVar(&opts.ScreenshotPath, "screenshot", "", "save a PNG of the first frames to this path and exit")
	var winW, winH int
	var safe string
	flag.IntVar(&winW, "width", 1024, "window width")
	flag.IntVar(&winH, "height", 720, "window height")
	flag.StringVar(&safe, "safe-area", "", "simulate phone system bars: insets in pixels as left,top,right,bottom")
	flag.Parse()

	if safe != "" {
		var l, t, r, b float64
		if _, err := fmt.Sscanf(safe, "%g,%g,%g,%g", &l, &t, &r, &b); err != nil {
			log.Fatalf("-safe-area: %v", err)
		}
		game.SetSafeArea(l, t, r, b)
	}

	g, err := game.New(opts)
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowTitle("Terraformer")
	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
