package game

import (
	"math"
	"slices"

	"github.com/zachaller/terraformer/sim"
)

// placeDemoHeaters puts fixed warmth on a ring of tiles around the equator,
// and a little super-greenhouse gas in the air: enough to tip the planet
// into runaway warming.
func placeDemoHeaters(p *sim.Planet) {
	p.Air.PFC = 0.5
	for idx := range p.Tiles {
		c := p.Grid.CellAt(idx)
		if d := p.Dir(c); math.Abs(d.Y) < 0.05 && idx%3 == 0 {
			p.AddHeat(c, demoHeat)
		}
	}
}

// placeDemoColony lands near the ore deposit closest to direction front and
// builds a small working colony: a mine, a smelter, a solar panel and a
// heater, all connected through the lander. Used to preview the economy.
// With showcase set, it also builds one of every other building.
func placeDemoColony(p *sim.Planet, front sim.Vec3, showcase bool) {
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
	build := func(bt sim.BuildingType, c sim.Cell) { _, _ = p.Build(bt, c) }
	build(sim.Mine, ore)
	build(sim.Smelter, free())
	build(sim.SolarPanel, free())
	build(sim.HeaterMachine, free())
	if !showcase {
		return
	}
	// One of everything else, on tiles made suitable for it.
	p.UnlockAll()
	p.AddMetal(5000)
	build(sim.Pylon, free())
	for bt := sim.WaterExtractor; bt < sim.NumBuildingTypes; bt++ {
		c := free()
		if bt == sim.Comet || bt == sim.Pylon {
			continue
		}
		t := &p.Tiles[g.Index(c)]
		switch sim.Specs[bt].Site {
		case sim.WaterSite:
			t.Ice = 1
		case sim.VolcanicSite:
			t.Volcanic = true
		}
		build(bt, c)
	}
}
