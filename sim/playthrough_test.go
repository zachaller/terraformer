package sim

import (
	"math"
	"testing"
)

// bot plays the game with simple rules, to check the goals can all be
// reached without shortcuts and to measure pacing.
type bot struct {
	t      *testing.T
	p      *Planet
	lander Cell
}

// nearest returns the free tile closest to from, within maxDist tiles, that
// satisfies ok.
func (b *bot) nearest(from Cell, maxDist float64, ok func(c Cell, t Tile) bool) (Cell, bool) {
	p := b.p
	best, bestD := Cell{}, math.Inf(1)
	for idx, t := range p.Tiles {
		c := p.Grid.CellAt(idx)
		if p.eco.at[idx] != nil || !ok(c, t) {
			continue
		}
		if d := p.LinkLength(from, c); d <= maxDist && d < bestD {
			best, bestD = c, d
		}
	}
	return best, bestD < math.Inf(1)
}

// distToType is the distance from c to the nearest building of any of the
// given types.
func (b *bot) distToType(c Cell, types ...BuildingType) float64 {
	d := math.Inf(1)
	for _, bl := range b.p.eco.buildings {
		for _, t := range types {
			if bl.Type == t {
				d = math.Min(d, b.p.LinkLength(c, b.p.Grid.CellAt(bl.Tile)))
			}
		}
	}
	return d
}

// linkToNetwork links c to the closest building in the lander's network.
func (b *bot) linkToNetwork(c Cell) bool {
	p := b.p
	net := p.NetworkOf(p.BuildingAt(b.lander))
	var best Cell
	bestD := math.Inf(1)
	for _, m := range net.Members {
		mc := p.Grid.CellAt(m.Tile)
		if mc == c {
			continue
		}
		if d := p.LinkLength(c, mc); d < bestD {
			best, bestD = mc, d
		}
	}
	return bestD < math.Inf(1) && p.Link(c, best) == nil
}

func (b *bot) build(t BuildingType, c Cell) bool {
	if _, err := b.p.Build(t, c); err != nil {
		return false
	}
	if !b.linkToNetwork(c) {
		_ = b.p.Demolish(c)
		return false
	}
	return true
}

func (b *bot) land() {
	p := b.p
	// The ore tile nearest the equator, then the nearest ore-free tile to it.
	ore, ok := b.nearest(p.Grid.CellFromDir(Vec3{X: 1}), 100, func(c Cell, t Tile) bool {
		return t.Ore > 0 && math.Abs(p.Grid.DirFromCell(c).Y) < 0.3
	})
	if !ok {
		b.t.Fatal("no ore near the equator")
	}
	b.lander, _ = b.nearest(ore, 3, func(c Cell, t Tile) bool { return t.Ore == 0 })
	if err := p.Land(b.lander); err != nil {
		b.t.Fatal(err)
	}
}

// industryCount counts buildings of type t.
func (b *bot) count(t BuildingType) int {
	n := 0
	for _, bl := range b.p.eco.buildings {
		if bl.Type == t {
			n++
		}
	}
	return n
}

func (b *bot) metal() float64 { return b.p.Stores()[Metal] }

// act makes one round of decisions.
func (b *bot) act() {
	p := b.p
	// Replace exhausted mines.
	for _, bl := range append([]*Building(nil), p.eco.buildings...) {
		if bl.Type == Mine && p.Tiles[bl.Tile].Ore == 0 {
			_ = p.Demolish(p.Grid.CellAt(bl.Tile))
		}
	}
	// Keep one mine per smelter.
	for b.count(Mine) < max(b.count(Smelter), 1) && b.metal() >= Specs[Mine].Cost+4 {
		c, ok := b.nearest(b.lander, MaxLinkLength-1, func(c Cell, t Tile) bool { return t.Ore > 0 })
		if !ok || !b.build(Mine, c) {
			break
		}
	}
	// Power: keep supply at or above demand.
	net := p.NetworkOf(p.BuildingAt(b.lander))
	for net.PowerSupply < net.PowerDemand && b.metal() >= Specs[SolarPanel].Cost+2 {
		c, ok := b.nearest(b.lander, 30, func(c Cell, t Tile) bool { return t.Ore == 0 && b.distToType(c, HeaterMachine) > 1.5 })
		if !ok || !b.build(SolarPanel, c) {
			break
		}
		net = p.NetworkOf(p.BuildingAt(b.lander))
	}
	// Industry: the first smelter as soon as possible, then up to three.
	if n := b.count(Smelter); n == 0 && b.metal() >= Specs[Smelter].Cost+2 || n < 3 && b.metal() >= 25 {
		c, ok := b.nearest(b.lander, MaxLinkLength-1, func(c Cell, t Tile) bool {
			return t.Ore == 0 && b.distToType(c, HeaterMachine) > 4
		})
		if ok {
			b.build(Smelter, c)
		}
	}
	// Heaters on icy, clean ground away from industry, spaced out.
	for b.metal() >= Specs[HeaterMachine].Cost+2*Specs[SolarPanel].Cost+4 {
		c, ok := b.nearest(b.lander, 60, func(c Cell, t Tile) bool {
			return t.Ice > 0 && t.Ore == 0 && t.Pollution < 0.02 &&
				b.distToType(c, Mine, Smelter) > 4 && b.distToType(c, HeaterMachine) > 2.5 &&
				math.Abs(p.Grid.DirFromCell(c).Y) < 0.6
		})
		if !ok || !b.build(HeaterMachine, c) {
			break
		}
		for range 2 {
			if s, ok := b.nearest(c, 2, func(_ Cell, t Tile) bool { return t.Ore == 0 }); ok {
				b.build(SolarPanel, s)
			}
		}
	}
}

// TestPlaythrough has a simple bot play from landing to the final goal. It
// fails if any goal can't be reached, or if the planet thaws implausibly
// fast (which would mean the balance has broken). Run with -v to see the
// pacing.
func TestPlaythrough(t *testing.T) {
	if testing.Short() {
		t.Skip("long-running")
	}
	p := NewPlanet(32, 1)
	b := &bot{t: t, p: p}
	b.land()
	reached := map[int]uint64{}
	const maxSteps = 30000 // 3000 days, 50 minutes at 1x
	for step := 0; step < maxSteps && p.GoalsDone() < len(Goals); step++ {
		if step%20 == 0 {
			b.act()
		}
		if testing.Verbose() && step%1000 == 0 {
			st := p.Stats()
			t.Logf("  trace day %4d: atm %5.1f kPa, avg %6.1fC, life cover %5.1f%% mean %5.1f%%, water %6.1f, metal made %5.0f",
				p.Ticks/10, p.Atmosphere, st.AvgTemp, st.LifeCover*100, st.LifeMean*100, st.Water, p.Produced()[Metal])
		}
		before := p.GoalsDone()
		p.Step()
		for g := before; g < p.GoalsDone(); g++ {
			reached[g] = p.Ticks
		}
	}
	for g := range Goals {
		if at, ok := reached[g]; ok {
			t.Logf("%-16s day %5d (%4.1f min at 1x)", Goals[g].Name, at/10, float64(at)/10/60)
		} else if g == 0 {
			t.Logf("%-16s at landing", Goals[g].Name)
		} else {
			t.Logf("%-16s not reached", Goals[g].Name)
		}
	}
	if p.GoalsDone() < len(Goals) {
		t.Errorf("bot stuck on goal %q after %d days", Goals[p.GoalsDone()].Name, p.Ticks/10)
	}
	if at, ok := reached[len(Goals)-2]; ok && at < 6000 {
		t.Errorf("planet thawed on day %d; balance is too easy (expected after day 600)", at/10)
	}
	st := p.Stats()
	t.Logf("end: day %d, %d buildings, metal %.0f, atmosphere %.1f kPa, avg %.1fC, life %.1f%%",
		p.Ticks/10, len(p.eco.buildings), b.metal(), p.Atmosphere, st.AvgTemp, st.LifeCover*100)
}
