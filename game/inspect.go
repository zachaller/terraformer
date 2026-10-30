package game

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/zachaller/terraformer/sim"
)

// inspectOpen reports whether the inspect sheet is showing.
func (g *Game) inspectOpen() bool {
	return g.tool == toolInspect && g.selected != nil && !g.menuOpen && !g.goalsOpen && !g.airOpen && !g.colonyOpen && g.planet.Landed()
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
	sub := latitude(p.Dir(c))
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
	// Life can't grow under a building, so only bare ground explains it.
	if b == nil {
		if next, ok := sim.NextSpecies(t); t.Life < 0.5 && !(ok && sim.SpeciesSpecs[next].Aquatic) {
			y += g.lifeChecklist(u, c, t, x, y, w)
		} else {
			y += g.speciesSection(u, c, t, x, y, w)
		}
	}
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
	case sim.StatusNoWind:
		label, ic = "No wind", iconAir
		fill, fg = colCard, colIcon
	case sim.StatusIncoming:
		label, ic = "Incoming", iconWarning
		fill, fg = rgba(0x2E2414), rgba(0xF2C27A)
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
	addPylon := func() {
		g.setTool(buildTool(sim.Pylon))
		g.showMessage("Tap a tile within 4 tiles of it to place a pylon", msgInfo)
	}
	pylonFix := fmt.Sprintf("Add a pylon · %.0f metal", sim.Specs[sim.Pylon].Cost)
	var msg, fix string
	var action func()
	fill, border, fg := color.Color(colAmberBg), color.Color(colAmberLn), color.Color(colAmberTx)
	switch st.Kind {
	case sim.StatusRunning:
		// Nothing to fix: just say what it does, as plain text.
		msg = runningMessage(p, b)
		if msg == "" {
			return 0
		}
		return u.paragraph(msg, x, y, w, 13, colBody) + 12
	case sim.StatusLowPower:
		msg = fmt.Sprintf("Running at %.0f%% because its network is short of power: %.0f supplied, %.0f needed.",
			st.Supply/st.Demand*100, st.Supply, st.Demand)
		fix = fmt.Sprintf("Add a Solar panel · %.0f metal", sim.Specs[sim.SolarPanel].Cost)
		action = func() {
			g.setTool(buildTool(sim.SolarPanel))
			g.showMessage("Tap a tile within 4 tiles of a hub in this network to place the solar panel", msgInfo)
		}
	case sim.StatusNoPower:
		msg = "No power: nothing connects it. Build a Pylon within 4 tiles of it, close enough to the colony's other hubs."
		if p.HubsFull(b) {
			msg = "No power: the pylons near it are full. Each building takes load by its distance from the pylon, so add another pylon closer to it."
		}
		fix, action = pylonFix, addPylon
	case sim.StatusNoInput:
		msg = fmt.Sprintf("Waiting for %s. Connect it to the colony, whose stores supply it: within 4 tiles of the lander or a connected pylon.", lower(st.Resource.String()))
		fix, action = pylonFix, addPylon
	case sim.StatusOutputFull:
		msg = fmt.Sprintf("Its %s has nowhere to go. Connect it to the colony so it reaches the stores.", lower(st.Resource.String()))
		fix, action = pylonFix, addPylon
	case sim.StatusNoWind:
		msg = fmt.Sprintf("The air is too thin to turn it (%s kPa). Wind turbines start working above 2 kPa and reach full power at 40.", pressure(p.Pressure()))
	case sim.StatusIncoming:
		days := (spec.Countdown - b.Timer) / 10
		msg = fmt.Sprintf("A comet is on its way: impact in %d days. It brings water, nitrogen and CO2, and destroys anything within two tiles.", days)
		fill, border, fg = colCard, rgba(0x2B374A), colBody
	case sim.StatusDepleted:
		msg = "This deposit is empty. Remove the mine and build a new one on another deposit (Industry view)."
		if spec.Site == sim.IceSite {
			msg = "The ice here is gone. Remove the drill and build it on another icy tile."
		}
		fix = fmt.Sprintf("Remove · +%.0f metal", spec.Cost/2)
		action = func() {
			if err := p.Demolish(c); err == nil {
				g.selected = nil
				g.atlasDirty = true
			}
		}
		if spec.Site == sim.DepositSite {
			msg = "This patch of ore is dug out. Move the mine to the nearest free deposit, still connected if one is in reach."
			fix = fmt.Sprintf("Move to a new deposit · %.0f metal", spec.Cost-spec.Cost/2)
			action = func() { g.moveMine(c) }
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
	if st.Kind == sim.StatusRunning || st.Kind == sim.StatusIncoming {
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
	u.text("MICROBES", x, y, sans, 11, semibold, colMuted)
	y += u.lineH(sans, 11, semibold) + 6

	warm := t.Temp > sim.MinLifeTemp && t.Temp < sim.MaxLifeTemp
	damp := t.Water > sim.MinLifeWater
	clean := t.Pollution < sim.MaxLifePollution
	y = u.flowChips([]chip{
		checkChip(warm, "Warm", degrees(t.Temp)),
		checkChip(damp, "Damp", fmt.Sprintf("%.2f", t.Water)),
		checkChip(clean, "Clean", fmt.Sprintf("%.2f", t.Pollution)),
	}, x, x+w, y) + 6

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
	case !damp && g.planet.Pressure() < sim.TriplePoint:
		summary = fmt.Sprintf("Too dry: the air (%s kPa) is too thin for liquid water, so ice turns straight to vapour. Thicken it past %.2f kPa.", pressure(g.planet.Pressure()), sim.TriplePoint)
	case !damp:
		summary = "Too dry. Melt ice here or uphill, so water reaches it."
	}
	y += u.paragraph(summary, x, y, w, 12, colBody)
	return y - y0 + 12
}

// checkChip is a small requirement: a tick or cross, a label and a value.
func checkChip(ok bool, label, val string) chip {
	ic, col := iconCheck, color.Color(colTeal)
	valCol := color.Color(colText)
	if !ok {
		ic, col, valCol = iconCross, colRed, colRed
	}
	return chip{parts: []chipPart{iconPart(ic, col), labelPart(label), numPart(val, valCol)}, fill: colCard}
}

// runningMessage describes what a building that's running normally does.
func runningMessage(p *sim.Planet, b *sim.Building) string {
	spec := b.Spec()
	switch b.Type {
	case sim.Lander:
		load, capacity := p.HubLoad(b)
		return fmt.Sprintf("Makes %.0f power and holds the colony's stores. Connects buildings within %.0f tiles (load %.0f of %.0f) and pylons within %.0f.", spec.PowerOut, spec.CoverRadius, load, capacity, spec.Reach)
	case sim.Pylon:
		load, capacity := p.HubLoad(b)
		return fmt.Sprintf("Load %.0f of %.0f. Connects buildings within %.0f tiles (each takes load by its distance: 1 next door, 4 at the edge), and joins pylons and the lander within %.0f tiles.", load, capacity, spec.CoverRadius, spec.Reach)
	case sim.SolarPanel, sim.Geothermal:
		return fmt.Sprintf("Makes %.0f power for its network.", spec.PowerOut)
	case sim.WindTurbine:
		return fmt.Sprintf("Makes %.1f of its %.0f power in %s kPa of air.", spec.PowerOut*p.WindFactor(), spec.PowerOut, pressure(p.Pressure()))
	case sim.IceDrill:
		return "Melts ice into water for the colony's stores."
	case sim.Electrolyser:
		return fmt.Sprintf("Splits water into oxygen: +%.2f kPa a year.", spec.Emits.O2*3650)
	case sim.GreenhouseFactory:
		return fmt.Sprintf("Turns fluorite into super-greenhouse gas: +%.1f Pa a year. Each Pa warms the planet by several degrees.", spec.Emits.PFC*3650)
	case sim.NitrogenPlant:
		return fmt.Sprintf("Turns nitrate into nitrogen: +%.2f kPa a year.", spec.Emits.N2*3650)
	case sim.DustSpreader:
		return fmt.Sprintf("Dusts the ice up to %d tiles around, so it absorbs sunlight and melts.", spec.DustRings)
	case sim.OrbitalMirror:
		return fmt.Sprintf("Focuses extra sunlight here: +%.0f° on this tile, fading over %d tiles.", spec.Heat, spec.HeatRings)
	case sim.MagneticShield:
		return "Deflects the solar wind, so the air no longer escapes to space."
	case sim.Biolab:
		return fmt.Sprintf("Seeds life up to %d tiles around: it takes hold far faster here.", spec.SeedRings)
	case sim.CarbonScrubber:
		return fmt.Sprintf("Captures CO2 and locks it away: −%.2f kPa a year.", -spec.Emits.CO2*3650)
	}
	return ""
}

// tileFacts shows the tile's other numbers as small chips.
func (g *Game) tileFacts(u *ui, c sim.Cell, t sim.Tile, x, y, w float64) float64 {
	chips := []chip{
		{parts: []chipPart{iconPart(iconDrop, colBlue), labelPart("Ice"), numPart(fmt.Sprintf("%.2f", t.Ice), colText)}, fill: colCard},
	}
	if t.Water > 0.005 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconDrop, colBlue), labelPart("Water"), numPart(fmt.Sprintf("%.2f", t.Water), colText)}, fill: colCard})
	}
	if t.DryIce > 0.005 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconAir, colIcon), labelPart("Dry ice"), numPart(fmt.Sprintf("%.2f", t.DryIce), colText)}, fill: colCard})
	}
	if t.Dust > 0.01 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconGlobe, rgba(0xB08A6A)), labelPart("Dust"), numPart(fmt.Sprintf("%.0f%%", t.Dust*100), colText)}, fill: colCard})
	}
	if t.Soil > 0.01 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconLeaf, rgba(0x9C7A4E)), labelPart("Soil"), numPart(fmt.Sprintf("%.0f%%", t.Soil*100), colText)}, fill: colCard})
	}
	if b := g.planet.BuildingAt(c); b != nil && b.Type == sim.Mine {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconMetal, mineralColor(t.Mineral)), labelPart(t.Mineral.String() + " in patch"), numPart(fmt.Sprintf("%.0f", g.planet.PatchOre(c)), colText)}, fill: colCard})
	} else if t.Ore > 0 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconMetal, mineralColor(t.Mineral)), labelPart(t.Mineral.String()), numPart(fmt.Sprintf("%.0f", t.Ore), colText)}, fill: colCard})
	}
	if t.Volcanic {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconThermo, rgba(0xE2553A)), labelPart("Volcanic")}, fill: colCard})
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
	reach, reachCol := "isolated", color.Color(colRed)
	if n.HasDepot {
		reach, reachCol = "colony", colTeal
	}
	spare := n.PowerSupply - n.PowerDemand
	spareCol := color.Color(colTeal)
	if spare < 0 {
		spareCol = colAmber
	}
	members := fmt.Sprintf("%d %s", len(n.Members), plural(len(n.Members), "building", "buildings"))
	chips := []chip{
		{parts: []chipPart{iconPart(iconLink, colIcon), numPart(members, colText)}, fill: colCard},
		{parts: []chipPart{numPart(reach, reachCol)}, fill: colCard},
		{parts: []chipPart{iconPart(iconBolt, colAmber), numPart(signed(spare), spareCol), labelPart("spare")}, fill: colCard},
	}
	bottom := u.flowChips(chips, x, x+w, y)
	return bottom - y + 14
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
	if len(acts) == 0 {
		return 0
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

// lifeState is what lifeNote says about a tile: which animals can be seen
// there (as the planet shader draws them) and how much oxygen its life makes
// (0 none, 1 a little, 2 some, 3 a lot).
type lifeState struct {
	herd, school, birds bool
	oxygen              int
	soil                bool
}

// readLife works out tile t's lifeState. Given the state last shown (prev,
// if any), each part only changes once its value is clearly past the
// threshold, so the note doesn't flicker as densities wobble around one.
func readLife(t sim.Tile, prev *lifeState) lifeState {
	var p lifeState
	if prev != nil {
		p = *prev
	}
	// on is a reading that turns on above hi and off below lo.
	on := func(was bool, v, lo, hi float64) bool {
		if was && prev != nil {
			return v > lo
		}
		return v > hi
	}
	s := lifeState{
		herd:   on(p.herd, t.Bio[sim.Grazers], 0.02, 0.03),
		school: on(p.school, t.Bio[sim.Fish], 0.02, 0.03),
		birds:  on(p.birds, t.Bio[sim.Forest], 0.35, 0.4),
		soil:   sim.BuildsSoil(t),
	}
	o := sim.Photosynthesis(t)
	levels := []float64{0.02, 0.4, 1.5} // where a little, some and a lot start
	s.oxygen = p.oxygen
	if prev == nil {
		s.oxygen = 0
		for _, l := range levels {
			if o > l {
				s.oxygen++
			}
		}
	} else {
		for s.oxygen < len(levels) && o > levels[s.oxygen]*1.1 {
			s.oxygen++
		}
		for s.oxygen > 0 && o < levels[s.oxygen-1]*0.9 {
			s.oxygen--
		}
	}
	return s
}

// lifeNote says what can be seen moving on a tile and what its life does
// for the planet.
func lifeNote(s lifeState) string {
	var seen []string
	if s.herd {
		seen = append(seen, "a herd of grazers wanders here")
	}
	if s.school {
		seen = append(seen, "a school of fish swims here")
	}
	if s.birds {
		seen = append(seen, "birds circle over the forest")
	}
	var does []string
	if s.oxygen > 0 {
		does = append(does, "makes "+[]string{"a little", "some", "a lot of"}[s.oxygen-1]+" oxygen from CO2")
	}
	if s.soil {
		does = append(does, "builds soil")
	}
	var out []string
	if len(seen) > 0 {
		out = append(out, capitalize(strings.Join(seen, ", and "))+".")
	}
	if len(does) > 0 {
		out = append(out, "Life here "+strings.Join(does, " and ")+".")
	}
	return strings.Join(out, " ")
}

// lifeNoteAt is lifeNote for tile c, keeping what was last shown for the
// same tile so its parts only change once clearly past their thresholds.
func (g *Game) lifeNoteAt(c sim.Cell, t sim.Tile) string {
	var prev *lifeState
	if g.noteValid && g.noteCell == c {
		prev = &g.noteState
	}
	g.noteState, g.noteCell, g.noteValid = readLife(t, prev), c, true
	return lifeNote(g.noteState)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// speciesSection shows what lives on the tile and what the next stage of life
// needs, once microbes are established (or the tile is under water).
func (g *Game) speciesSection(u *ui, c sim.Cell, t sim.Tile, x, y, w float64) float64 {
	y0 := y
	u.text("LIVING HERE", x, y, sans, 11, semibold, colMuted)
	y += u.lineH(sans, 11, semibold) + 6
	var chips []chip
	if t.Life > 0.01 {
		chips = append(chips, chip{parts: []chipPart{iconPart(iconLeaf, colTeal), labelPart("Microbes"), numPart(fmt.Sprintf("%.0f%%", t.Life*100), colText)}, fill: colCard})
	}
	for s := range sim.NumSpecies {
		if t.Bio[s] > 0.01 {
			col := speciesColors[s]
			chips = append(chips, chip{parts: []chipPart{iconPart(iconLeaf, color.RGBA{uint8(col[0]), uint8(col[1]), uint8(col[2]), 255}), labelPart(s.String()), numPart(fmt.Sprintf("%.0f%%", t.Bio[s]*100), colText)}, fill: colCard})
		}
	}
	if len(chips) == 0 {
		y += u.paragraph("Nothing yet.", x, y, w, 12, colBody)
	} else {
		y = u.flowChips(chips, x, x+w, y) + 6
		if note := g.lifeNoteAt(c, t); note != "" {
			y += u.paragraph(note, x, y, w, 12, colBody) + 4
		}
	}
	y += 8

	next, ok := sim.NextSpecies(t)
	if !ok {
		y += u.paragraph("Every stage of life thrives here.", x, y, w, 12, colBody)
		return y - y0 + 12
	}
	u.text("NEXT: "+strings.ToUpper(next.String()), x, y, sans, 11, semibold, colMuted)
	y += u.lineH(sans, 11, semibold) + 6
	// What's missing, in full; what's met, as a count.
	var need []chip
	met := 0
	for _, r := range g.planet.Requirements(next, t) {
		if r.OK {
			met++
			continue
		}
		need = append(need, checkChip(false, r.Label, r.Have+" / "+r.Need))
	}
	if met > 0 {
		need = append(need, chip{parts: []chipPart{iconPart(iconCheck, colTeal), numPart(fmt.Sprintf("%d met", met), colTealTx)}, fill: colCard})
	}
	y = u.flowChips(need, x, x+w, y)
	return y - y0 + 12
}
