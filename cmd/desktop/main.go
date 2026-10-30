// Command desktop runs Terraformer in a desktop window, the main development loop.
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/zachaller/terraformer/game"
)

// icon is the window icon (drawn by cmd/icon). macOS uses the app bundle's
// icon instead.
//
//go:embed icon.png
var icon []byte

func main() {
	var opts game.Options
	flag.Uint64Var(&opts.Seed, "seed", 1, "planet seed for a new game")
	flag.IntVar(&opts.GridSize, "size", 25, "hex grid size: steps along each icosahedron edge (tiles = 10·size² + 2)")
	flag.BoolVar(&opts.DemoHeaters, "demo-heaters", false, "put fixed warmth on a ring of tiles around the equator")
	flag.BoolVar(&opts.DemoColony, "demo-colony", false, "land and build a small working colony")
	flag.BoolVar(&opts.DemoShowcase, "demo-showcase", false, "with -demo-colony, also build one of every building")
	flag.IntVar(&opts.DemoSteps, "demo-steps", 0, "fast-forward this many simulation steps (10 = one day)")
	flag.Float64Var(&opts.Zoom, "zoom", 1, "starting zoom")
	flag.StringVar(&opts.View, "view", "surface", "starting view: surface, temp, water, life or industry")
	flag.StringVar(&opts.ScreenshotPath, "screenshot", "", "save a PNG of the first frames to this path and exit")
	var winW, winH int
	var safe string
	flag.IntVar(&winW, "width", 1024, "window width")
	flag.IntVar(&winH, "height", 720, "window height")
	flag.StringVar(&safe, "safe-area", "", "simulate phone system bars: insets in pixels as left,top,right,bottom")
	flag.BoolVar(&opts.NoSave, "no-save", false, "don't load or save the game")
	flag.BoolVar(&opts.NoAudio, "no-audio", false, "turn sound off entirely")
	flag.Parse()
	// Starting audio with no sound device is fatal; Linux machines without
	// one (servers, containers) run silently.
	if _, err := os.Stat("/dev/snd"); runtime.GOOS == "linux" && err != nil {
		opts.NoAudio = true
	}

	if dir, err := os.UserConfigDir(); err == nil {
		game.SetSaveDir(filepath.Join(dir, "terraformer"))
	}

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
	if img, err := png.Decode(bytes.NewReader(icon)); err == nil {
		ebiten.SetWindowIcon([]image.Image{img})
	}
	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
	if err := g.SaveNow(); err != nil {
		log.Printf("saving: %v", err)
	}
}
