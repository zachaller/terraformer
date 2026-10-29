package game

import (
	"image/color"
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
	viewIndustry
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
	case viewIndustry:
		return "Industry"
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
	iceColor    = rgb{230, 238, 248}
	dryIceColor = rgb{246, 249, 255}
	waterColor  = rgb{30, 80, 160}
	lifeColor   = rgb{60, 140, 60}
	smogColor   = rgb{75, 55, 70}
	greyColor   = rgb{70, 70, 75}

	// speciesColors is how each species tints the surface, from the UX
	// mocks' life sheet: land shifts olive, then green, then deep green.
	speciesColors = [sim.NumSpecies]rgb{
		sim.Moss:     {143, 160, 90},
		sim.Grass:    {110, 154, 58},
		sim.Shrubs:   {62, 107, 44},
		sim.Forest:   {34, 64, 30},
		sim.Plankton: {42, 122, 128},
		sim.Fish:     {60, 140, 175},
		sim.Grazers:  {150, 120, 70},
	}
	// lifeViewColors are brighter versions for the Life view.
	lifeViewColors = [sim.NumSpecies]rgb{
		sim.Moss:     {175, 195, 95},
		sim.Grass:    {125, 205, 60},
		sim.Shrubs:   {70, 160, 55},
		sim.Forest:   {30, 110, 35},
		sim.Plankton: {60, 175, 175},
		sim.Fish:     {100, 205, 240},
		sim.Grazers:  {230, 180, 90},
	}
)

// The tile atlas the shader reads is (10·P)×(3·P) pixels, where P = N+1:
// one P×P block per diamond of the hex grid (see sim/grid.go), side by side,
// with a pixel for every lattice point (i, j) in [0, N]², including those on
// the diamond's edges, which belong to neighbouring diamonds. That way every
// lattice triangle the shader blends across has all three corners in one
// block. Three bands of rows hold, for the tile at each lattice point: its
// colour; building data (red: building type + 1, or 0 for none; green:
// activity; blue: sim.StatusKind); and its tile index (red + 256·green), so
// the shader can match tiles to the selection and placement preview.

// atlasSize returns the atlas's width and height for grid g.
func atlasSize(g sim.Grid) (int, int) { return sim.Diamonds * (g.N + 1), 3 * (g.N + 1) }

// atlasSources maps each pixel in a band of the atlas to its tile's index.
func atlasSources(g sim.Grid) []int32 {
	p := g.N + 1
	w, _ := atlasSize(g)
	src := make([]int32, w*p)
	for d := range sim.Diamonds {
		for j := range p {
			for i := range p {
				src[j*w+d*p+i] = int32(g.Index(g.LatticeCell(d, i, j)))
			}
		}
	}
	return src
}

// fillAtlas writes the planet into pix, the atlas's RGBA pixels, using src
// from atlasSources.
func fillAtlas(pix []byte, src []int32, p *sim.Planet, v viewMode) {
	colors := make([]rgb, len(p.Tiles))
	for idx, t := range p.Tiles {
		colors[idx] = tileColor(t, v)
	}
	type bdata struct{ typ, act, status uint8 }
	buildings := make([]bdata, len(p.Tiles))
	for _, b := range p.Buildings() {
		buildings[b.Tile] = bdata{uint8(b.Type) + 1, uint8(b.Activity * 255), uint8(p.Diagnose(b).Kind)}
	}
	band := len(src) * 4
	for k, idx := range src {
		col, b := colors[idx], buildings[idx]
		copy(pix[k*4:], []byte{uint8(col[0]), uint8(col[1]), uint8(col[2]), 255})
		copy(pix[band+k*4:], []byte{b.typ, b.act, b.status, 255})
		copy(pix[2*band+k*4:], []byte{uint8(idx), uint8(idx >> 8), 0, 255})
	}
}

func tileColor(t sim.Tile, v viewMode) rgb {
	switch v {
	case viewTemp:
		return tempColor(t.Temp)
	case viewWater:
		c := mix(greyColor, iceColor, t.Ice*1.5)
		c = mix(c, rgb{200, 210, 235}, t.DryIce)
		return mix(c, rgb{40, 120, 255}, t.Water*8)
	case viewLife:
		c := mix(greyColor, rgb{110, 85, 60}, t.Soil*0.8)
		if sim.Habitable(t) {
			c = rgb{110, 100, 50} // could support life
		}
		if t.Life > 0 {
			c = mix(c, rgb{80, 220, 80}, 0.3+0.7*t.Life)
		}
		for s := range sim.NumSpecies {
			if t.Bio[s] > 0 {
				c = mix(c, lifeViewColors[s], 0.4+0.6*t.Bio[s])
			}
		}
		return c
	case viewIndustry:
		c := mix(greyColor, rgb{120, 120, 125}, (t.Height+1)/2)
		if t.Volcanic {
			c = mix(c, rgb{200, 70, 50}, 0.6)
		}
		if t.Ore > 0 {
			m := mineralColor(t.Mineral)
			c = mix(c, rgb{float64(m.R), float64(m.G), float64(m.B)}, 0.5+t.Ore/600)
		}
		return mix(c, smogColor, t.Pollution*1.5)
	}
	return surfaceColor(t)
}

func surfaceColor(t sim.Tile) rgb {
	base := terrainColors[t.Kind()]
	// Vary brightness a little with height so flat regions aren't uniform.
	shade := 0.85 + 0.3*(t.Height+1)/2
	c := rgb{base[0] * shade, base[1] * shade, base[2] * shade}
	if t.Ore > 0 {
		c = mix(c, rgb{80, 105, 140}, 0.4) // ore deposit
	}
	c = mix(c, smogColor, t.Pollution)
	c = mix(c, rgb{c[0] * 0.75, c[1] * 0.75, c[2] * 0.75}, t.Water/0.05) // damp soil
	c = mix(c, lifeColor, t.Life*0.75)
	for _, s := range []sim.Species{sim.Moss, sim.Grass, sim.Shrubs, sim.Forest} {
		c = mix(c, speciesColors[s], t.Bio[s]*0.85)
	}
	c = mix(c, waterColor, (t.Water-0.05)*4*0.85) // standing water
	c = mix(c, speciesColors[sim.Plankton], t.Bio[sim.Plankton]*0.45)
	c = mix(c, speciesColors[sim.Fish], t.Bio[sim.Fish]*0.25)
	c = mix(c, speciesColors[sim.Grazers], t.Bio[sim.Grazers]*0.2)
	ice := mix(iceColor, rgb{150, 112, 88}, t.Dust*0.55)
	c = mix(c, ice, t.Ice*2)
	return mix(c, mix(dryIceColor, rgb{150, 112, 88}, t.Dust*0.55), t.DryIce*1.5)
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

// mineralColor is how deposits of m show in the Industry view.
func mineralColor(m sim.Mineral) color.RGBA {
	switch m {
	case sim.Fluorite:
		return color.RGBA{0xB0, 0x7C, 0xE8, 0xFF}
	case sim.Nitrate:
		return color.RGBA{0xB8, 0xE0, 0xA0, 0xFF}
	}
	return color.RGBA{0xF2, 0xC0, 0x4A, 0xFF}
}
