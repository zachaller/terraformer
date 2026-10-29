package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/zachaller/terraformer/sim"
)

// openAir shows the atmosphere sheet, closing anything else that's open.
func (g *Game) openAir() {
	g.setTool(toolInspect)
	g.selected = nil
	g.airOpen = true
}

// airSheet draws the atmosphere sheet along the bottom of the screen and
// returns its top.
func (g *Game) airSheet(u *ui, sheetLeft, sheetRight, screenBottom float64) float64 {
	_, _, _, sb := currentSafeArea()
	sb /= u.s
	pad := 18.0
	x, w := sheetLeft+pad, sheetRight-sheetLeft-2*pad

	was := u.measuring
	u.measuring = true
	h := g.airContent(u, x, 0, w)
	u.measuring = was

	top := screenBottom - sb - h - 30
	u.fillRound(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top+24, 24, colSheet)
	u.strokeRound(sheetLeft+0.5, top+0.5, sheetRight-sheetLeft-1, screenBottom-top+24, 24, 1, rgba(0x2B374A))
	u.area(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top, nil)
	u.fillRound(sheetLeft+(sheetRight-sheetLeft-40)/2, top+8, 40, 4, 2, rgba(0x3A4658))
	g.airContent(u, x, top+20, w)
	return top
}

// Gas colours, shared by the bars and their labels.
var (
	colCO2 = rgba(0xC9A27A)
	colN2  = rgba(0x7FA7E8)
	colO2  = rgba(0x5FD3B5)
	colPFC = rgba(0xE07A5F)
)

// airContent lays out the atmosphere sheet from y and returns its height:
// what the air is made of, how warm it keeps the planet, and what's
// changing it.
func (g *Game) airContent(u *ui, x, y, w float64) float64 {
	p := g.planet
	a := p.Air
	y0 := y
	u.text("Atmosphere", x, y, sans, 20, semibold, colText)
	u.icon(iconCross, x+w-28, y+2, 24, colMuted)
	u.area(x+w-44, y-8, 44, 44, func() { g.airOpen = false })
	y += 34

	total := p.Pressure()
	u.text(pressure(total)+" kPa", x, y, mono, 22, semibold, colText)
	u.textRight("Earth: 101 kPa · Mars: 0.6", x+w, y+8, sans, 12, regular, colMuted)
	y += 34

	// Composition bar.
	gases := []struct {
		name string
		v    float64
		col  color.Color
	}{
		{"CO2", a.CO2, colCO2},
		{"N2", a.N2, colN2},
		{"O2", a.O2, colO2},
	}
	bx := x
	if u.drawing() {
		u.fillRound(x, y, w, 12, 6, colTrack)
		for _, gs := range gases {
			bw := w * gs.v / math.Max(total, 1e-9)
			if bw > 0.5 {
				u.fillRound(bx, y, bw, 12, 3, gs.col)
			}
			bx += bw
		}
	}
	y += 20
	for _, gs := range gases {
		share := gs.v / math.Max(total, 1e-9) * 100
		g.airRow(u, x, y, w, gs.name, fmt.Sprintf("%s kPa", pressure(gs.v)), fmt.Sprintf("%.0f%%", share), gs.col)
		y += 26
	}
	g.airRow(u, x, y, w, "Super-greenhouse gas", fmt.Sprintf("%.2f Pa", a.PFC), "", colPFC)
	y += 34

	// Greenhouse warming, by cause.
	gh := p.Greenhouse()
	u.text("GREENHOUSE WARMING", x, y, sans, 11, semibold, colMuted)
	u.textRight("+"+degrees(gh.Total()), x+w, y-2, mono, 13, semibold, colAmber)
	y += u.lineH(sans, 11, semibold) + 6
	for _, r := range []struct {
		label string
		v     float64
	}{{"CO2", gh.CO2}, {"Super-greenhouse gas", gh.PFC}, {"Thicker air", gh.Broadening}, {"Water vapour", gh.Vapour}} {
		g.airRow(u, x, y, w, r.label, "+"+degrees(r.v), "", colAmber)
		y += 26
	}
	y += 8

	// Water, caps, shielding, and what's changing.
	u.text("CONDITIONS", x, y, sans, 11, semibold, colMuted)
	y += u.lineH(sans, 11, semibold) + 6
	water := "Ice turns straight to vapour"
	waterCol := color.Color(colRed)
	if total >= sim.TriplePoint {
		water, waterCol = fmt.Sprintf("Liquid from 0° to %s", degrees(p.BoilingPoint())), colTeal
	}
	rows := []struct {
		label, val string
		col        color.Color
	}{
		{"Water", water, waterCol},
		{"Polar caps (dry ice)", fmt.Sprintf("%.0f%% left", g.capsLeft()*100), colBody},
		{"Ozone layer", fmt.Sprintf("%.0f%%", p.Ozone()*100), colBody},
		{"Humidity", fmt.Sprintf("%.0f%%", p.Humidity()*100), colBody},
	}
	if esc := p.EscapePerDay(); esc > 0 {
		rows = append(rows, struct {
			label, val string
			col        color.Color
		}{"Lost to space", fmt.Sprintf("%.3f kPa a year", esc*365), colAmber})
	} else {
		rows = append(rows, struct {
			label, val string
			col        color.Color
		}{"Lost to space", "None: shielded", colTeal})
	}
	for _, r := range rows {
		u.text(r.label, x, y+(26-u.lineH(sans, 13, regular))/2, sans, 13, regular, colText)
		u.textRight(r.val, x+w, y+(26-u.lineH(sans, 13, semibold))/2, sans, 13, semibold, r.col)
		y += 26
	}
	return y - y0
}

// airRow is one line of the atmosphere sheet: a colour dot, label, value and
// an optional share.
func (g *Game) airRow(u *ui, x, y, w float64, label, val, share string, col color.Color) {
	u.circle(x+5, y+13, 5, col)
	u.text(label, x+18, y+(26-u.lineH(sans, 13, regular))/2, sans, 13, regular, colText)
	right := x + w
	if share != "" {
		u.textRight(share, right, y+(26-u.lineH(mono, 12, regular))/2, mono, 12, regular, colMuted)
		right -= 48
	}
	u.textRight(val, right, y+(26-u.lineH(mono, 13, semibold))/2, mono, 13, semibold, colText)
}

// capsLeft is the share of the starting polar dry ice still frozen.
func (g *Game) capsLeft() float64 {
	return g.planet.DryIceLeft()
}
