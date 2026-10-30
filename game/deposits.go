package game

import (
	"math"
	"sort"

	"github.com/zachaller/terraformer/sim"
)

// depositsFor lists the free deposits (ore left, nothing built on them) a
// mine could go on, best first: those a hub already covers, so a mine there
// connects straight away, then the rest. Within each group they're ordered
// by distance from near, or from the nearest hub when near is nil. If only
// is set, it lists deposits of that mineral alone; otherwise those of the
// minerals that are any use yet.
func (g *Game) depositsFor(near *sim.Cell, only *sim.Mineral) []sim.Cell {
	p := g.planet
	var hubs []*sim.Building
	for _, b := range p.Buildings() {
		if b.Spec().Reach > 0 {
			hubs = append(hubs, b)
		}
	}
	type cand struct {
		c       sim.Cell
		covered bool
		dist    float64
	}
	var cands []cand
	for i, t := range p.Tiles {
		if t.Ore <= 0 || (only != nil && t.Mineral != *only) || (only == nil && !g.mineralWanted(t.Mineral)) {
			continue
		}
		c := p.Grid.CellAt(i)
		if p.BuildingAt(c) != nil {
			continue
		}
		hubDist := math.Inf(1)
		for _, h := range hubs {
			hubDist = math.Min(hubDist, p.Distance(c, p.Grid.CellAt(h.Tile)))
		}
		d := hubDist
		if near != nil {
			d = p.Distance(c, *near)
		}
		cands = append(cands, cand{c, len(hubs) > 0 && hubDist <= sim.Specs[sim.Pylon].CoverRadius, d})
	}
	sort.SliceStable(cands, func(a, b int) bool {
		if cands[a].covered != cands[b].covered {
			return cands[a].covered
		}
		return cands[a].dist < cands[b].dist
	})
	out := make([]sim.Cell, len(cands))
	for i, c := range cands {
		out[i] = c.c
	}
	return out
}

// mineralWanted reports whether mineral m is any use yet: iron always,
// fluorite and nitrate once a building that uses them is unlocked.
func (g *Game) mineralWanted(m sim.Mineral) bool {
	switch m {
	case sim.Fluorite:
		return g.planet.Unlocked(sim.GreenhouseFactory)
	case sim.Nitrate:
		return g.planet.Unlocked(sim.NitrogenPlant)
	}
	return true
}

// nextDeposit previews the mine on the next free deposit in depositsFor's
// order, turning the planet to it; tapping again moves on to the one after.
func (g *Game) nextDeposit() {
	ds := g.depositsFor(nil, nil)
	if len(ds) == 0 {
		g.showMessage("There are no free deposits left", msgWarn)
		return
	}
	c := ds[g.depositNext%len(ds)]
	g.depositNext++
	g.pending, g.selected, g.focus = &c, &c, &c
	g.atlasDirty = true
}

// snapToDeposit returns the deposit a tap on c meant, for placing a mine:
// c itself if it's a free deposit, else the nearest free one within two
// tiles, else c.
func (g *Game) snapToDeposit(c sim.Cell) sim.Cell {
	p := g.planet
	free := func(c sim.Cell) bool {
		return p.Tiles[p.Grid.Index(c)].Ore > 0 && p.BuildingAt(c) == nil
	}
	if free(c) {
		return c
	}
	for _, a := range areaCells(p.Grid, c, 2) {
		if free(a.cell) {
			return a.cell
		}
	}
	return c
}

// moveMine moves the mine on c, whose patch is dug out, to the nearest free
// deposit, preferring its own mineral. It reports what happened.
func (g *Game) moveMine(c sim.Cell) {
	p := g.planet
	spec := sim.Specs[sim.Mine]
	m := p.Tiles[p.Grid.Index(c)].Mineral
	ds := g.depositsFor(&c, &m)
	if len(ds) == 0 {
		ds = g.depositsFor(&c, nil)
	}
	if len(ds) == 0 {
		g.showMessage("There are no free deposits left", msgWarn)
		return
	}
	if p.Stores()[sim.Metal]+spec.Cost/2 < spec.Cost {
		g.showMessage("Not enough metal to move it", msgWarn)
		return
	}
	dest := ds[0]
	if err := p.Demolish(c); err != nil {
		g.showMessage(err.Error(), msgWarn)
		return
	}
	if _, err := p.Build(sim.Mine, dest); err != nil {
		g.showMessage(err.Error(), msgWarn)
		return
	}
	g.sound(sndBuildIndustry)
	g.selected, g.focus = &dest, &dest
	g.atlasDirty = true
	g.showMessage("Moved the mine to a new deposit", msgGood)
}
