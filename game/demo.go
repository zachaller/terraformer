package game

import (
	"math"
	"slices"

	"github.com/zachaller/terraformer/sim"
)

// placeDemoHeaters puts fixed warmth on a ring of tiles around the equator:
// enough to tip the planet into runaway warming.
func placeDemoHeaters(p *sim.Planet) {
	for idx := range p.Tiles {
		c := p.Grid.CellAt(idx)
		if d := p.Grid.DirFromCell(c); math.Abs(d.Y) < 0.05 && idx%7 == 0 {
			p.AddHeat(c, demoHeat)
		}
	}
}

// placeDemoColony lands near the ore deposit closest to direction front and
// builds a small working colony: a mine, a smelter, a solar panel and a
// heater, all linked to the lander. Used to preview the economy.
func placeDemoColony(p *sim.Planet, front sim.Vec3) {
	g := p.Grid
	byDistance := func(from sim.Vec3) []sim.Cell {
		cells := make([]sim.Cell, g.NumCells())
		for i := range cells {
			cells[i] = g.CellAt(i)
		}
		slices.SortStableFunc(cells, func(a, b sim.Cell) int {
			da, db := g.DirFromCell(a).Dot(from), g.DirFromCell(b).Dot(from)
			switch {
			case da > db:
				return -1
			case da < db:
				return 1
			}
			return 0
		})
		return cells
	}

	var ore sim.Cell
	for _, c := range byDistance(front) {
		if p.Tile(c).Ore > 0 {
			ore = c
			break
		}
	}
	near := byDistance(g.DirFromCell(ore))
	free := func() sim.Cell {
		for _, c := range near[2:] {
			if p.BuildingAt(c) == nil && p.Tile(c).Ore == 0 {
				return c
			}
		}
		return near[0]
	}
	lander := free()
	if p.Land(lander) != nil {
		return
	}
	build := func(bt sim.BuildingType, c sim.Cell) {
		if _, err := p.Build(bt, c); err == nil {
			_ = p.Link(lander, c)
		}
	}
	build(sim.Mine, ore)
	build(sim.Smelter, free())
	build(sim.SolarPanel, free())
	build(sim.HeaterMachine, free())
}
