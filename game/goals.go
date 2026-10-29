package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/zachaller/terraformer/sim"
)

// openGoals shows the goals sheet, closing anything else that's open.
func (g *Game) openGoals() {
	g.setTool(toolInspect)
	g.selected = nil
	g.goalsOpen = true
}

// announceGoals shows a message when goals are completed, with a button to
// see where it happened, or else the goal list.
func (g *Game) announceGoals() {
	done := g.planet.GoalsDone()
	if done <= g.goalsSeen {
		return
	}
	g.goalsSeen = done
	if done == 0 {
		return
	}
	msg := "Goal complete: " + sim.Goals[done-1].Name + "!"
	if done < len(sim.Goals) {
		msg += " Next: " + sim.Goals[done].Name
	} else {
		msg += " The planet is alive"
	}
	if c, ok := g.goalPlace(sim.Goals[done-1].Name); ok {
		g.showMessageAction(msg, msgGood, "Show me", func() {
			g.setTool(toolInspect)
			g.selected, g.focus = &c, &c
		})
		return
	}
	g.showMessageAction(msg, msgGood, "Goals", g.openGoals)
}

// goalPlace picks a tile that shows off a just-completed goal, if it has one.
func (g *Game) goalPlace(name string) (sim.Cell, bool) {
	p := g.planet
	score := func(t sim.Tile) float64 { return 0 }
	switch name {
	case "Liquid water":
		score = func(t sim.Tile) float64 { return t.Water }
	case "First life", "Spread life":
		score = func(t sim.Tile) float64 { return t.Life }
	default:
		return sim.Cell{}, false
	}
	best, bestScore := -1, 0.0
	for idx, t := range p.Tiles {
		if s := score(t); s > bestScore {
			best, bestScore = idx, s
		}
	}
	if best < 0 {
		return sim.Cell{}, false
	}
	return p.Grid.CellAt(best), true
}

// goalsSheet draws the list of goals along the bottom of the screen and
// returns its top.
func (g *Game) goalsSheet(u *ui, sheetLeft, sheetRight, screenBottom float64) float64 {
	_, _, _, sb := currentSafeArea()
	sb /= u.s
	pad := 18.0
	x, w := sheetLeft+pad, sheetRight-sheetLeft-2*pad

	was := u.measuring
	u.measuring = true
	h := g.goalsList(u, x, 0, w)
	u.measuring = was

	top := screenBottom - sb - h - 30
	u.fillRound(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top+24, 24, colSheet)
	u.strokeRound(sheetLeft+0.5, top+0.5, sheetRight-sheetLeft-1, screenBottom-top+24, 24, 1, rgba(0x2B374A))
	u.area(sheetLeft, top, sheetRight-sheetLeft, screenBottom-top, nil)
	u.fillRound(sheetLeft+(sheetRight-sheetLeft-40)/2, top+8, 40, 4, 2, rgba(0x3A4658))
	g.goalsList(u, x, top+20, w)
	return top
}

func (g *Game) goalsList(u *ui, x, y, w float64) float64 {
	p := g.planet
	y0 := y
	u.text("Goals", x, y, sans, 20, semibold, colText)
	u.textRight(fmt.Sprintf("%d / %d", p.GoalsDone(), len(sim.Goals)), x+w-52, y+6, mono, 13, semibold, colMuted)
	u.icon(iconCross, x+w-28, y+2, 24, colMuted)
	u.area(x+w-44, y-8, 44, 44, func() { g.goalsOpen = false })
	y += 40

	current, prog, _ := p.CurrentGoal()
	for i, goal := range sim.Goals {
		done := i < p.GoalsDone()
		if goal.Name == current.Name && !done {
			// The current goal: expanded, with hint and progress.
			inner := w - 24
			measure := func() float64 {
				was := u.measuring
				u.measuring = true
				defer func() { u.measuring = was }()
				return g.currentGoalBody(u, x+12, 0, inner, goal, prog)
			}
			h := measure() + 24
			y += 4
			u.panel(x, y, w, h, 14, colCard, colTeal)
			g.currentGoalBody(u, x+12, y+12, inner, goal, prog)
			y += h + 4
			continue
		}
		rowH := 38.0
		dotX, dotY := x+11, y+rowH/2
		col := color.Color(rgba(0x8C98AC))
		if done {
			u.circle(dotX, dotY, 11, rgba(0x164A42))
			u.icon(iconCheck, dotX-7, dotY-7, 14, colTealTx)
			col = colMuted
		} else if u.drawing() {
			u.strokeRound(dotX-10, dotY-10, 20, 20, 10, 2, rgba(0x3A4658))
		}
		u.text(goal.Name, x+34, y+(rowH-u.lineH(sans, 13, regular))/2, sans, 13, regular, col)
		if i < len(sim.Goals)-1 {
			u.fillRound(x, y+rowH-1, w, 1, 0, rgba(0x1F2836))
		}
		y += rowH
	}
	return y - y0
}

func (g *Game) currentGoalBody(u *ui, x, y, w float64, goal sim.Goal, prog float64) float64 {
	y0 := y
	u.circle(x+11, y+11, 10, colTeal)
	u.circle(x+11, y+11, 8, colCard)
	u.circle(x+11, y+11, 4, colTeal)
	u.text(goal.Name, x+34, y+1, sans, 15, semibold, colText)
	u.textRight(fmt.Sprintf("%.0f%%", prog*100), x+w, y+3, mono, 13, semibold, colTeal)
	y += 28
	y += u.paragraph(goal.Hint, x, y, w, 12, colBody)
	y += 8
	u.fillRound(x, y, w, 6, 3, colTrack)
	u.fillRound(x, y, math.Max(w*prog, 6), 6, 3, colTeal)
	return y + 6 - y0
}

// alert is a live problem with the colony and a one-tap way to deal with it.
type alert struct {
	icon   iconName
	text   string
	warn   bool
	label  string
	action func()
}

// alerts lists the colony's current problems, most urgent first.
func (g *Game) alerts() []alert {
	p := g.planet
	if !p.Landed() {
		return nil
	}
	var unpowered, starved, full, empty []*sim.Building
	for _, b := range p.Buildings() {
		switch p.Diagnose(b).Kind {
		case sim.StatusNoPower:
			unpowered = append(unpowered, b)
		case sim.StatusNoInput:
			starved = append(starved, b)
		case sim.StatusOutputFull:
			full = append(full, b)
		case sim.StatusDepleted:
			empty = append(empty, b)
		}
	}
	show := func(b *sim.Building) func() {
		return func() {
			c := p.Grid.CellAt(b.Tile)
			g.setTool(toolInspect)
			g.selected, g.focus = &c, &c
		}
	}
	plural := func(bs []*sim.Building, one, many string) string {
		if len(bs) == 1 {
			return fmt.Sprintf(one, bs[0].Spec().Name)
		}
		return fmt.Sprintf(many, len(bs))
	}

	var out []alert
	if len(unpowered) > 0 {
		out = append(out, alert{iconLink, plural(unpowered, "%s has no power", "%d buildings have no power"), true, "Show", show(unpowered[0])})
	}
	if n := p.NetworkOf(p.Buildings()[0]); n.PowerSatisfaction() < 1 {
		out = append(out, alert{iconBolt, fmt.Sprintf("Colony short of power (%.0f / %.0f)", n.PowerSupply, n.PowerDemand), true, "Add solar", func() {
			g.setTool(toolSolar)
			g.showMessage("Tap a tile to place a solar panel, then link it to the colony", msgInfo)
		}})
	}
	if len(empty) > 0 {
		out = append(out, alert{iconMetal, plural(empty, "A %s ran out of ore", "%d mines ran out of ore"), false, "Find ore", func() {
			g.view = viewIndustry
			g.atlasDirty = true
			g.setTool(toolMine)
			g.showMessage("Ore deposits are gold in this view. Tap one to place a new mine", msgInfo)
		}})
	}
	if len(starved) > 0 {
		out = append(out, alert{iconWarning, plural(starved, "%s is waiting for ore", "%d buildings are waiting for ore"), false, "Show", show(starved[0])})
	}
	if len(full) > 0 {
		out = append(out, alert{iconWarning, plural(full, "%s has nowhere to send its output", "%d buildings have nowhere to send output"), false, "Show", show(full[0])})
	}
	return out
}

// alertRows draws up to two alerts from y and returns the bottom.
func (g *Game) alertRows(u *ui, left, right, y float64) float64 {
	as := g.alerts()
	if len(as) > 2 {
		as = as[:2]
	}
	for _, a := range as {
		y += 6
		h := 44.0
		fill, border, fg := color.Color(colCard), color.Color(rgba(0x2B374A)), color.Color(colIcon)
		if a.warn {
			fill, border, fg = colAmberBg, colAmberLn, colAmberTx
		}
		u.panel(left, y, right-left, h, 14, fill, border)
		u.icon(a.icon, left+12, y+13, 18, fg)
		bw := u.textW(a.label, sans, 12, semibold) + 20
		u.text(a.text, left+40, y+(h-u.lineH(sans, 13, regular))/2, sans, 13, regular, fg)
		bx := right - bw - 6
		u.fillRound(bx, y+5, bw, 34, 10, withAlpha(rgba(0x000000), 60))
		u.strokeRound(bx, y+5, bw, 34, 10, 1, border)
		u.text(a.label, bx+10, y+5+(34-u.lineH(sans, 12, semibold))/2, sans, 12, semibold, colText)
		u.area(bx, y+5, bw, 34, a.action)
		y += h
	}
	return y
}
