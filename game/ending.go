package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/zachaller/terraformer/sim"
)

// The ending: once the final goal is done, the lander lifts off the living
// planet and a closing card sums up the game. After that the planet runs
// on as a sandbox (a new lander can land to build again), or a new, harder
// planet can be started.

// liftoffTicks is how long the lift-off animation lasts.
const liftoffTicks = 240

// updateEnding starts the lift-off when the final goal is done, and opens
// the closing card when it's over.
func (g *Game) updateEnding() {
	p := g.planet
	if g.liftoffAt == 0 && p.GoalsDone() == len(sim.Goals) && !p.LiftedOff() {
		c, ok := p.LanderCell()
		if !ok {
			return
		}
		g.liftoffFrom = p.Dir(c)
		if err := p.LiftOff(); err != nil {
			return
		}
		g.liftoffAt = g.ticks
		g.setTool(toolInspect)
		g.focus = &c
		g.atlasDirty = true
		g.sound(sndBuildProject)
		// The next planet is a level harder.
		g.settings.Level = max(g.settings.Level, p.Level+1)
		g.saveSettings()
	}
	if g.liftoffAt > 0 && !g.endingOpen && g.ticks-g.liftoffAt >= liftoffTicks {
		g.endingOpen = true
		g.sound(sndGoal)
	}
}

// drawLiftoff draws the lander rising off the planet.
func (g *Game) drawLiftoff(screen *ebiten.Image) {
	if g.liftoffAt == 0 || g.ticks-g.liftoffAt >= liftoffTicks {
		return
	}
	x, y, ok := g.cam.project(g.liftoffFrom)
	if !ok {
		return
	}
	t := float64(g.ticks-g.liftoffAt) / liftoffTicks
	// It hovers a moment, then climbs away, shrinking into the distance.
	climb := math.Pow(max(t-0.1, 0)/0.9, 2)
	h := float64(g.h)
	y -= climb * h * 0.6
	size := g.cam.radius * 0.06 * (1 - 0.7*climb)
	size = max(size, 4)
	alpha := uint8(255 * (1 - max(t-0.85, 0)/0.15))
	// Flame, flickering.
	flick := 0.8 + 0.2*math.Sin(float64(g.ticks)*1.3)
	flame := size * (1.2 + 1.5*min(t*4, 1)) * flick
	vector.FillCircle(screen, float32(x), float32(y+size*0.6+flame*0.5), float32(size*0.35), withAlpha(color.RGBA{0xFF, 0xB0, 0x40, 0xFF}, alpha/2), true)
	var fp vector.Path
	fp.MoveTo(float32(x-size*0.35), float32(y+size*0.5))
	fp.LineTo(float32(x+size*0.35), float32(y+size*0.5))
	fp.LineTo(float32(x), float32(y+size*0.5+flame))
	fp.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(withAlpha(color.RGBA{0xFF, 0xD3, 0x6B, 0xFF}, alpha))
	vector.FillPath(screen, &fp, nil, op)
	// Body: legs, a hexagonal pad and the dome with its cyan ring.
	legC := withAlpha(color.RGBA{0x8A, 0x96, 0xAA, 0xFF}, alpha)
	for _, sx := range []float64{-1, 1} {
		vector.StrokeLine(screen, float32(x+sx*size*0.35), float32(y+size*0.2), float32(x+sx*size*0.6), float32(y+size*0.6), float32(max(size*0.08, 1)), legC, true)
	}
	var hex vector.Path
	for i := range 6 {
		a := math.Pi/6 + float64(i)*math.Pi/3
		px, py := float32(x+size*0.5*math.Cos(a)), float32(y+size*0.5*math.Sin(a))
		if i == 0 {
			hex.MoveTo(px, py)
		} else {
			hex.LineTo(px, py)
		}
	}
	hex.Close()
	op = &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(withAlpha(color.RGBA{0x5A, 0x64, 0x78, 0xFF}, alpha))
	vector.FillPath(screen, &hex, nil, op)
	vector.FillCircle(screen, float32(x), float32(y), float32(size*0.32), withAlpha(color.RGBA{0xE8, 0xEC, 0xF2, 0xFF}, alpha), true)
	vector.StrokeCircle(screen, float32(x), float32(y), float32(size*0.27), float32(max(size*0.06, 1)), withAlpha(color.RGBA{0x5A, 0xD1, 0xF5, 0xFF}, alpha), true)
}

// endingCard draws the closing card along the bottom of the screen and
// returns its top.
func (g *Game) endingCard(u *ui, sheetLeft, sheetRight, screenBottom float64) float64 {
	_, _, _, sb := currentSafeArea()
	sb /= u.s
	pad := 18.0
	x, w := sheetLeft+pad, sheetRight-sheetLeft-2*pad
	was := u.measuring
	u.measuring = true
	h := g.endingContent(u, x, 0, w)
	u.measuring = was
	top := screenBottom - sb - h - 30
	u.fillRound(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top+24, 24, colSheet)
	u.strokeRound(sheetLeft+0.5, top+0.5, sheetRight-sheetLeft-1, screenBottom-top+24, 24, 1, colTeal)
	u.area(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top, nil)
	g.endingContent(u, x, top+22, w)
	return top
}

func (g *Game) endingContent(u *ui, x, y, w float64) float64 {
	p := g.planet
	y0 := y
	u.text("A world that runs itself", x, y, sans, 20, semibold, colText)
	y += 32
	y += u.paragraph("The lander has lifted off. The planet you found frozen and airless now keeps its own warmth, water and air, and its life will carry on without you.", x, y, w, 13, colBody) + 14

	st := p.Stats()
	var chips []chip
	stat := func(icon iconName, col color.Color, label, val string) {
		chips = append(chips, chip{parts: []chipPart{iconPart(icon, col), labelPart(label), numPart(val, colText)}, fill: colCard})
	}
	stat(iconGlobe, colIcon, "Finished on day", fmt.Sprintf("%d", p.WonOnDay()))
	stat(iconBuild, colAmber, "Built", fmt.Sprintf("%d", p.Built()))
	stat(iconLeaf, colTeal, "Life", fmt.Sprintf("%.0f%%", st.LifeCover*100))
	stat(iconAir, colO2, "Oxygen", fmt.Sprintf("%.1f kPa", p.Air.O2))
	stat(iconThermo, colAmber, "Average", degrees(st.AvgTemp))
	if p.Level > 0 {
		stat(iconWarning, colRed, "Level", fmt.Sprintf("%d", p.Level))
	}
	y = u.flowChips(chips, x, x+w, y) + 16

	bw := (w - gap) / 2
	u.panel(x, y, bw, 48, 14, colCard, rgba(0x2B374A))
	lw := u.textW("Keep playing", sans, 14, semibold)
	u.text("Keep playing", x+(bw-lw)/2, y+(48-u.lineH(sans, 14, semibold))/2, sans, 14, semibold, colText)
	u.area(x, y, bw, 48, func() {
		g.endingOpen = false
		g.showMessage("The planet is yours to watch. Land again to build more", msgInfo)
	})
	nx := x + bw + gap
	label := fmt.Sprintf("New planet · level %d", g.settings.Level)
	u.panel(nx, y, bw, 48, 14, colTeal, nil)
	lw = u.textW(label, sans, 14, bold)
	u.text(label, nx+(bw-lw)/2, y+(48-u.lineH(sans, 14, bold))/2, sans, 14, bold, rgba(0x06231E))
	u.area(nx, y, bw, 48, func() {
		g.endingOpen = false
		g.newPlanet()
	})
	y += 48
	return y - y0
}
