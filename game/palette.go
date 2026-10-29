package game

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/zachaller/terraformer/sim"
)

var tileColors = map[sim.TileKind]color.RGBA{
	sim.Basin:     {92, 52, 40, 255},
	sim.Plains:    {160, 98, 62, 255},
	sim.Highlands: {140, 115, 96, 255},
	sim.Peaks:     {200, 190, 180, 255},
	sim.IceCap:    {230, 238, 248, 255},
}

// tileAtlas encodes the planet as an image the shader can read: 6 faces side
// by side, N×N pixels each, one pixel per tile.
func tileAtlas(p *sim.Planet) *image.RGBA {
	n := p.Grid.N
	img := image.NewRGBA(image.Rect(0, 0, sim.Faces*n, n))
	for idx, t := range p.Tiles {
		c := p.Grid.CellAt(idx)
		img.SetRGBA(c.Face*n+c.I, c.J, tileColor(t))
	}
	return img
}

func tileColor(t sim.Tile) color.RGBA {
	c := tileColors[t.Kind]
	if t.Kind == sim.IceCap {
		return c
	}
	// Vary brightness a little with height so flat regions aren't uniform.
	shade := 0.85 + 0.3*(t.Height+1)/2
	scale := func(v uint8) uint8 { return uint8(min(float64(v)*shade, 255)) }
	return color.RGBA{scale(c.R), scale(c.G), scale(c.B), 255}
}

func newAtlasImage(p *sim.Planet) *ebiten.Image {
	return ebiten.NewImageFromImage(tileAtlas(p))
}
