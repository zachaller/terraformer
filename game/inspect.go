package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/zachaller/terraformer/sim"
)

// inspectOpen reports whether the inspect sheet is showing.
func (g *Game) inspectOpen() bool {
	return g.tool == toolInspect && g.selected != nil && !g.menuOpen && !g.goalsOpen && g.planet.Landed()
}

// inspectSheet draws the inspect sheet along the bottom of the screen (from
// sheetLeft to sheetRight, ending at screenBottom) and returns its top.
func (g *Game) inspectSheet(u *ui, sheetLeft, sheetRight, screenBottom float64) float64 {
	_, _, _, sb := currentSafeArea()
	sb /= u.s
	pad := 16.0
	x, w := sheetLeft+pad, sheetRight-sheetLeft-2*pad

	was := u.measuring
	u.measuring = true
	h := g.inspectContent(u, x, 0, w)
	u.measuring = was

	top := screenBottom - sb - h - 30
	u.fillRound(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top+24, 24, colSheet)
	u.strokeRound(sheetLeft+0.5, top+0.5, sheetRight-sheetLeft-1, screenBottom-top+24, 24, 1, rgba(0x2B374A))
	u.area(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top, nil)
	u.fillRound(sheetLeft+(sheetRight-sheetLeft-40)/2, top+8, 40, 4, 2, rgba(0x3A4658))
	g.inspectContent(u, x, top+20, w)
	return top
}

// inspectContent lays out the sheet's contents from y and returns their height.
func (g *Game) inspectContent(u *ui, x, y, w float64) float64 {
	p := g.planet
	c := *g.selected
	t := p.Tile(c)
	b := p.BuildingAt(c)
	y0 := y

	// Header: picture, name, where, status, close.
	if b != nil {
		u.fillRound(x, y, 44, 44, 12, rgba(0x0E141E))
		g.buildingGlyph(u, b.Type, x+8, y+8, 28)
	} else {
		col := surfaceColor(t)
		u.fillRound(x, y, 44, 44, 12, color.RGBA{uint8(col[0]), uint8(col[1]), uint8(col[2]), 255})
	}
	name := t.Kind().String()
	sub := latitude(p.Grid.DirFromCell(c))
	if b != nil {
		name = b.Spec().Name
		sub = t.Kind().String() + " · " + sub
	}
	u.text(name, x+56, y+2, sans, 18, semibold, colText)
	u.text(sub, x+56, y+26, sans, 12, regular, colMuted)
	u.icon(iconCross, x+w-28, y+10, 24, colMuted)
	u.area(x+w-44, y, 44, 44, func() { g.selected = nil })
	if b != nil {
		g.statusPill(u, p.Diagnose(b), x+w-52, y+10)
	}
	y += 44 + 14

	if b != nil {
		y += g.diagnosis(u, b, x, y, w)
	}
	y += g.lifeChecklist(u, c, t, x, y, w)
	y += g.tileFacts(u, c, t, x, y, w)
	if b != nil {
		y += g.networkStats(u, b, x, y, w)
	}
	y += g.inspectActions(u, c, b, x, y, w)
	return y - y0
}

func latitude(d sim.Vec3) string {
	lat := math.Asin(d.Y) * 180 / math.Pi
	switch {
	case lat > 0.5:
		return fmt.Sprintf("%.0f°N", lat)
	case lat < -0.5:
		return fmt.Sprintf("%.0f°S", -lat)
	}
	return "Equator"
}

// statusPill draws a building's status, right-aligned to right.
func (g *Game) statusPill(u *ui, st sim.Status, right, y float64) {
	label, ic := "Running", iconCheck
	fill, fg := color.Color(colTealBg), color.Color(colTealTx)
	switch st.Kind {
	case sim.StatusLowPower:
		label, ic = fmt.Sprintf("%.0f%%", math.Min(st.Supply/st.Demand, 1)*100), iconBolt
		fill, fg = rgba(0x2E2414), rgba(0xF2C27A)
	case sim.StatusNoPower:
		label, ic = "No power", iconBolt
		fill, fg = colRedBg, colRed
	case sim.StatusNoInput:
		label, ic = "No "+lower(st.Resource.String()), iconWarning
		fill, fg = colRedBg, colRed
	case sim.StatusOutputFull:
		label, ic = "Full", iconWarning
		fill, fg = colRedBg, colRed
	case sim.StatusDepleted:
		label, ic = "Empty", iconWarning
		fill, fg = colCard, colIcon
	}
	w := 10 + 14 + 6 + u.textW(label, sans, 12, semibold) + 10
	x := right - w
	u.fillRound(x, y, w, 26, 13, fill)
	u.icon(ic, x+10, y+6, 14, fg)
	u.text(label, x+30, y+(26-u.lineH(sans, 12, semibold))/2, sans, 12, semibold, fg)
}

func lower(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]+'a'-'A') + s[1:]
}

// diagnosis explains what's holding a building back, with a one-tap fix.
func (g *Game) diagnosis(u *ui, b *sim.Building, x, y, w float64) float64 {
	p := g.planet
	st := p.Diagnose(b)
	c := p.Grid.CellAt(b.Tile)
	spec := b.Spec()
	linkIt := func() {
		g.setTool(toolLink)
		g.linkFrom = &c
		g.showMessage("Now tap the building to link it to", msgInfo)
	}
	var msg, fix string
	var action func()
	fill, border, fg := color.Color(colAmberBg), color.Color(colAmberLn), color.Color(colAmberTx)
	switch st.Kind {
	case sim.StatusRunning:
		switch {
		case spec.PowerOut > 0 && spec.Depot:
			msg = fmt.Sprintf("Makes %.0f power and holds the colony's stores.", spec.PowerOut)
		case spec.PowerOut > 0:
			msg = fmt.Sprintf("Makes %.0f power for its network.", spec.PowerOut)
		default:
			return 0
		}
		fill, border, fg = colCard, rgba(0x2B374A), colBody
	case sim.StatusLowPower:
		msg = fmt.Sprintf("Running at %.0f%% because its network is short of power: %.0f supplied, %.0f needed.",
			st.Supply/st.Demand*100, st.Supply, st.Demand)
		fix = fmt.Sprintf("Add a Solar panel · %.0f metal", sim.Specs[sim.SolarPanel].Cost)
		action = func() {
			g.setTool(toolSolar)
			g.showMessage("Tap a tile to place the solar panel, then link it into this network", msgInfo)
		}
	case sim.StatusNoPower:
		msg = "No power. Link it to the lander or a Solar panel."
		fix, action = "Link it", linkIt
	case sim.StatusNoInput:
		msg = fmt.Sprintf("Waiting for %s. Link it to the lander, which shares the colony's stores, or to a mine.", lower(st.Resource.String()))
		fix, action = "Link it", linkIt
	case sim.StatusOutputFull:
		msg = fmt.Sprintf("Its %s has nowhere to go. Link it to the lander so it reaches the stores.", lower(st.Resource.String()))
		fix, action = "Link it", linkIt
	case sim.StatusDepleted:
		msg = "This deposit is empty. Remove the mine and build a new one on ore (gold in the Industry view)."
		fix = fmt.Sprintf("Remove · +%.0f metal", spec.Cost/2)
		action = func() {
			if err := p.Demolish(c); err == nil {
				g.selected = nil
				g.atlasDirty = true
			}
		}
	}

	inner := w - 24
	measure := func() float64 {
		was := u.measuring
		u.measuring = true
		defer func() { u.measuring = was }()
		return g.diagnosisBody(u, msg, fix, nil, x+12, 0, inner, st)
	}
	h := measure() + 24
	u.panel(x, y, w, h, 14, fill, border)
	_ = fg
	g.diagnosisBody(u, msg, fix, action, x+12, y+12, inner, st)
	return h + 12
}

func (g *Game) diagnosisBody(u *ui, msg, fix string, action func(), x, y, w float64, st sim.Status) float64 {
	y0 := y
	fg := color.Color(colAmberTx)
	if st.Kind == sim.StatusRunning {
		fg = colBody
	}
	y += u.paragraph(msg, x, y, w, 13, fg)
	if st.Kind == sim.StatusLowPower {
		y += 8
		frac := math.Min(st.Supply/st.Demand, 1)
		u.fillRound(x, y, w, 8, 4, rgba(0x4A3A22))
		u.fillRound(x, y, w*frac, 8, 4, colAmber)
		y += 8
	}
	if fix != "" {
		y += 10
		bw := u.textW(fix, sans, 13, bold) + 28
		u.fillRound(x, y, bw, 40, 12, colAmber)
		u.text(fix, x+14, y+(40-u.lineH(sans, 13, bold))/2, sans, 13, bold, colOnAmber)
		u.area(x, y, bw, 40, action)
		y += 40
	}
	return y - y0
}

// lifeChecklist shows whether microbes can live on the tile, and why not.
func (g *Game) lifeChecklist(u *ui, c sim.Cell, t sim.Tile, x, y, w float64) float64 {
	y0 := y
	u.text("CAN MICROBES LIVE HERE?", x, y, sans, 11, semibold, colMuted)
	y += u.lineH(sans, 11, semibold) + 4

	warm := t.Temp > sim.MinLifeTemp && t.Temp < sim.MaxLifeTemp
	damp := t.Water > sim.MinLifeWater
	clean := t.Pollution < sim.MaxLifePollution
	warmReq := "(above −5°)"
	if t.Temp >= sim.MaxLifeTemp {
		warmReq = "(below 60°)"
	}
	rows := []struct {
		ok         bool
		label, val string
		req        string
	}{
		{warm, "Warm enough", degrees(t.Temp), warmReq},
		{damp, "Damp ground", fmt.Sprintf("%.2f", t.Water), "(needs moisture)"},
		{clean, "Clean enough", fmt.Sprintf("%.2f", t.Pollution), "(max 0.25)"},
	}
	for i, r := range rows {
		rh := 34.0
		ic, col := iconCheck, color.Color(colTeal)
		valCol := color.Color(colIcon)
		if !r.ok {
			ic, col, valCol = iconCross, colRed, colRed
		}
		u.icon(ic, x, y+(rh-18)/2, 18, col)
		u.text(r.label, x+28, y+(rh-u.lineH(sans, 13, regular))/2, sans, 13, regular, colText)
		reqW := u.textW(" "+r.req, sans, 12, regular)
		u.textRight(r.req, x+w, y+(rh-u.lineH(sans, 12, regular))/2, sans, 12, regular, colMuted)
		u.textRight(r.val, x+w-reqW, y+(rh-u.lineH(mono, 13, semibold))/2, mono, 13, semibold, valCol)
		if i < len(rows)-1 {
			u.fillRound(x, y+rh-1, w, 1, 0, rgba(0x1F2836))
		}
		y += rh
	}

	var summary string
	switch {
	case warm && damp && clean && t.Life > 0:
		summary = fmt.Sprintf("Microbes cover %.0f%% of this tile.", t.Life*100)
	case warm && damp && clean:
		summary = "Conditions are right. Microbes should appear soon."
	case !warm && t.Temp >= sim.MaxLifeTemp:
		summary = "Too hot. Move heaters further apart."
	case !warm:
		summary = "Too cold. A Heater on or next to this tile would warm it."
	case !clean:
		if pb, d := g.planet.NearestPolluter(c, 4); pb != nil {
			summary = fmt.Sprintf("The %s %.0f tiles away is polluting this tile.", lower(pb.Spec().Name), math.Max(d, 1))
		} else {
			summary = "Still polluted. It clears slowly once industry moves away."
		}
	case !damp:
		summary = "Too dry. Melt ice here or uphill, so water reaches it."
	}
	y += 4
	y += u.paragraph(summary, x, y, w, 12, colBody)
	return y - y0 + 12
}

// tileFacts shows the tile's other numbers as small chips.
func (g *Game) tileFacts(u *ui, c sim.Cell, t sim.Tile, x, y, w float64) float64 {
	chips := []chip{
		{parts: []chipPart{iconPart(iconDrop, colBlue), labelPart("Ice"), numPart(fmt.Sprintf("%.2f", t.Ice), colText)}, fill: colCard},
	}
	if t.Ore > 0 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconMetal, rgba(0xF2C04A)), labelPart("Ore"), numPart(fmt.Sprintf("%.0f", t.Ore), colText)}, fill: colCard})
	}
	if h := g.planet.TotalHeat(g.planet.Grid.Index(c)); h > 0.5 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconThermo, colAmber), labelPart("Heated"), numPart(fmt.Sprintf("+%.0f°", h), colAmber)}, fill: colCard})
	}
	bottom := u.flowChips(chips, x, x+w, y)
	return bottom - y + 14
}

// networkStats summarises the building's network.
func (g *Game) networkStats(u *ui, b *sim.Building, x, y, w float64) float64 {
	n := g.planet.NetworkOf(b)
	y0 := y
	u.text("NETWORK", x, y, sans, 11, semibold, colMuted)
	y += u.lineH(sans, 11, semibold) + 6
	reach, reachCol := "No", color.Color(colRed)
	if n.HasDepot {
		reach, reachCol = "Yes", colTeal
	}
	spare := n.PowerSupply - n.PowerDemand
	spareCol := color.Color(colTeal)
	if spare < 0 {
		spareCol = colAmber
	}
	boxes := []struct {
		val, label string
		col        color.Color
	}{
		{fmt.Sprint(len(n.Members)), "buildings", colText},
		{reach, "reaches lander", reachCol},
		{signed(spare), "power spare", spareCol},
	}
	bw := (w - 2*gap) / 3
	for i, bx := range boxes {
		xx := x + float64(i)*(bw+gap)
		u.fillRound(xx, y, bw, 52, 12, colCard)
		u.text(bx.val, xx+10, y+8, mono, 16, semibold, bx.col)
		u.text(bx.label, xx+10, y+30, sans, 11, regular, colMuted)
	}
	return y + 52 - y0 + 14
}

func signed(v float64) string {
	switch {
	case v > 0.05:
		return fmt.Sprintf("+%.0f", v)
	case v < -0.05:
		return fmt.Sprintf("−%.0f", -v)
	}
	return "0"
}

// inspectActions is the sheet's row of buttons.
func (g *Game) inspectActions(u *ui, c sim.Cell, b *sim.Building, x, y, w float64) float64 {
	p := g.planet
	type act struct {
		icon   iconName
		label  string
		danger bool
		action func()
	}
	var acts []act
	if b == nil {
		acts = append(acts, act{iconBuild, "Build here", false, func() { g.menuOpen = true }})
	} else {
		acts = append(acts, act{iconLink, "Link", false, func() {
			g.setTool(toolLink)
			g.linkFrom = &c
			g.showMessage("Now tap the building to link it to", msgInfo)
		}})
		if b.Type != sim.Lander {
			refund := b.Spec().Cost / 2
			acts = append(acts, act{iconRemove, fmt.Sprintf("Remove · +%.0f", refund), true, func() {
				if err := p.Demolish(c); err != nil {
					g.showMessage(err.Error(), msgWarn)
					return
				}
				g.selected = nil
				g.atlasDirty = true
				g.showMessage(fmt.Sprintf("Removed. %.0f metal refunded", refund), msgInfo)
			}})
		}
	}
	bw := (w - gap*float64(len(acts)-1)) / float64(len(acts))
	for i, a := range acts {
		xx := x + float64(i)*(bw+gap)
		fg := color.Color(colText)
		if a.danger {
			fg = rgba(0xFFB4A8)
		}
		u.panel(xx, y, bw, 48, 14, colCard, rgba(0x2B374A))
		lw := u.textW(a.label, sans, 13, semibold)
		cx := xx + (bw-lw-24)/2
		u.icon(a.icon, cx, y+15, 18, fg)
		u.text(a.label, cx+24, y+(48-u.lineH(sans, 13, semibold))/2, sans, 13, semibold, fg)
		u.area(xx, y, bw, 48, a.action)
	}
	return 48
}
