package game

import (
	"fmt"
	"image/color"

	"github.com/zachaller/terraformer/sim"
)

// openColony shows the colony overview, closing anything else that's open.
func (g *Game) openColony() {
	g.setTool(toolInspect)
	g.colonyOpen = true
}

// colonySheet draws the colony overview along the bottom of the screen and
// returns its top.
func (g *Game) colonySheet(u *ui, sheetLeft, sheetRight, screenBottom float64) float64 {
	_, _, _, sb := currentSafeArea()
	sb /= u.s
	pad := 18.0
	x, w := sheetLeft+pad, sheetRight-sheetLeft-2*pad

	was := u.measuring
	u.measuring = true
	h := g.colonyContent(u, x, 0, w)
	u.measuring = was

	top := screenBottom - sb - h - 30
	u.fillRound(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top+24, 24, colSheet)
	u.strokeRound(sheetLeft+0.5, top+0.5, sheetRight-sheetLeft-1, screenBottom-top+24, 24, 1, rgba(0x2B374A))
	u.area(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top, nil)
	u.fillRound(sheetLeft+(sheetRight-sheetLeft-40)/2, top+8, 40, 4, 2, rgba(0x3A4658))
	g.colonyContent(u, x, top+20, w)
	return top
}

// resourceColor is how resource r is shown.
func resourceColor(r sim.Resource) color.Color {
	switch r {
	case sim.Metal:
		return colIcon
	case sim.Ore:
		return mineralColor(sim.Iron)
	case sim.Water:
		return colBlue
	case sim.FluoriteRes:
		return mineralColor(sim.Fluorite)
	case sim.NitrateRes:
		return mineralColor(sim.Nitrate)
	}
	return colIcon
}

// colonyContent lays out the overview from y and returns its height: the
// stores and what's being produced, power, and the buildings by type.
func (g *Game) colonyContent(u *ui, x, y, w float64) float64 {
	p := g.planet
	y0 := y
	u.text("Colony", x, y, sans, 20, semibold, colText)
	u.icon(iconCross, x+w-28, y+2, 24, colMuted)
	u.area(x+w-44, y-8, 44, 44, func() { g.colonyOpen = false })
	y += 36

	// Resources: in the stores, and made per day.
	u.text("RESOURCES", x, y, sans, 11, semibold, colMuted)
	u.textRight("stores · per day", x+w, y, sans, 11, regular, colMuted)
	y += u.lineH(sans, 11, semibold) + 4
	for r := range sim.NumResources {
		res := sim.Resource(r)
		stock, rate := p.Stores()[res], g.produceRate(res)
		if stock < 0.05 && rate < 0.005 && res != sim.Metal {
			continue
		}
		rh := 26.0
		u.circle(x+5, y+rh/2, 5, resourceColor(res))
		u.text(res.String(), x+18, y+(rh-u.lineH(sans, 13, regular))/2, sans, 13, regular, colText)
		rateS, rateCol := "–", color.Color(colMuted)
		if rate >= 0.005 {
			rateS, rateCol = fmt.Sprintf("+%.1f", rate), colTeal
		}
		u.textRight(rateS, x+w, y+(rh-u.lineH(mono, 13, semibold))/2, mono, 13, semibold, rateCol)
		u.textRight(fmt.Sprintf("%.1f", stock), x+w-64, y+(rh-u.lineH(mono, 13, semibold))/2, mono, 13, semibold, colText)
		y += rh
	}
	y += 10

	// Power: the colony's network, and any outposts.
	u.text("POWER", x, y, sans, 11, semibold, colMuted)
	y += u.lineH(sans, 11, semibold) + 6
	var colony *sim.Network
	outposts := map[*sim.Network]bool{}
	for _, b := range p.Buildings() {
		n := p.NetworkOf(b)
		if n.HasDepot {
			colony = n
		} else if len(n.Members) > 1 || b.Spec().PowerIn > 0 {
			outposts[n] = true
		}
	}
	var chips []chip
	if colony != nil {
		col := color.Color(colTeal)
		if colony.PowerSatisfaction() < 1 {
			col = colAmber
		}
		chips = append(chips, chip{parts: []chipPart{iconPart(iconBolt, colAmber), labelPart("Colony"), numPart(fmt.Sprintf("%.0f / %.0f", colony.PowerSupply, colony.PowerDemand), col)}, fill: colCard})
	}
	if len(outposts) > 0 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconLink, colIcon), numPart(fmt.Sprint(len(outposts)), colText), labelPart(plural(len(outposts), "outpost network", "outpost networks"))}, fill: colCard})
	}
	y = u.flowChips(chips, x, x+w, y) + 14

	// Buildings by type, with how many aren't running fully.
	u.text("BUILDINGS", x, y, sans, 11, semibold, colMuted)
	y += u.lineH(sans, 11, semibold) + 6
	var count, idle [sim.NumBuildingTypes]int
	var stuck [sim.NumBuildingTypes]*sim.Building
	for _, b := range p.Buildings() {
		count[b.Type]++
		if p.Diagnose(b).Kind != sim.StatusRunning {
			idle[b.Type]++
			if stuck[b.Type] == nil {
				stuck[b.Type] = b
			}
		}
	}
	chips = chips[:0]
	for bt := range sim.NumBuildingTypes {
		if count[bt] == 0 {
			continue
		}
		parts := []chipPart{labelPart(sim.Specs[bt].Name), numPart(fmt.Sprint(count[bt]), colText)}
		if idle[bt] > 0 {
			parts = append(parts, chipPart{text: fmt.Sprintf("%d held up", idle[bt]), kind: sans, size: 11, weight: semibold, col: colAmber})
		}
		c := chip{parts: parts, fill: colCard}
		if b := stuck[bt]; b != nil {
			// Tap to see one of the held-up buildings.
			c.action = func() {
				cell := p.Grid.CellAt(b.Tile)
				g.setTool(toolInspect)
				g.selected, g.focus = &cell, &cell
			}
		}
		chips = append(chips, c)
	}
	y = u.flowChips(chips, x, x+w, y)
	return y - y0 + 6
}
