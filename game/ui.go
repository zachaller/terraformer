package game

import (
	"fmt"
	"image/color"
	"math"

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
	toolSolar
	toolMine
	toolSmelter
	toolHeater
)

// buildTools are the tools offered in the build menu, in order.
var buildTools = []tool{toolSolar, toolMine, toolSmelter, toolHeater}

// building returns the building type a tool places, if it places one.
func (t tool) building() (sim.BuildingType, bool) {
	switch t {
	case toolSolar:
		return sim.SolarPanel, true
	case toolMine:
		return sim.Mine, true
	case toolSmelter:
		return sim.Smelter, true
	case toolHeater:
		return sim.HeaterMachine, true
	}
	return 0, false
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
}

// buildInfo is the build menu's description of each building.
var buildInfo = map[sim.BuildingType]struct{ effect, tag string }{
	sim.SolarPanel:    {"Makes 2 power", "Clean"},
	sim.Mine:          {"Digs ore from a deposit", "1 power · pollutes"},
	sim.Smelter:       {"Turns 2 ore into 1 metal", "2 power · pollutes"},
	sim.HeaterMachine: {"+40° here, +20° around", "3 power · clean"},
}

// setTool switches tool, closing the menu and cancelling a half-made link.
func (g *Game) setTool(t tool) {
	g.tool = t
	g.menuOpen = false
	g.linkFrom = nil
	g.pending = nil
	g.linkDrag = nil
	g.goalsOpen = false
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
		if !g.inspectOpen() && !g.goalsOpen && !g.menuOpen {
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
			g.toast(u, left+3*44+16, narrow, above)
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
		x += w + 6
	}
	return y + chipH
}

func (g *Game) statusBar(u *ui, left, right, y float64) float64 {
	p := g.planet
	st := p.Stats()
	chips := []chip{
		{parts: []chipPart{labelPart("Day"), numPart(fmt.Sprint(p.Ticks/10), colText)}, fill: colChip, border: colBorder},
		{parts: []chipPart{iconPart(iconAir, colBlue), numPart(fmt.Sprintf("%.2f", p.Atmosphere), colText), labelPart("kPa")}, fill: colChip, border: colBorder},
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
	return u.flowChips(chips, left, right, y+6)
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

	if g.goalCollapsed || g.inspectOpen() || g.menuOpen || g.goalsOpen {
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
	u.panel(x, y, 3*w+8, h+8, 14, colChip, colBorder)
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

// buildMenu draws the building picker ending at bottom and returns its top.
func (g *Game) buildMenu(u *ui, left, right, bottom float64) float64 {
	w := right - left
	cardH := 96.0
	h := 16 + 26 + 12 + 2*cardH + gap + 12 + 40 + 14
	y := bottom - h
	u.panel(left, y, w, h, 20, colSheet, colBorder)
	x := left + 14
	inner := w - 28
	cy := y + 16
	u.text("Build", x, cy, sans, 20, semibold, colText)
	metal := fmt.Sprintf("%.1f metal", g.planet.Stores()[sim.Metal])
	u.textRight(metal, x+inner, cy+5, mono, 13, semibold, colBody)
	cy += 26 + 12
	cw := (inner - gap) / 2
	for i, t := range buildTools {
		bt, _ := t.building()
		spec := sim.Specs[bt]
		info := buildInfo[bt]
		cx := x + float64(i%2)*(cw+gap)
		ccy := cy + float64(i/2)*(cardH+gap)
		affordable := g.planet.Stores()[sim.Metal] >= spec.Cost
		fill, border := color.Color(colCard), color.Color(rgba(0x2B374A))
		if g.tool == t {
			fill, border = rgba(0x221A10), colAmber
		}
		u.panel(cx, ccy, cw, cardH, 14, fill, border)
		nameCol, bodyCol := color.Color(colText), color.Color(colBody)
		if !affordable {
			nameCol, bodyCol = colMuted, colMuted
		}
		g.buildingGlyph(u, bt, cx+10, ccy+10, 28)
		u.text(spec.Name, cx+46, ccy+10, sans, 15, semibold, nameCol)
		u.textRight(fmt.Sprintf("%.0f", spec.Cost), cx+cw-10, ccy+12, mono, 13, semibold, nameCol)
		u.paragraph(info.effect, cx+10, ccy+44, cw-20, 12, bodyCol)
		tagCol := color.Color(colAmber)
		if spec.Pollution == 0 {
			tagCol = colTealTx
		}
		u.text(info.tag, cx+10, ccy+cardH-22, sans, 11, semibold, tagCol)
		u.area(cx, ccy, cw, cardH, func() {
			g.setTool(t)
			g.showMessage(fmt.Sprintf("Tap a tile to build a %s", spec.Name), msgInfo)
		})
	}
	fy := cy + 2*cardH + gap + 12
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
