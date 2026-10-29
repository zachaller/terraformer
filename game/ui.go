package game

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/zachaller/terraformer/sim"
)

// tool is what tapping the planet does.
type tool int

const (
	toolLand tool = iota
	toolInspect
	toolLink
	toolDemolish
	// toolBuild + a building type builds that type.
	toolBuild
)

// buildTool is the tool that places building type bt.
func buildTool(bt sim.BuildingType) tool { return toolBuild + tool(bt) }

// building returns the building type a tool places, if it places one.
func (t tool) building() (sim.BuildingType, bool) {
	if t <= toolBuild { // toolBuild itself would be the lander
		return 0, false
	}
	return sim.BuildingType(t - toolBuild), true
}

func (t tool) String() string {
	if bt, ok := t.building(); ok {
		return bt.String()
	}
	switch t {
	case toolLand:
		return "Land"
	case toolLink:
		return "Link"
	case toolDemolish:
		return "Remove"
	}
	return "Inspect"
}

var speeds = []struct {
	label string
	steps float64 // simulation steps per second
}{
	{"Paused", 0},
	{"1×", 10},
	{"4×", 40},
	{"16×", 160},
}

// buildInfo is the build menu's description of each building.
var buildInfo = map[sim.BuildingType]struct{ effect, tag string }{
	sim.SolarPanel:        {"Makes 2 power", "Clean"},
	sim.Pylon:             {"Connects all within 4 tiles; links reach 12", "No power needed"},
	sim.WindTurbine:       {"Up to 6 power once the air is thick", "Clean"},
	sim.Geothermal:        {"10 power, and warms its tile", "Volcanic tiles only"},
	sim.Mine:              {"Digs iron, fluorite or nitrate", "1 power · pollutes"},
	sim.Smelter:           {"Turns 2 ore into 1 metal", "2 power · pollutes"},
	sim.IceDrill:          {"Mines ice for water", "2 power · on ice"},
	sim.HeaterMachine:     {"About +30° here, spreading out", "3 power · clean"},
	sim.DustSpreader:      {"Darkens ice 3 tiles around", "2 power · clean"},
	sim.GreenhouseFactory: {"Fluorite into super-greenhouse gas", "4 power · pollutes"},
	sim.OrbitalMirror:     {"+45° of sunlight over a wide area", "No power needed"},
	sim.Electrolyser:      {"Splits water into oxygen", "4 power · clean"},
	sim.NitrogenPlant:     {"Turns nitrate into nitrogen gas", "3 power · clean"},
	sim.MagneticShield:    {"Stops the air escaping to space", "20 power · one only"},
	sim.Biolab:            {"Helps life take hold nearby", "3 power · clean"},
	sim.CarbonScrubber:    {"Captures CO2 from the air", "6 power · clean"},
	sim.Comet:             {"Strikes in 30 days: water, nitrogen", "Far from buildings"},
}

// setTool switches tool, closing the menu and cancelling a half-made link.
func (g *Game) setTool(t tool) {
	g.tool = t
	g.menuOpen = false
	g.linkFrom = nil
	g.pending = nil
	g.linkDrag = nil
	g.goalsOpen = false
	g.airOpen = false
}

// Layout sizes in dp.
const (
	margin  = 12
	chipH   = 30
	dockH   = 64
	railBtn = 48
	gap     = 8
)

// msgKind styles a message.
type msgKind int

const (
	msgInfo msgKind = iota
	msgWarn
	msgGood
)

// runUI lays out (and, depending on u's mode, draws or hit-tests) the whole
// interface, and records the free space left for the planet.
func (g *Game) runUI(u *ui) {
	s := u.s
	W, H := float64(g.w)/s, float64(g.h)/s
	sl, st, sr, sb := currentSafeArea()
	sl, st, sr, sb = sl/s, st/s, sr/s, sb/s
	left, right := sl+margin, W-sr-margin
	top, bottom := st+10, H-sb-12

	// On wide screens, keep cards and the dock to a phone-like width.
	narrow := math.Min(right, left+480)
	y := g.statusBar(u, left, right, top)
	_, placing := g.placing()
	if (placing || g.tool == toolLink) && !g.menuOpen {
		y = g.modeBanner(u, left, narrow, y+gap)
	} else {
		y = g.goalCard(u, left, narrow, y+gap)
		if !g.inspectOpen() && !g.goalsOpen && !g.menuOpen && !g.airOpen {
			y = g.alertRows(u, left, narrow, y)
		}
	}
	planetTop := y + gap
	planetBottom := bottom
	planetLeft, planetRight := sl, W-sr
	sheetRight := math.Min(W-sr, sl+520)
	// On wide screens, sheets sit at the left with the planet beside them.
	sideBySide := W-sr-sheetRight > 320
	beside := func(sheetTop float64) {
		if sideBySide {
			planetLeft, planetTop, planetBottom = sheetRight, st+10, bottom
		} else {
			planetBottom = sheetTop - gap
		}
	}

	switch {
	case !g.planet.Landed():
		h := 56.0
		u.panel(left, bottom-h, narrow-left, h, 16, colChip, colBorder)
		msg := "Tap the planet to choose your landing site"
		u.text(msg, left+(narrow-left-u.textW(msg, sans, 14, semibold))/2, bottom-h/2-u.lineH(sans, 14, semibold)/2, sans, 14, semibold, colText)
		planetBottom = bottom - h - gap
		g.toast(u, left, narrow, planetBottom)
	case g.goalsOpen:
		beside(g.goalsSheet(u, sl, sheetRight, H))
	case g.airOpen:
		beside(g.airSheet(u, sl, sheetRight, H))
	case g.inspectOpen():
		beside(g.inspectSheet(u, sl, sheetRight, H))
	case placing && g.pending != nil:
		planetBottom = g.placementPanel(u, left, narrow, bottom) - gap
		g.toast(u, left, narrow, planetBottom)
	default:
		dockTop := bottom - dockH
		g.dock(u, left, narrow, dockTop)
		above := dockTop - gap
		if g.menuOpen {
			above = g.buildMenu(u, left, narrow, above) - gap
			planetBottom = above
			break
		}
		speedTop := above - 44
		g.speedControl(u, left, speedTop)
		planetBottom = speedTop - gap
		if g.msgAction != nil && g.ticks < g.msgUntil {
			// Messages with a button get a full-width row of their own.
			planetBottom = g.toast(u, left, narrow, planetBottom) - gap
		} else {
			g.toast(u, left+float64(len(speeds))*44+16, narrow, above)
		}
		g.viewRail(u, right, planetTop, planetBottom)
	}
	g.linkBubble(u, left, right)
	if !u.measuring {
		g.planetTarget = [4]float64{planetLeft * s, planetTop * s, (planetRight - planetLeft) * s, math.Max(planetBottom-planetTop, 80) * s}
	}
}

// chip is a pill of icon and text parts.
type chip struct {
	parts        []chipPart
	fill, border color.Color
	action       func() // when tapped, if set
}

type chipPart struct {
	icon    iconName
	hasIcon bool
	text    string
	kind    fontKind
	size    float64
	weight  int
	col     color.Color
}

func iconPart(i iconName, c color.Color) chipPart { return chipPart{icon: i, hasIcon: true, col: c} }
func numPart(t string, c color.Color) chipPart {
	return chipPart{text: t, kind: mono, size: 13, weight: semibold, col: c}
}
func labelPart(t string) chipPart {
	return chipPart{text: t, kind: sans, size: 11, weight: regular, col: colMuted}
}

func (u *ui) chipWidth(c chip) float64 {
	w := 20.0
	for i, p := range c.parts {
		if i > 0 {
			w += 6
		}
		if p.hasIcon {
			w += 14
		} else {
			w += u.textW(p.text, p.kind, p.size, p.weight)
		}
	}
	return w
}

func (u *ui) drawChip(c chip, x, y float64) {
	u.panel(x, y, u.chipWidth(c), chipH, chipH/2, c.fill, c.border)
	cx := x + 10
	for i, p := range c.parts {
		if i > 0 {
			cx += 6
		}
		if p.hasIcon {
			u.icon(p.icon, cx, y+(chipH-14)/2, 14, p.col)
			cx += 14
			continue
		}
		cx += u.text(p.text, cx, y+(chipH-u.lineH(p.kind, p.size, p.weight))/2, p.kind, p.size, p.weight, p.col)
	}
}

// flowChips lays chips out left to right, wrapping, and returns the bottom.
func (u *ui) flowChips(chips []chip, left, right, y float64) float64 {
	x := left
	for _, c := range chips {
		w := u.chipWidth(c)
		if x > left && x+w > right {
			x = left
			y += chipH + 6
		}
		u.drawChip(c, x, y)
		if c.action != nil {
			u.area(x, y, w, chipH, c.action)
		}
		x += w + 6
	}
	return y + chipH
}

func (g *Game) statusBar(u *ui, left, right, y float64) float64 {
	p := g.planet
	st := p.Stats()
	chips := []chip{
		{parts: []chipPart{labelPart("Day"), numPart(fmt.Sprint(p.Ticks/10), colText)}, fill: colChip, border: colBorder},
		{parts: []chipPart{iconPart(iconAir, colBlue), numPart(pressure(p.Pressure()), colText), labelPart("kPa")}, fill: colChip, border: colBorder, action: g.openAir},
		{parts: []chipPart{iconPart(iconThermo, colBlue), numPart(degrees(st.AvgTemp), colText)}, fill: colChip, border: colBorder},
		{parts: []chipPart{iconPart(iconLeaf, colTeal), numPart(fmt.Sprintf("%.1f%%", st.LifeCover*100), colTeal)}, fill: colChip, border: colBorder},
	}
	y = u.flowChips(chips, left, right, y)
	if !p.Landed() {
		return y
	}
	metal := chip{
		parts:  []chipPart{iconPart(iconMetal, colIcon), numPart(fmt.Sprintf("%.1f", p.Stores()[sim.Metal]), colText), labelPart("metal")},
		fill:   colChip,
		border: colBorder,
	}
	if rate := g.metalRate(); rate > 0.005 {
		metal.parts = append(metal.parts, chipPart{text: fmt.Sprintf("+%.1f/day", rate), kind: mono, size: 11, weight: semibold, col: colTeal})
	}
	chips = []chip{metal}
	if lander := p.BuildingAt(p.Grid.CellAt(p.Buildings()[0].Tile)); lander != nil {
		n := p.NetworkOf(lander)
		pc := chip{
			parts:  []chipPart{iconPart(iconBolt, colAmber), numPart(fmt.Sprintf("%.0f / %.0f", n.PowerSupply, n.PowerDemand), colText), labelPart("power")},
			fill:   colChip,
			border: colBorder,
		}
		if n.PowerSatisfaction() < 1 {
			pc.parts[1].col = colAmber
			pc.fill, pc.border = colAmberBg, colAmberLn
		}
		chips = append(chips, pc)
	}
	for _, r := range []struct {
		res sim.Resource
		col color.Color
	}{{sim.Water, colBlue}, {sim.FluoriteRes, mineralColor(sim.Fluorite)}, {sim.NitrateRes, mineralColor(sim.Nitrate)}} {
		if v := p.Stores()[r.res]; v >= 0.05 {
			chips = append(chips, chip{parts: []chipPart{iconPart(iconMetal, r.col), numPart(fmt.Sprintf("%.0f", v), colText), labelPart(lower(r.res.String()))}, fill: colChip, border: colBorder})
		}
	}
	return u.flowChips(chips, left, right, y+6)
}

// pressure formats kPa with enough digits to see small changes.
func pressure(kPa float64) string {
	switch {
	case kPa < 10:
		return fmt.Sprintf("%.2f", kPa)
	case kPa < 100:
		return fmt.Sprintf("%.1f", kPa)
	}
	return fmt.Sprintf("%.0f", kPa)
}

func degrees(c float64) string {
	if c < 0 {
		return fmt.Sprintf("−%.1f°", -c)
	}
	return fmt.Sprintf("%.1f°", c)
}

func (g *Game) goalCard(u *ui, left, right, y float64) float64 {
	p := g.planet
	goal, prog, ok := p.CurrentGoal()
	w := right - left
	n := p.GoalsDone() + 1
	toggle := func() { g.goalCollapsed = !g.goalCollapsed }

	if g.goalCollapsed || g.inspectOpen() || g.menuOpen || g.goalsOpen || g.airOpen {
		h := 44.0
		u.panel(left, y, w, h, 14, colChip, colBorder)
		label := "All goals complete"
		if ok {
			label = fmt.Sprintf("Goal %d/%d · %s", n, len(sim.Goals), goal.Name)
		}
		u.text(label, left+14, y+(h-u.lineH(sans, 14, semibold))/2, sans, 14, semibold, colText)
		if ok && prog > 0 {
			u.textRight(fmt.Sprintf("%.0f%%", prog*100), right-40, y+(h-u.lineH(mono, 13, semibold))/2, mono, 13, semibold, colTeal)
		}
		u.icon(iconChevronDown, right-34, y+11, 22, colMuted)
		u.area(left, y, w-48, h, g.openGoals)
		u.area(right-48, y, 48, h, toggle)
		return y + h
	}

	pad := 14.0
	inner := w - 2*pad
	h := g.goalCardBody(u, left+pad, y+12, inner, goal, prog, ok, n, true) + 24
	u.panel(left, y, w, h, 16, colChip, colBorder)
	g.goalCardBody(u, left+pad, y+12, inner, goal, prog, ok, n, false)
	u.area(left, y, w, h, g.openGoals)
	u.area(right-48, y, 48, 44, toggle)
	return y + h
}

// goalCardBody draws the expanded goal card's contents and returns their
// height. With measure set, it only measures.
func (g *Game) goalCardBody(u *ui, x, y, w float64, goal sim.Goal, prog float64, ok bool, n int, measure bool) float64 {
	was := u.measuring
	u.measuring = measure || was
	defer func() { u.measuring = was }()
	y0 := y
	if !ok {
		u.text("All goals complete", x, y, sans, 17, semibold, colText)
		y += u.lineH(sans, 17, semibold) + 4
		y += u.paragraph("The planet is alive. Keep terraforming, or start a new planet from the Build menu.", x, y, w, 13, colBody)
		return y - y0
	}
	u.text(fmt.Sprintf("GOAL %d OF %d", n, len(sim.Goals)), x, y, sans, 11, semibold, colMuted)
	u.icon(iconChevronUp, x+w-20, y-2, 20, colMuted)
	y += u.lineH(sans, 11, semibold) + 6
	u.text(goal.Name, x, y, sans, 17, semibold, colText)
	y += u.lineH(sans, 17, semibold) + 4
	y += u.paragraph(goal.Hint, x, y, w, 13, colBody)
	if prog > 0 && prog < 1 {
		y += 6
		u.fillRound(x, y, w, 6, 3, colTrack)
		u.fillRound(x, y, math.Max(w*prog, 6), 6, 3, colTeal)
		y += 6
	}
	return y - y0
}

func (g *Game) dock(u *ui, left, right, y float64) {
	type dockBtn struct {
		icon   iconName
		label  string
		active bool
		amber  bool
		action func()
	}
	bt, building := g.tool.building()
	buildLabel := "Build"
	if building {
		buildLabel = bt.String()
	}
	btns := []dockBtn{
		{iconBuild, buildLabel, g.menuOpen || building, true, func() { g.menuOpen = !g.menuOpen }},
		{iconLink, "Link", g.tool == toolLink, false, func() { g.setTool(toolLink) }},
		{iconInspect, "Inspect", g.tool == toolInspect, false, func() { g.setTool(toolInspect) }},
		{iconRemove, "Remove", g.tool == toolDemolish, false, func() { g.setTool(toolDemolish) }},
	}
	w := (right - left - gap*float64(len(btns)-1)) / float64(len(btns))
	for i, b := range btns {
		x := left + float64(i)*(w+gap)
		fill, border, fg := color.Color(colChip), color.Color(colBorder), color.Color(colText)
		switch {
		case b.active && b.amber:
			fill, border, fg = rgba(0x2A2112), colAmber, rgba(0xFFD9A0)
		case b.active:
			fill, border = colActiveBg, colActive
		}
		u.panel(x, y, w, dockH, 16, fill, border)
		u.icon(b.icon, x+(w-22)/2, y+10, 22, fg)
		lw := u.textW(b.label, sans, 12, semibold)
		u.text(b.label, x+(w-lw)/2, y+38, sans, 12, semibold, fg)
		u.area(x, y, w, dockH, b.action)
	}
}

func (g *Game) speedControl(u *ui, x, y float64) {
	w, h := 44.0, 36.0
	u.panel(x, y, float64(len(speeds))*w+8, h+8, 14, colChip, colBorder)
	for i := range speeds {
		bx := x + 4 + float64(i)*w
		active := g.speed == i
		if active {
			u.fillRound(bx, y+4, w, h, 10, colActiveBg)
		}
		fg := color.Color(colIcon)
		if active {
			fg = colText
		}
		if i == 0 {
			u.icon(iconPause, bx+(w-18)/2, y+4+(h-18)/2, 18, fg)
		} else {
			lw := u.textW(speeds[i].label, mono, 13, semibold)
			u.text(speeds[i].label, bx+(w-lw)/2, y+4+(h-u.lineH(mono, 13, semibold))/2, mono, 13, semibold, fg)
		}
		u.area(bx, y+4, w, h, func() { g.speed = i })
	}
}

// toast draws the current message, if any, in the space ending at bottom,
// and returns its top (bottom if there's no message).
func (g *Game) toast(u *ui, left, right, bottom float64) float64 {
	if g.ticks >= g.msgUntil || g.message == "" {
		return bottom
	}
	fill, border, fg, ic := color.Color(colChip), color.Color(colBorder), color.Color(colText), iconInspect
	switch g.msgKind {
	case msgWarn:
		fill, border, fg, ic = colAmberBg, colAmberLn, colAmberTx, iconWarning
	case msgGood:
		fill, border, fg, ic = rgba(0x0F2A26), colTeal, rgba(0xD5F7F0), iconLeaf
	}
	w := right - left
	f := u.face(sans, 13, regular)
	btnW := 0.0
	if g.msgAction != nil {
		btnW = u.textW(g.msgActionLabel, sans, 12, bold) + 24
	}
	lines := wrapWidth(g.message, f, (w-46-btnW)*u.s)
	if len(lines) > 3 {
		lines = lines[:3]
	}
	lh := 13 * 1.45
	h := float64(len(lines))*lh + 20
	if btnW > 0 {
		h = math.Max(h, 56)
	}
	y := bottom - h
	u.panel(left, y, w, h, 14, fill, border)
	if btnW > 0 {
		bx, bh := left+w-btnW-8, 40.0
		by := y + (h-bh)/2
		u.fillRound(bx, by, btnW, bh, 12, colTeal)
		u.text(g.msgActionLabel, bx+12, by+(bh-u.lineH(sans, 12, bold))/2, sans, 12, bold, rgba(0x06231E))
		action := g.msgAction
		u.area(bx, by, btnW, bh, func() {
			g.msgUntil = 0
			action()
		})
	}
	u.icon(ic, left+12, y+10, 18, fg)
	for i, line := range lines {
		u.text(line, left+38, y+10+float64(i)*lh, sans, 13, regular, fg)
	}
	return y
}

func (g *Game) viewRail(u *ui, right, top, bottom float64) {
	icons := []iconName{iconGlobe, iconThermo, iconDrop, iconLeaf, iconIndustry}
	n := float64(len(icons))
	h := n*railBtn + (n-1)*gap
	x := right - railBtn
	if bottom-top < h {
		// Not enough room for the whole rail: one button that cycles.
		y := bottom - railBtn
		u.panel(x, y, railBtn, railBtn, 14, colActiveBg, colActive)
		u.icon(icons[g.view], x+13, y+13, 22, colText)
		u.area(x, y, railBtn, railBtn, func() { g.view = (g.view + 1) % numViews; g.atlasDirty = true })
		return
	}
	y := top + (bottom-top-h)/2
	for i, ic := range icons {
		v := viewMode(i)
		by := y + float64(i)*(railBtn+gap)
		fill, border, fg := color.Color(colChip), color.Color(colBorder), color.Color(colIcon)
		if v == g.view {
			fill, border, fg = colActiveBg, colActive, colText
		}
		u.panel(x, by, railBtn, railBtn, 14, fill, border)
		u.icon(ic, x+13, by+13, 22, fg)
		u.area(x, by, railBtn, railBtn, func() { g.view = v; g.atlasDirty = true })
	}
}

// buildMenu draws the building picker ending at bottom and returns its top:
// a row of category tabs, then that category's buildings. Locked ones say
// which goal unlocks them.
func (g *Game) buildMenu(u *ui, left, right, bottom float64) float64 {
	p := g.planet
	w := right - left
	cardH := 104.0
	var types []sim.BuildingType
	for bt := range sim.NumBuildingTypes {
		if bt != sim.Lander && sim.Specs[bt].Category == g.buildTab {
			types = append(types, bt)
		}
	}
	rows := (len(types) + 1) / 2
	h := 16 + 26 + 12 + 34 + 10 + float64(rows)*cardH + float64(rows-1)*gap + 12 + 40 + 14
	y := bottom - h
	u.panel(left, y, w, h, 20, colSheet, colBorder)
	x := left + 14
	inner := w - 28
	cy := y + 16
	u.text("Build", x, cy, sans, 20, semibold, colText)
	metal := fmt.Sprintf("%.1f metal", p.Stores()[sim.Metal])
	u.textRight(metal, x+inner, cy+5, mono, 13, semibold, colBody)
	cy += 26 + 12

	// Category tabs.
	tw := (inner - 4*4) / float64(sim.NumCategories)
	for c := range sim.NumCategories {
		tx := x + float64(c)*(tw+4)
		fill, fg := color.Color(colCard), color.Color(colIcon)
		if c == g.buildTab {
			fill, fg = colActiveBg, colText
		}
		u.fillRound(tx, cy, tw, 34, 10, fill)
		lw := u.textW(c.String(), sans, 12, semibold)
		u.text(c.String(), tx+(tw-lw)/2, cy+(34-u.lineH(sans, 12, semibold))/2, sans, 12, semibold, fg)
		u.area(tx, cy, tw, 34, func() { g.buildTab = c })
	}
	cy += 34 + 10

	cw := (inner - gap) / 2
	for i, bt := range types {
		t := buildTool(bt)
		spec := sim.Specs[bt]
		info := buildInfo[bt]
		cx := x + float64(i%2)*(cw+gap)
		ccy := cy + float64(i/2)*(cardH+gap)
		unlocked := p.Unlocked(bt)
		affordable := p.Stores()[sim.Metal] >= spec.Cost
		fill, border := color.Color(colCard), color.Color(rgba(0x2B374A))
		if g.tool == t {
			fill, border = rgba(0x221A10), colAmber
		}
		u.panel(cx, ccy, cw, cardH, 14, fill, border)
		nameCol, bodyCol := color.Color(colText), color.Color(colBody)
		if !affordable || !unlocked {
			nameCol, bodyCol = colMuted, colMuted
		}
		g.buildingGlyph(u, bt, cx+10, ccy+10, 28)
		// The name, on two lines if it's too long for one.
		costW := u.textW(fmt.Sprintf("%.0f", spec.Cost), mono, 13, semibold) + 8
		nameW := cw - 46 - 10 - costW
		if first, rest, ok := strings.Cut(spec.Name, " "); ok && u.textW(spec.Name, sans, 14, semibold) > nameW {
			u.text(first, cx+46, ccy+4, sans, 13, semibold, nameCol)
			u.text(rest, cx+46, ccy+21, sans, 13, semibold, nameCol)
		} else {
			u.text(spec.Name, cx+46, ccy+10, sans, 14, semibold, nameCol)
		}
		u.textRight(fmt.Sprintf("%.0f", spec.Cost), cx+cw-10, ccy+12, mono, 13, semibold, nameCol)
		effect, tag := info.effect, info.tag
		tagCol := color.Color(colTealTx)
		if spec.Pollution > 0 {
			tagCol = colAmber
		}
		if !unlocked {
			tag, tagCol = "After: "+spec.UnlockedBy, colMuted
		}
		u.paragraph(effect, cx+10, ccy+46, cw-20, 12, bodyCol)
		u.text(tag, cx+10, ccy+cardH-22, sans, 11, semibold, tagCol)
		u.area(cx, ccy, cw, cardH, func() {
			if !unlocked {
				g.showMessage(fmt.Sprintf("%s unlocks after the goal %q", spec.Name, spec.UnlockedBy), msgInfo)
				return
			}
			g.setTool(t)
			g.showMessage(placeHint(bt), msgInfo)
		})
	}
	fy := cy + float64(rows)*cardH + float64(rows-1)*gap + 12
	label := "Start a new planet"
	col := color.Color(colMuted)
	if g.ticks < g.confirmNewUntil {
		label, col = "Tap again to abandon this planet", colRed
	}
	u.text(label, x, fy+(40-u.lineH(sans, 13, semibold))/2, sans, 13, semibold, col)
	u.area(x, fy, inner, 40, func() {
		if g.ticks < g.confirmNewUntil {
			g.confirmNewUntil = 0
			g.newPlanet()
			return
		}
		g.confirmNewUntil = g.ticks + 3*ebiten.TPS()
	})
	return y
}

// placeHint is the message shown when a build tool is picked.
func placeHint(bt sim.BuildingType) string {
	switch sim.Specs[bt].Site {
	case sim.DepositSite:
		return "Tap a deposit to mine it (they show in the Industry view: gold iron, purple fluorite, pale green nitrate)"
	case sim.IceSite:
		return "Tap an icy tile to drill it"
	case sim.VolcanicSite:
		return "Tap a volcanic tile (red in the Industry view)"
	case sim.RemoteSite:
		return "Tap where the comet should strike, well away from your buildings"
	}
	return fmt.Sprintf("Tap a tile to build a %s", sim.Specs[bt].Name)
}

// buildingGlyph draws a small picture of building type bt in a size×size box,
// matching the designs the planet shader draws.
func (g *Game) buildingGlyph(u *ui, bt sim.BuildingType, x, y, size float64) {
	if !u.drawing() {
		return
	}
	k := size / 24
	cx, cy := x+size/2, y+size/2
	s := u.s
	pt := func(px, py float64) (float32, float32) { return f32((cx + px*k) * s), f32((cy + py*k) * s) }
	ring := func(r, width float64, c color.Color) {
		vector.StrokeCircle(u.dst, f32(cx*s), f32(cy*s), f32(r*k*s), f32(width*k*s), c, true)
	}
	line := func(x0, y0, x1, y1, width float64, c color.Color) {
		ax, ay := pt(x0, y0)
		bx, by := pt(x1, y1)
		vector.StrokeLine(u.dst, ax, ay, bx, by, f32(width*k*s), c, true)
	}
	fillPath := func(p *vector.Path, c color.Color) {
		op := &vector.DrawPathOptions{AntiAlias: true}
		op.ColorScale.ScaleWithColor(c)
		vector.FillPath(u.dst, p, nil, op)
	}
	switch bt {
	case sim.Lander:
		line(-7, 4, -11, 7, 1.4, rgba(0x8A96AA))
		line(7, 4, 11, 7, 1.4, rgba(0x8A96AA))
		line(0, -7, 0, -11, 1.4, rgba(0x8A96AA))
		var hex vector.Path
		for i := range 6 {
			a := math.Pi/6 + float64(i)*math.Pi/3
			px, py := pt(8.5*math.Cos(a), 8.5*math.Sin(a))
			if i == 0 {
				hex.MoveTo(px, py)
			} else {
				hex.LineTo(px, py)
			}
		}
		hex.Close()
		fillPath(&hex, rgba(0x3A4658))
		u.circle(cx, cy, 5*k, colText)
		ring(4.3, 1.6, rgba(0x5AD1F5))
		u.circle(cx, cy, 1.8*k, rgba(0x5AD1F5))
	case sim.SolarPanel:
		for i := range 2 {
			for j := range 2 {
				px, py := x+(2+float64(i)*10.5)*k, y+(3+float64(j)*9.5)*k
				u.fillRound(px, py, 9.5*k, 8.5*k, 1, rgba(0x1E3A78))
				u.strokeRound(px, py, 9.5*k, 8.5*k, 1, 1, colBlue)
			}
		}
	case sim.Mine:
		u.circle(cx, cy, 10*k, rgba(0x6E5446))
		u.circle(cx, cy, 7*k, rgba(0x4A372D))
		u.circle(cx, cy, 4*k, rgba(0x2A1E18))
		u.circle(cx-5*k, cy+3*k, 1*k, rgba(0xF2C04A))
		u.circle(cx+6*k, cy-1*k, 0.9*k, rgba(0xF2C04A))
		line(-3.5, -5, 0, -11, 1.2, colIcon)
		line(3.5, -5, 0, -11, 1.2, colIcon)
		line(0, -11, 0, -1, 1, colIcon)
	case sim.Smelter:
		u.fillRound(x+2.5*k, y+9*k, 19*k, 13*k, 2, rgba(0xC46A2A))
		u.fillRound(x+2.5*k, y+9*k, 19*k, 3.5*k, 1, rgba(0x8E4A1C))
		u.fillRound(x+15*k, y+3*k, 4*k, 9*k, 1, rgba(0x2A2A30))
		u.fillRound(x+5*k, y+15*k, 7.5*k, 4.5*k, 1, rgba(0xFFD36B))
	case sim.HeaterMachine:
		u.circle(cx, cy, 11.5*k, withAlpha(colAmber, 60))
		for i := range 8 {
			a := float64(i) * math.Pi / 4
			line(7*math.Cos(a), 7*math.Sin(a), 10.3*math.Cos(a), 10.3*math.Sin(a), 1.6, rgba(0xB83A26))
		}
		ring(5.8, 2.6, rgba(0xE2553A))
		u.circle(cx, cy, 2.9*k, rgba(0xFFD36B))
	case sim.IceDrill:
		u.circle(cx, cy+3*k, 9*k, rgba(0x9EBCDB))
		u.circle(cx, cy+3*k, 7*k, rgba(0x4D6B8F))
		line(-4, 6, 0, -10, 1.3, colIcon)
		line(4, 6, 0, -10, 1.3, colIcon)
		line(0, -10, 0, 7, 1.1, colIcon)
		u.circle(cx, cy+7*k, 1.8*k, rgba(0x5AD1F5))
	case sim.Electrolyser:
		line(-6, 0, 6, 0, 2, rgba(0x9AA6B8))
		u.circle(cx-5.5*k, cy+1*k, 5*k, rgba(0x528CE6))
		u.circle(cx+5.5*k, cy+1*k, 5*k, rgba(0x5FD3B5))
		u.circle(cx+5.5*k, cy-8*k, 1.4*k, rgba(0xC0FFF0))
		u.circle(cx+7*k, cy-11*k, 1*k, rgba(0xC0FFF0))
	case sim.GreenhouseFactory:
		u.fillRound(x+2.5*k, y+10*k, 19*k, 12*k, 2, rgba(0x6B5C80))
		u.fillRound(x+5*k, y+3*k, 3.5*k, 9*k, 1, rgba(0x332E3D))
		u.fillRound(x+15*k, y+2*k, 3.5*k, 10*k, 1, rgba(0x332E3D))
		u.fillRound(x+5*k, y+16*k, 11*k, 3*k, 1, rgba(0xB07CE8))
		u.circle(cx+5*k, cy-11*k, 2*k, withAlpha(colPFC, 160))
	case sim.NitrogenPlant:
		u.fillRound(x+2*k, y+4*k, 20*k, 16*k, 2, rgba(0x6E7A8C))
		for _, d := range [][2]float64{{-5, 3}, {5, 3}, {0, -4}} {
			u.circle(cx+d[0]*k, cy+d[1]*k, 4*k, rgba(0x80A6E8))
			u.circle(cx+d[0]*k, cy+d[1]*k, 2.6*k, rgba(0xB8D6FF))
		}
	case sim.DustSpreader:
		u.circle(cx, cy, 8*k, rgba(0x8C6950))
		u.circle(cx, cy, 5*k, rgba(0x614534))
		line(-9, -5, 9, 5, 1.6, colIcon)
		u.circle(cx+10*k, cy-6*k, 1.2*k, rgba(0x9C7458))
		u.circle(cx-10*k, cy+7*k, 1.2*k, rgba(0x9C7458))
	case sim.WindTurbine:
		line(0, 0, 0, 11, 1.4, rgba(0x9AA6B8))
		for i := range 3 {
			a := -math.Pi/2 + float64(i)*2*math.Pi/3
			line(0, 0, 10*math.Cos(a), 10*math.Sin(a), 1.8, colText)
		}
		u.circle(cx, cy, 1.8*k, rgba(0x9AA6B8))
	case sim.Geothermal:
		u.circle(cx, cy+2*k, 9.5*k, rgba(0x403333))
		u.circle(cx, cy+2*k, 4.5*k, rgba(0xE5512A))
		u.circle(cx, cy+2*k, 2*k, rgba(0xFFC04D))
		u.circle(cx+1*k, cy-8*k, 2.6*k, withAlpha(colText, 140))
	case sim.OrbitalMirror:
		u.circle(cx, cy, 11*k, withAlpha(rgba(0xFFE08A), 60))
		for i := range 8 {
			a := float64(i) * math.Pi / 4
			line(5.5*math.Cos(a), 5.5*math.Sin(a), 9.5*math.Cos(a), 9.5*math.Sin(a), 1.3, rgba(0xFFD866))
		}
		u.circle(cx, cy, 3.6*k, rgba(0xFFF5CC))
	case sim.Comet:
		ring(8, 1.4, colRed)
		line(-11, 0, -4, 0, 1.3, colRed)
		line(4, 0, 11, 0, 1.3, colRed)
		line(0, -11, 0, -4, 1.3, colRed)
		line(0, 4, 0, 11, 1.3, colRed)
	case sim.MagneticShield:
		ring(10, 1.2, rgba(0x73A6FF))
		ring(7, 1.2, rgba(0x73A6FF))
		u.circle(cx, cy, 4*k, rgba(0x9AA6B8))
		u.circle(cx-1*k, cy-1*k, 1.8*k, rgba(0xC0E0FF))
	case sim.Pylon:
		line(0, -10, 0, 10, 1.6, rgba(0x9AA6B8))
		line(-7, -6, 7, -6, 1.4, rgba(0x9AA6B8))
		line(-5, -1, 5, -1, 1.2, rgba(0x9AA6B8))
		line(-4, 10, 0, 2, 1.2, rgba(0x9AA6B8))
		line(4, 10, 0, 2, 1.2, rgba(0x9AA6B8))
		u.circle(cx-7*k, cy-6*k, 1.5*k, rgba(0x5AD1F5))
		u.circle(cx+7*k, cy-6*k, 1.5*k, rgba(0x5AD1F5))
	case sim.CarbonScrubber:
		u.fillRound(x+3*k, y+5*k, 18*k, 15*k, 3, rgba(0x55606E))
		for i := range 3 {
			u.fillRound(x+(5+float64(i)*5.2)*k, y+7*k, 3.6*k, 11*k, 1, rgba(0x2A313B))
		}
		u.circle(cx, cy-9*k, 2.4*k, withAlpha(colCO2, 200))
	case sim.Biolab:
		u.circle(cx, cy, 10*k, rgba(0x9AA6B8))
		u.circle(cx, cy, 8.5*k, rgba(0x294D4D))
		u.circle(cx-3*k, cy+1*k, 2.6*k, rgba(0x4DB35A))
		u.circle(cx+2.5*k, cy-1.5*k, 3*k, rgba(0x4DB35A))
		u.circle(cx, cy+4*k, 2*k, rgba(0x4DB35A))
	}
}

// metalRate is metal produced per in-game day, recently.
func (g *Game) metalRate() float64 {
	h := g.metalHist
	if len(h) < 2 {
		return 0
	}
	return (h[len(h)-1] - h[0]) / float64(len(h)-1)
}
