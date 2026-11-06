package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/zachaller/terraformer/sim"
)

// The orbital projects, drawn around the planet: the Soletta as a tilted
// ring of glinting mirrors (its far half behind the planet, its near half in
// front), and the L1 shield as a small satellite off towards the sun with
// the edge of its field. Built projects are drawn whole; one being built
// shows as much of it as is done.

// solettaPoint is where the Soletta's mirror at angle a sits on screen, and
// whether it's on the near side of the planet.
func (g *Game) solettaPoint(a float64) (x, y float64, near bool) {
	const tilt = 0.32 // how far the ring is tipped towards the viewer
	r := g.cam.radius * 1.32
	x = g.cam.cx + r*math.Cos(a)
	y = g.cam.cy + r*math.Sin(a)*tilt
	return x, y, math.Sin(a) > 0
}

// drawOrbits draws the orbital projects over the planet. The planet's shader
// paints the whole screen, so the Soletta's far side is drawn here too, but
// only where it isn't hidden behind the planet.
func (g *Game) drawOrbits(screen *ebiten.Image) {
	p := g.planet
	if built := p.OrbitalBuilt(sim.Soletta); p.Launched(sim.Soletta) {
		const mirrors = 48
		shown := int(math.Ceil(built * mirrors))
		dot := math.Max(g.cam.radius*0.018, 1.5)
		u := &ui{g: g, dst: screen, s: 1}
		for i := range shown {
			a := float64(i) / mirrors * 2 * math.Pi
			x, y, near := g.solettaPoint(a + float64(g.ticks)*0.0004)
			behind := !near
			if behind && math.Hypot(x-g.cam.cx, y-g.cam.cy) < g.cam.radius*1.02 {
				continue // hidden behind the planet
			}
			// Each mirror glints now and then as it turns towards the sun.
			glint := math.Pow(math.Max(math.Sin(float64(g.ticks)*0.03+float64(i)*1.7), 0), 16)
			col := color.RGBA{0xC8, 0xD4, 0xE4, 0xFF}
			if behind {
				col = color.RGBA{0x7A, 0x86, 0x98, 0xFF}
			}
			u.circle(x, y, dot, col)
			if glint > 0.05 {
				u.circle(x, y, dot*(1+2*glint), withAlpha(color.RGBA{0xFF, 0xF2, 0xC0, 0xFF}, uint8(200*glint)))
			}
		}
	}
	if built := p.OrbitalBuilt(sim.L1Shield); p.Launched(sim.L1Shield) {
		// Off towards the sun (upper left), with the field's edge facing it.
		r := g.cam.radius
		x, y := g.cam.cx-r*1.55, g.cam.cy-r*1.2
		size := float32(math.Max(r*0.035, 3))
		vector.FillRect(screen, float32(x)-size, float32(y)-size*0.6, size*2, size*1.2, color.RGBA{0xDD, 0xE6, 0xF2, 0xFF}, true)
		vector.FillRect(screen, float32(x)-size*3, float32(y)-size*0.3, size*1.6, size*0.6, color.RGBA{0x3A, 0x5C, 0x9A, 0xFF}, true)
		vector.FillRect(screen, float32(x)+size*1.4, float32(y)-size*0.3, size*1.6, size*0.6, color.RGBA{0x3A, 0x5C, 0x9A, 0xFF}, true)
		if built >= 1 {
			pulse := 0.6 + 0.4*math.Sin(float64(g.ticks)*0.05)
			var arc vector.Path
			arc.Arc(float32(x+r*0.5), float32(y+r*0.4), float32(r*0.55), float32(math.Pi*1.0), float32(math.Pi*1.5), vector.Clockwise)
			vector.StrokePath(screen, &arc, &vector.StrokeOptions{Width: float32(math.Max(r*0.012, 1.5))}, &vector.DrawPathOptions{AntiAlias: true, ColorScale: colorScale(withAlpha(color.RGBA{0x73, 0xA6, 0xFF, 0xFF}, uint8(150*pulse)))})
		}
	}
}

func colorScale(c color.Color) ebiten.ColorScale {
	var cs ebiten.ColorScale
	cs.ScaleWithColor(c)
	return cs
}
