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
	iceColor   = rgb{230, 238, 248}
	waterColor = rgb{30, 80, 160}
	lifeColor  = rgb{60, 140, 60}
	oreColor   = rgb{230, 180, 60}
	smogColor  = rgb{75, 55, 70}
	greyColor  = rgb{70, 70, 75}

	// speciesColors is how each species tints the surface, from the UX
	// mocks' life sheet: land shifts olive, then green, then deep green.
	speciesColors = [sim.NumSpecies]rgb{
		sim.Moss:     {143, 160, 90},
		sim.Grass:    {110, 154, 58},
		sim.Shrubs:   {62, 107, 44},
		sim.Forest:   {34, 64, 30},
		sim.Plankton: {42, 122, 128},
		sim.Fish:     {60, 140, 175},
	}
	// lifeViewColors are brighter versions for the Life view.
	lifeViewColors = [sim.NumSpecies]rgb{
		sim.Moss:     {175, 195, 95},
		sim.Grass:    {125, 205, 60},
		sim.Shrubs:   {70, 160, 55},
		sim.Forest:   {30, 110, 35},
		sim.Plankton: {60, 175, 175},
		sim.Fish:     {100, 205, 240},
	}
)

// The tile atlas the shader reads is (6·P)×(2·P) pixels, where P = N+2: one
// P×P block per cube face, side by side. Each block holds the face's N×N
// tiles plus a one-tile border copied from the neighbouring faces, so the
// shader can blend colours smoothly across face seams. The top half holds
// tile colours; the bottom half holds building data (red: building type + 1,
// or 0 for none; green: activity; blue: sim.StatusKind).

// atlasSources maps each pixel in the top half of the atlas to the tile whose
// colour it shows.
func atlasSources(g sim.Grid) []int32 {
	n := g.N
	p := n + 2
	src := make([]int32, sim.Faces*p*p)
	for f := range sim.Faces {
		for j := -1; j <= n; j++ {
			for i := -1; i <= n; i++ {
				src[(j+1)*sim.Faces*p+f*p+i+1] = int32(g.Index(g.Wrap(f, i, j)))
			}
		}
	}
	return src
}

// fillAtlas writes the planet into pix, the atlas's RGBA pixels, using src
// from atlasSources.
func fillAtlas(pix []byte, src []int32, p *sim.Planet, v viewMode) {
	half := len(pix) / 2
	colors := make([]rgb, len(p.Tiles))
	for idx, t := range p.Tiles {
		colors[idx] = tileColor(t, v)
	}
	for k, idx := range src {
		col := colors[idx]
		pix[k*4] = uint8(col[0])
		pix[k*4+1] = uint8(col[1])
		pix[k*4+2] = uint8(col[2])
		pix[k*4+3] = 255
	}
	clear(pix[half:])
	for k := 3; k < half; k += 4 {
		pix[half+k] = 255
	}
	n := p.Grid.N
	stride := sim.Faces * (n + 2)
	for _, b := range p.Buildings() {
		c := p.Grid.CellAt(b.Tile)
		off := half + ((c.J+1)*stride+c.Face*(n+2)+c.I+1)*4
		pix[off] = uint8(b.Type) + 1
		pix[off+1] = uint8(b.Activity * 255)
		pix[off+2] = uint8(p.Diagnose(b).Kind)
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
		for s := range sim.NumSpecies {
			if t.Bio[s] > 0 {
				c = mix(c, lifeViewColors[s], 0.4+0.6*t.Bio[s])
			}
		}
		return c
	case viewIndustry:
		c := mix(greyColor, rgb{120, 120, 125}, (t.Height+1)/2)
		if t.Ore > 0 {
			c = mix(c, oreColor, 0.5+t.Ore/600)
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
