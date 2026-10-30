package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/zachaller/terraformer/sim"
)

// Placing a building is two steps: tapping a tile previews it there (the
// pending tile), and the Build button confirms. Tapping elsewhere moves the
// preview, which is forgiving of fingers on a small, rotating globe.

// placing returns the building type being placed, if a build tool is active.
func (g *Game) placing() (sim.BuildingType, bool) {
	if !g.planet.Landed() {
		return 0, false
	}
	return g.tool.building()
}

// confirmPlacement builds the pending building.
func (g *Game) confirmPlacement() {
	bt, ok := g.placing()
	if !ok || g.pending == nil {
		return
	}
	c := *g.pending
	p := g.planet
	if _, err := p.Build(bt, c); err != nil {
		g.showMessage(err.Error(), msgWarn)
		return
	}
	g.pending = nil
	g.atlasDirty = true
	g.sound(sndBuild)
	b := p.BuildingAt(c)
	switch {
	case p.NetworkOf(b).HasDepot:
		g.showMessage(fmt.Sprintf("Built a %s, connected to the colony", lower(bt.String())), msgGood)
	case len(p.NetworkOf(b).Members) > 1:
		g.showMessage(fmt.Sprintf("Built a %s. Its network doesn't reach the colony", lower(bt.String())), msgInfo)
	case b.Spec().PowerIn > 0:
		g.showMessageAction(fmt.Sprintf("Built a %s, but nothing connects it", lower(bt.String())), msgWarn, "Add a pylon", func() {
			g.setTool(buildTool(sim.Pylon))
			g.showMessage("Tap a tile within 4 tiles of it to place a pylon", msgInfo)
		})
	default:
		g.showMessage(fmt.Sprintf("Built a %s", lower(bt.String())), msgGood)
	}
}

// hubsInReach lists the hubs (pylons and the lander) a hub of type t placed
// on c would join.
func (g *Game) hubsInReach(t sim.BuildingType, c sim.Cell) []*sim.Building {
	p := g.planet
	var out []*sim.Building
	for _, b := range p.Buildings() {
		if b.Spec().Reach > 0 && p.Grid.CellAt(b.Tile) != c && p.Distance(c, p.Grid.CellAt(b.Tile)) <= math.Max(b.Spec().Reach, sim.Specs[t].Reach) {
			out = append(out, b)
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// drawReach draws the reach of a pylon being placed: a dashed circle as far
// as it can join other hubs.
func (g *Game) drawReach(screen *ebiten.Image) {
	bt, ok := g.placing()
	if !ok || g.pending == nil || sim.Specs[bt].Reach == 0 {
		return
	}
	p := g.planet
	s := ebiten.Monitor().DeviceScaleFactor()
	a := p.Dir(*g.pending)
	reach := sim.Specs[bt].Reach * p.Grid.TileAngle()
	ref := sim.Vec3{Y: 1}
	if math.Abs(a.Y) > 0.9 {
		ref = sim.Vec3{X: 1}
	}
	e1 := a.Cross(ref).Normalize()
	e2 := a.Cross(e1)
	pt := func(k int) sim.Vec3 {
		th := 2 * math.Pi * float64(k) / 96
		return a.Scale(math.Cos(reach)).Add(e1.Scale(math.Cos(th) * math.Sin(reach))).Add(e2.Scale(math.Sin(th) * math.Sin(reach)))
	}
	for i := 0; i < 96; i += 2 {
		x0, y0, v0 := g.cam.project(pt(i))
		x1, y1, v1 := g.cam.project(pt(i + 1))
		if v0 && v1 {
			vector.StrokeLine(screen, f32(x0), f32(y0), f32(x1), f32(y1), f32(2*s), withAlpha(rgba(0x5AD1F5), 200), true)
		}
	}
}

// isHubType reports whether buildings of type t are network hubs.
func isHubType(t sim.BuildingType) bool { return sim.Specs[t].Reach > 0 }

// connectionRow describes how a building placed on c would join a network,
// for the placement preview.
func (g *Game) connectionRow(bt sim.BuildingType, c sim.Cell) previewRow {
	p := g.planet
	via, ok, colony, full := p.Connection(bt, c)
	switch {
	case full:
		return previewRow{iconLink, colAmber, "Not connected", "nearby pylons are full", colAmber}
	case ok && colony && !isHubType(bt):
		hub := p.BuildingAt(via)
		load, capacity := p.HubLoad(hub)
		return previewRow{iconLink, rgba(0x5AD1F5), "Connects", fmt.Sprintf("via %s · load %.0f/%.0f", lower(hub.Spec().Name), load+math.Max(p.Distance(c, via), 1), capacity), colTeal}
	case ok && colony:
		n := len(g.hubsInReach(bt, c))
		return previewRow{iconLink, rgba(0x5AD1F5), "Joins", fmt.Sprintf("%d %s · the colony", n, plural(n, "hub", "hubs")), colTeal}
	case ok:
		return previewRow{iconLink, rgba(0x5AD1F5), "Connects", "not to the colony", colAmber}
	}
	if bt == sim.Pylon {
		return previewRow{iconLink, colAmber, "Joins", fmt.Sprintf("no hub within %.0f tiles", sim.Specs[bt].Reach), colAmber}
	}
	return previewRow{iconLink, colAmber, "Not connected", "needs a pylon within 4", colAmber}
}

// maxAreaCells is the most tiles a placement preview tints: four rings of
// hexes. The shader has arrays of this size.
const maxAreaCells = 90

// heatShownMin is the least warming, in °C, a heater preview tints a ring
// for.
const heatShownMin = 1.5

// areaTint is one tile a placement preview tints, and how strongly.
type areaTint struct {
	cell   sim.Cell
	amount float64 // 0 to 1
}

// placementArea lists the tiles building bt on c would affect, each with how
// strongly to tint it, and the tint's colour. A heater's area fades out
// with the warming each ring gets (see sim.Grid.HeatRings); mines and
// smelters pollute evenly out to their reach.
func (g *Game) placementArea(bt sim.BuildingType, c sim.Cell) ([]areaTint, color.RGBA) {
	const full = 0.38
	var out []areaTint
	switch bt {
	case sim.HeaterMachine:
		rings := g.heatRings
		n := 0
		for n+1 < len(rings) && rings[n+1] >= heatShownMin {
			n++
		}
		for _, a := range areaCells(g.planet.Grid, c, n) {
			// Fade with the warming, but keep faint rings visible.
			out = append(out, areaTint{a.cell, math.Max(full*rings[a.ring]/rings[1], 0.12)})
		}
		return out, colAmber
	case sim.Mine, sim.Smelter:
		steps := 1
		if bt == sim.Smelter {
			steps = 2
		}
		for _, a := range areaCells(g.planet.Grid, c, steps) {
			out = append(out, areaTint{a.cell, full})
		}
		return out, rgba(0xB07CA0)
	}
	// Other area effects fade out linearly over their reach.
	spec := sim.Specs[bt]
	fade := func(rings int, col color.RGBA) ([]areaTint, color.RGBA) {
		for _, a := range areaCells(g.planet.Grid, c, rings) {
			out = append(out, areaTint{a.cell, math.Max(full*(1-float64(a.ring-1)/float64(rings)), 0.12)})
		}
		return out, col
	}
	switch {
	case spec.HeatRings > 0:
		return fade(spec.HeatRings, colAmber)
	case spec.DustRings > 0:
		return fade(spec.DustRings, rgba(0xB08A6A))
	case spec.SeedRings > 0:
		return fade(spec.SeedRings, colTeal)
	case spec.CoverRadius > 0:
		// Its cover faintly, and the hubs it would join strongly.
		for _, a := range areaCells(g.planet.Grid, c, int(spec.CoverRadius)) {
			out = append(out, areaTint{a.cell, 0.2})
		}
		for _, h := range g.hubsInReach(bt, c) {
			if len(out) < maxAreaCells {
				out = append(out, areaTint{g.planet.Grid.CellAt(h.Tile), 0.7})
			}
		}
		return out, rgba(0x5AD1F5)
	case bt == sim.Comet:
		for _, a := range areaCells(g.planet.Grid, c, 2) {
			out = append(out, areaTint{a.cell, full})
		}
		return out, colRed
	}
	return nil, colAmber
}

// hubCoverage is the area to shade while inspecting a hub (a pylon or the
// lander): the tiles it covers faintly, and the buildings it connects
// strongly.
func (g *Game) hubCoverage() ([]areaTint, bool) {
	if !g.inspectOpen() {
		return nil, false
	}
	p := g.planet
	hub := p.BuildingAt(*g.selected)
	if hub == nil || hub.Spec().CoverRadius == 0 {
		return nil, false
	}
	connected := map[int]bool{}
	for _, s := range p.Spokes() {
		if s[0] == hub.Tile {
			connected[s[1]] = true
		}
	}
	var out []areaTint
	for _, a := range areaCells(p.Grid, *g.selected, int(math.Ceil(hub.Spec().CoverRadius))) {
		if p.Distance(*g.selected, a.cell) > hub.Spec().CoverRadius {
			continue
		}
		amount := 0.15
		if connected[p.Grid.Index(a.cell)] {
			amount = 0.5
		}
		out = append(out, areaTint{a.cell, amount})
	}
	return out, true
}

// ringCell is a tile and how many neighbour-steps it is from somewhere.
type ringCell struct {
	cell sim.Cell
	ring int
}

// areaCells lists the tiles within steps neighbour-steps of c (not c itself),
// nearest first, following the same neighbours as the simulation.
func areaCells(grid sim.Grid, c sim.Cell, steps int) []ringCell {
	seen := map[sim.Cell]bool{c: true}
	frontier := []sim.Cell{c}
	var out []ringCell
	for r := 1; r <= steps; r++ {
		var next []sim.Cell
		for _, f := range frontier {
			for _, nb := range grid.Neighbors(f) {
				if !seen[nb] && len(out) < maxAreaCells {
					seen[nb] = true
					out = append(out, ringCell{nb, r})
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}
	return out
}

// modeBanner replaces the goal card while placing or linking, saying what to
// do. It returns its bottom.
func (g *Game) modeBanner(u *ui, left, right, y float64) float64 {
	h := 60.0
	u.panel(left, y, right-left, h, 16, withAlpha(colSheet, 240), rgba(0x2B374A))
	var title, sub, cost string
	if bt, ok := g.placing(); ok {
		g.buildingGlyph(u, bt, left+14, y+16, 28)
		title = "Placing " + bt.String()
		sub = "Tap a tile to preview it there"
		if g.pending != nil {
			sub = "Tap another tile to move it, or Build here"
		}
		cost = fmt.Sprintf("%.0f metal", sim.Specs[bt].Cost)
	}
	u.text(title, left+54, y+11, sans, 14, semibold, colText)
	u.text(sub, left+54, y+32, sans, 12, regular, colBody)
	if cost != "" {
		u.textRight(cost, right-14, y+12, mono, 13, semibold, colText)
	}
	return y + h
}

// previewRow is one line of the placement preview card.
type previewRow struct {
	icon   iconName
	iconC  color.Color
	label  string
	value  string
	valueC color.Color
}

// placementPreview describes what building bt would do on c.
func (g *Game) placementPreview(bt sim.BuildingType, c sim.Cell) (rows []previewRow, warning string) {
	p := g.planet
	t := p.Tile(c)
	spec := sim.Specs[bt]
	rows = append(rows, previewRow{iconThermo, colBlue, "This tile", t.Kind().String() + " · " + degrees(t.Temp), colIcon})
	switch bt {
	case sim.HeaterMachine:
		rings := g.heatRings
		rows = append(rows, previewRow{iconThermo, colAmber, "Warms", fmt.Sprintf("+%.0f° here · then +%.0f°, +%.0f°, +%.0f°", rings[0], rings[1], rings[2], rings[3]), rgba(0xFFD36B)})
		if t.Ice > 0 {
			fate, col := "stays frozen", color.Color(colMuted)
			if t.Temp+rings[0] > 0 {
				fate, col = "will melt", colBlue
			}
			rows = append(rows, previewRow{iconDrop, colBlue, "Ice here", fmt.Sprintf("%.2f · %s", t.Ice, fate), col})
		}
		if t.Pollution >= sim.MaxLifePollution {
			warning = "This tile is polluted. It will warm, but microbes won't grow here."
		} else if pb, d := p.NearestPolluter(c, 2.5); pb != nil {
			warning = fmt.Sprintf("A %s is %.0f tiles away. Its pollution may stop microbes growing here.", lower(pb.Spec().Name), math.Max(d, 1))
		}
	case sim.Mine:
		if t.Ore > 0 {
			rows = append(rows, previewRow{iconMetal, mineralColor(t.Mineral), t.Mineral.String(), fmt.Sprintf("%.0f · lasts ~%.0f days", t.Ore, t.Ore/spec.Extracts/10), colText})
		}
		warning = "Mines pollute their own tile: no microbes will grow here."
	case sim.Smelter:
		warning = "Smelters pollute up to 2 tiles around. Keep them away from heated ground."
	case sim.SolarPanel, sim.Geothermal:
		rows = append(rows, previewRow{iconBolt, colAmber, "Makes", fmt.Sprintf("%.0f power once connected", spec.PowerOut), colText})
		if bt == sim.Geothermal && !t.Volcanic {
			warning = "Geothermal plants need a volcanic tile (red in the Industry view)."
		}
	case sim.WindTurbine:
		rows = append(rows, previewRow{iconBolt, colAmber, "Makes now", fmt.Sprintf("%.1f of %.0f power", spec.PowerOut*p.WindFactor(), spec.PowerOut), colText})
		if p.WindFactor() < 0.2 {
			warning = "The air is still thin: turbines make little power until it's several kPa."
		}
	case sim.IceDrill:
		rows = append(rows, previewRow{iconDrop, colBlue, "Ice here", fmt.Sprintf("%.2f", t.Ice), colText})
		if t.Ice < 0.05 {
			warning = "Ice drills need ice to drill."
		}
	case sim.DustSpreader:
		rows = append(rows, previewRow{iconAir, colIcon, "Dusts ice", fmt.Sprintf("up to %d tiles around", spec.DustRings), colText})
		if t.DryIce <= 0 && t.Ice < 0.3 {
			warning = "There's little ice here. Dust spreaders do most good on the polar caps."
		}
	case sim.GreenhouseFactory:
		rows = append(rows, previewRow{iconThermo, colAmber, "Makes", fmt.Sprintf("+%.1f Pa a year of PFCs", spec.Emits.PFC*3650), rgba(0xFFD36B)})
		rows = append(rows, previewRow{iconMetal, mineralColor(sim.Fluorite), "Fluorite in stores", fmt.Sprintf("%.1f", p.Stores()[sim.FluoriteRes]), colText})
	case sim.Electrolyser:
		rows = append(rows, previewRow{iconAir, colTeal, "Makes", fmt.Sprintf("+%.2f kPa a year of O2", spec.Emits.O2*3650), colText})
		rows = append(rows, previewRow{iconDrop, colBlue, "Water in stores", fmt.Sprintf("%.1f", p.Stores()[sim.Water]), colText})
	case sim.NitrogenPlant:
		rows = append(rows, previewRow{iconAir, colBlue, "Makes", fmt.Sprintf("+%.2f kPa a year of N2", spec.Emits.N2*3650), colText})
		rows = append(rows, previewRow{iconMetal, mineralColor(sim.Nitrate), "Nitrate in stores", fmt.Sprintf("%.1f", p.Stores()[sim.NitrateRes]), colText})
	case sim.OrbitalMirror:
		rows = append(rows, previewRow{iconThermo, colAmber, "Warms", fmt.Sprintf("+%.0f° here, fading over %d tiles", spec.Heat, spec.HeatRings), rgba(0xFFD36B)})
		if t.DryIce > 0 {
			rows = append(rows, previewRow{iconAir, colIcon, "Dry ice here", "will turn to CO2", colBlue})
		}
	case sim.MagneticShield:
		rows = append(rows, previewRow{iconAir, colAmber, "Air lost now", fmt.Sprintf("%.3f kPa a year", p.EscapePerDay()*365), colText})
	case sim.Biolab:
		rows = append(rows, previewRow{iconLeaf, colTeal, "Seeds life", fmt.Sprintf("up to %d tiles around", spec.SeedRings), colText})
	case sim.Pylon:
		rows = append(rows, previewRow{iconLink, rgba(0x5AD1F5), "Carries", fmt.Sprintf("load %.0f within %.0f tiles", spec.Capacity, spec.CoverRadius), colText})
	case sim.CarbonScrubber:
		rows = append(rows, previewRow{iconAir, colCO2, "Removes", fmt.Sprintf("%.2f kPa a year of CO2", -spec.Emits.CO2*3650), colText})
	case sim.Comet:
		rows = append(rows, previewRow{iconDrop, colBlue, "Brings", "water, nitrogen and CO2", colText})
		rows = append(rows, previewRow{iconWarning, colRed, "Strikes in", fmt.Sprintf("%d days", spec.Countdown/10), colText})
		warning = "The impact destroys everything within two tiles and leaves a crater."
	}
	if bt != sim.Comet && bt != sim.OrbitalMirror {
		rows = append(rows, g.connectionRow(bt, c))
	}
	if spec.PowerIn > 0 && p.Landed() {
		n := p.NetworkOf(p.Buildings()[0])
		after := fmt.Sprintf("%.0f / %.0f", n.PowerSupply, n.PowerDemand+spec.PowerIn)
		col := color.Color(colTeal)
		if n.PowerSupply < n.PowerDemand+spec.PowerIn {
			col = colAmber
		}
		rows = append(rows, previewRow{iconBolt, colAmber, fmt.Sprintf("Uses %.0f power · colony then", spec.PowerIn), after, col})
	}
	return rows, warning
}

// placementPanel draws the preview card and confirm bar, ending at bottom,
// and returns the top.
func (g *Game) placementPanel(u *ui, left, right, bottom float64) float64 {
	bt, _ := g.placing()
	c := *g.pending
	err := g.planet.CanBuild(bt, c)

	// Confirm bar.
	barH := 64.0
	by := bottom - barH
	u.panel(left, by, 64, barH, 18, colChip, rgba(0x2B374A))
	u.icon(iconCross, left+20, by+20, 24, colIcon)
	u.area(left, by, 64, barH, func() { g.pending = nil })
	bx, bw := left+64+gap, right-left-64-gap
	label := fmt.Sprintf("Build here · %.0f metal", sim.Specs[bt].Cost)
	fill, fg := color.Color(colAmber), color.Color(colOnAmber)
	if err != nil {
		fill, fg = colCard, colMuted
	}
	u.panel(bx, by, bw, barH, 18, fill, nil)
	lw := u.textW(label, sans, 15, bold) + 30
	lx := bx + (bw-lw)/2
	u.icon(iconCheck, lx, by+20, 24, fg)
	u.text(label, lx+30, by+(barH-u.lineH(sans, 15, bold))/2, sans, 15, bold, fg)
	if err == nil {
		u.area(bx, by, bw, barH, g.confirmPlacement)
	}

	// Preview card.
	rows, warning := g.placementPreview(bt, c)
	inner := right - left - 28
	rowH := 26.0
	// A value too long to sit beside its label goes on a line of its own.
	wraps := func(r previewRow) bool {
		return u.textW(r.label, sans, 13, regular)+26+12+u.textW(r.value, mono, 12, semibold) > inner
	}
	h := 14.0
	for _, r := range rows {
		h += rowH
		if wraps(r) {
			h += 18
		}
	}
	note, noteFill, noteBorder, noteFg := warning, color.Color(rgba(0x2A1E26)), color.Color(rgba(0x5B3B52)), color.Color(rgba(0xF0D2E6))
	if err != nil {
		note, noteFill, noteBorder, noteFg = err.Error(), colRedBg, colRedLn, rgba(0xFFC9C0)
	}
	noteH := 0.0
	if note != "" {
		was := u.measuring
		u.measuring = true
		noteH = u.paragraph(note, 0, 0, inner-50, 12, noteFg) + 20
		u.measuring = was
		h += noteH + 6
	}
	h += 8
	top := by - gap - h
	u.panel(left, top, right-left, h, 16, withAlpha(colSheet, 245), rgba(0x2B374A))
	y := top + 12
	x := left + 14
	for _, r := range rows {
		u.icon(r.icon, x, y+(rowH-16)/2, 16, r.iconC)
		u.text(r.label, x+26, y+(rowH-u.lineH(sans, 13, regular))/2, sans, 13, regular, colIcon)
		if wraps(r) {
			y += 18
		}
		u.textRight(r.value, x+inner, y+(rowH-u.lineH(mono, 12, semibold))/2, mono, 12, semibold, r.valueC)
		y += rowH
	}
	if note != "" {
		y += 6
		u.panel(x, y, inner, noteH, 12, noteFill, noteBorder)
		u.icon(iconWarning, x+12, y+10, 16, noteFg)
		u.paragraph(note, x+38, y+10, inner-50, 12, noteFg)
	}
	return top
}
