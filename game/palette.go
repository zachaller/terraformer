package game

import (
	"math"

	"github.com/zachaller/terraformer/sim"
)

// viewMode selects what the globe shows.
type viewMode int

const (
	viewSurface viewMode = iota
	viewTemp
	viewWater
	viewLife
	numViews
)

func (v viewMode) String() string {
	switch v {
	case viewSurface:
		return "Surface"
	case viewTemp:
		return "Temperature"
	case viewWater:
		return "Water"
	case viewLife:
		return "Life"
	}
	return "?"
}

type rgb [3]float64

func mix(a, b rgb, t float64) rgb {
	t = clamp01(t)
	return rgb{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

func clamp01(t float64) float64 { return math.Min(math.Max(t, 0), 1) }

var (
	terrainColors = map[sim.TileKind]rgb{
		sim.Basin:     {92, 52, 40},
		sim.Plains:    {160, 98, 62},
		sim.Highlands: {140, 115, 96},
		sim.Peaks:     {200, 190, 180},
	}
	iceColor   = rgb{230, 238, 248}
	waterColor = rgb{30, 80, 160}
	lifeColor  = rgb{60, 140, 60}
	heatColor  = rgb{255, 90, 40}
	greyColor  = rgb{70, 70, 75}
)

// fillAtlas writes the planet into pix, the RGBA pixels of the tile atlas the
// shader reads: 6 faces side by side, N×N pixels each, one pixel per tile.
func fillAtlas(pix []byte, p *sim.Planet, v viewMode) {
	n := p.Grid.N
	stride := sim.Faces * n
	for idx, t := range p.Tiles {
		c := p.Grid.CellAt(idx)
		off := (c.J*stride + c.Face*n + c.I) * 4
		col := tileColor(t, v)
		pix[off] = uint8(col[0])
		pix[off+1] = uint8(col[1])
		pix[off+2] = uint8(col[2])
		pix[off+3] = 255
	}
}

func tileColor(t sim.Tile, v viewMode) rgb {
	switch v {
	case viewTemp:
		return tempColor(t.Temp)
	case viewWater:
		c := mix(greyColor, iceColor, t.Ice*1.5)
		return mix(c, rgb{40, 120, 255}, t.Water*8)
	case viewLife:
		c := greyColor
		if sim.Habitable(t) {
			c = rgb{110, 100, 50} // could support life
		}
		if t.Life > 0 {
			c = mix(c, rgb{80, 220, 80}, 0.3+0.7*t.Life)
		}
		return c
	}
	return surfaceColor(t)
}

func surfaceColor(t sim.Tile) rgb {
	base := terrainColors[t.Kind()]
	// Vary brightness a little with height so flat regions aren't uniform.
	shade := 0.85 + 0.3*(t.Height+1)/2
	c := rgb{base[0] * shade, base[1] * shade, base[2] * shade}
	c = mix(c, heatColor, t.Heat/100*0.35)
	c = mix(c, rgb{c[0] * 0.75, c[1] * 0.75, c[2] * 0.75}, t.Water/0.05) // damp soil
	c = mix(c, lifeColor, t.Life*0.75)
	c = mix(c, waterColor, (t.Water-0.05)*4*0.85) // standing water
	return mix(c, iceColor, t.Ice*2)
}

// tempColor maps °C onto a ramp: purple (-80) → blue → white (0) → orange → red (50).
func tempColor(c float64) rgb {
	stops := []struct {
		t   float64
		col rgb
	}{
		{-80, rgb{60, 20, 110}},
		{-40, rgb{40, 90, 220}},
		{0, rgb{235, 240, 245}},
		{25, rgb{250, 160, 40}},
		{50, rgb{200, 30, 30}},
	}
	if c <= stops[0].t {
		return stops[0].col
	}
	for i := 1; i < len(stops); i++ {
		if c <= stops[i].t {
			a, b := stops[i-1], stops[i]
			return mix(a.col, b.col, (c-a.t)/(b.t-a.t))
		}
	}
	return stops[len(stops)-1].col
}
