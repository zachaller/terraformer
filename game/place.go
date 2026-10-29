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
	if _, err := g.planet.Build(bt, c); err != nil {
		g.showMessage(err.Error(), msgWarn)
		return
	}
	g.pending = nil
	g.atlasDirty = true
	g.showMessageAction(fmt.Sprintf("Built a %s. Link it to a network to run it", bt), msgGood, "Link it", func() {
		g.setTool(toolLink)
		g.linkFrom = &c
		g.showMessage("Drag from the new building to another, or tap the one to link it to", msgInfo)
	})
}

// maxAreaCells is the most tiles a placement preview tints: three rings of
// hexes. The shader has arrays of this size.
const maxAreaCells = 36

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
	return nil, colAmber
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
	} else {
		u.icon(iconLink, left+16, y+18, 24, rgba(0x5AD1F5))
		title = "Linking"
		sub = "Drag from one building to another"
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
			rows = append(rows, previewRow{iconMetal, rgba(0xF2C04A), "Ore deposit", fmt.Sprintf("%.0f · lasts ~%.0f days", t.Ore, t.Ore/spec.Outputs[sim.Ore]/10), colText})
		}
		warning = "Mines pollute their own tile: no microbes will grow here."
	case sim.Smelter:
		warning = "Smelters pollute up to 2 tiles around. Keep them away from heated ground."
	case sim.SolarPanel:
		rows = append(rows, previewRow{iconBolt, colAmber, "Makes", fmt.Sprintf("%.0f power once linked", spec.PowerOut), colText})
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
	h := 14 + float64(len(rows))*rowH
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

// linkDrag is a link being made by dragging from a building.
type linkDrag struct {
	from sim.Cell
	x, y float64 // finger position, screen pixels
}

// linkTarget is the building under the finger during a link drag, if any.
func (g *Game) linkTarget() (sim.Cell, bool) {
	c, ok := g.cam.pick(g.planet.Grid, g.linkDrag.x, g.linkDrag.y)
	if !ok || g.planet.BuildingAt(c) == nil || c == g.linkDrag.from {
		return sim.Cell{}, false
	}
	return c, true
}

// finishLinkDrag links the drag's building to the one it was released on.
func (g *Game) finishLinkDrag() {
	d := g.linkDrag
	g.linkDrag = nil
	target, ok := g.cam.pick(g.planet.Grid, d.x, d.y)
	if !ok || g.planet.BuildingAt(target) == nil {
		g.showMessage("Let go over a building to link to it", msgInfo)
		return
	}
	if target == d.from {
		return
	}
	cost := g.planet.LinkCost(d.from, target)
	if err := g.planet.Link(d.from, target); err != nil {
		g.showMessage(err.Error(), msgWarn)
		return
	}
	g.linkFrom = nil
	g.showMessage(fmt.Sprintf("Linked for %.1f metal", cost), msgGood)
}

var (
	colReach   = rgba(0x5AD1F5)
	colPreview = rgba(0xFFE08A)
)

// drawLinkPreview draws the link being dragged: the source's reach, and a
// dashed line from it to the finger.
func (g *Game) drawLinkPreview(screen *ebiten.Image) {
	d := g.linkDrag
	if d == nil {
		return
	}
	p := g.planet
	s := ebiten.Monitor().DeviceScaleFactor()
	a := p.Dir(d.from)

	// Reach: a dashed circle MaxLinkLength tiles around the source.
	reach := sim.MaxLinkLength * p.Grid.TileAngle()
	ref := sim.Vec3{Y: 1}
	if math.Abs(a.Y) > 0.9 {
		ref = sim.Vec3{X: 1}
	}
	e1 := a.Cross(ref).Normalize()
	e2 := a.Cross(e1)
	const n = 72
	for i := 0; i < n; i += 2 {
		pt := func(k int) sim.Vec3 {
			th := 2 * math.Pi * float64(k) / n
			return a.Scale(math.Cos(reach)).Add(e1.Scale(math.Cos(th) * math.Sin(reach))).Add(e2.Scale(math.Sin(th) * math.Sin(reach)))
		}
		x0, y0, v0 := g.cam.project(pt(i))
		x1, y1, v1 := g.cam.project(pt(i + 1))
		if v0 && v1 {
			vector.StrokeLine(screen, f32(x0), f32(y0), f32(x1), f32(y1), f32(2*s), withAlpha(colReach, 200), true)
		}
	}

	// Dashed line to the finger, along the surface when it's over the planet.
	clr := colPreview
	if t, ok := g.linkTarget(); ok && p.CanLink(d.from, t) != nil {
		clr = colRed
	}
	sx, sy, _ := g.cam.project(a)
	b, onPlanet := g.cam.pickDir(d.x, d.y)
	const segs = 24
	px, py := sx, sy
	for i := 1; i <= segs; i++ {
		t := float64(i) / segs
		var x, y float64
		if onPlanet {
			x, y, _ = g.cam.project(a.Scale(1 - t).Add(b.Scale(t)).Normalize())
		} else {
			x, y = sx+(d.x-sx)*t, sy+(d.y-sy)*t
		}
		if i%2 == 1 {
			vector.StrokeLine(screen, f32(px), f32(py), f32(x), f32(y), f32(3.5*s), clr, true)
		}
		px, py = x, y
	}
	vector.StrokeCircle(screen, f32(sx), f32(sy), f32(14*s), f32(3*s), colReach, true)
}

// linkBubble shows what releasing the drag here would do, next to the finger.
func (g *Game) linkBubble(u *ui, minX, maxX float64) {
	d := g.linkDrag
	if d == nil {
		return
	}
	p := g.planet
	title, sub := "Drag to a building", fmt.Sprintf("Links reach %d tiles", sim.MaxLinkLength)
	subCol, border := color.Color(colMuted), color.Color(rgba(0x2B374A))
	if t, ok := g.linkTarget(); ok {
		title = "Link to " + p.BuildingAt(t).Spec().Name
		if err := p.CanLink(d.from, t); err != nil {
			sub, subCol, border = err.Error(), colRed, colRed
		} else {
			sub = fmt.Sprintf("%.0f tiles · %.1f metal", math.Ceil(p.LinkLength(d.from, t)), p.LinkCost(d.from, t))
			subCol, border = colPreview, colPreview
		}
	}
	w := math.Max(u.textW(title, sans, 13, semibold), u.textW(sub, mono, 12, semibold)) + 24
	h := 50.0
	x := math.Min(math.Max(d.x/u.s-w/2, minX), maxX-w)
	y := d.y/u.s - 64 - h
	if y < 0 {
		y = d.y/u.s + 40
	}
	u.panel(x, y, w, h, 12, colSheet, border)
	u.text(title, x+12, y+8, sans, 13, semibold, colText)
	u.text(sub, x+12, y+28, mono, 12, semibold, subCol)
}
