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
	fd := p.Dir(from)
	cosMax := math.Cos(maxDist * p.Grid.TileAngle())
	for idx, t := range p.Tiles {
		if p.eco.at[idx] != nil {
			continue
		}
		dot := fd.Dot(p.dirs[idx])
		if dot < cosMax || -dot >= bestD {
			continue
		}
		c := p.Grid.CellAt(idx)
		if ok(c, t) {
			best, bestD = c, -dot
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
				d = math.Min(d, b.p.Distance(c, b.p.Grid.CellAt(bl.Tile)))
			}
		}
	}
	return d
}

// colonyHubs returns the hubs in the lander's network.
func (b *bot) colonyHubs() []Cell {
	p := b.p
	var hubs []Cell
	for _, bl := range p.eco.buildings {
		if isHub(bl.Type) && p.NetworkOf(bl).HasDepot {
			hubs = append(hubs, p.Grid.CellAt(bl.Tile))
		}
	}
	return hubs
}

// connect connects the building on c to the colony, building pylons
// (chained out from the colony if need be) as a player would.
func (b *bot) connect(c Cell) bool {
	p := b.p
	for range 30 {
		if p.NetworkOf(p.BuildingAt(c)).HasDepot {
			return true
		}
		if !b.afford(Specs[Pylon].Cost, 0) {
			return false
		}
		hubs := b.colonyHubs()
		inReach := func(x Cell) bool {
			for _, h := range hubs {
				if p.Distance(x, h) <= hubReach-0.5 {
					return true
				}
			}
			return false
		}
		// A pylon covering c and in reach of the colony, if there's room;
		// otherwise the free tile in reach of the colony nearest c, to
		// step towards it.
		pc, ok := b.nearest(c, hubCover-0.5, func(x Cell, _ Tile) bool { return inReach(x) })
		if !ok {
			pc, ok = b.nearest(c, 200, func(x Cell, _ Tile) bool { return inReach(x) })
		}
		if !ok {
			return false
		}
		if _, err := p.Build(Pylon, pc); err != nil {
			return false
		}
	}
	return false
}

// build builds t on c and connects it to the colony, or undoes it.
func (b *bot) build(t BuildingType, c Cell) bool {
	if _, err := b.p.Build(t, c); err != nil {
		return false
	}
	if !b.connect(c) {
		_ = b.p.Demolish(c)
		return false
	}
	return true
}

// outpost builds t on c with a pylon and solar panels beside it, not
// connected to the colony.
func (b *bot) outpost(t BuildingType, c Cell) bool {
	p := b.p
	need := Specs[t].PowerIn
	if _, err := p.Build(t, c); err != nil {
		return false
	}
	if p.NetworkOf(p.BuildingAt(c)).PowerSupply >= need {
		return true
	}
	if py, ok := b.nearest(c, 1.5, func(Cell, Tile) bool { return true }); !ok {
		return false
	} else if _, err := p.Build(Pylon, py); err != nil {
		return false
	}
	for need > 0 {
		s, ok := b.nearest(c, 2.5, func(Cell, Tile) bool { return true })
		if !ok {
			return false
		}
		if _, err := p.Build(SolarPanel, s); err != nil {
			return false
		}
		need -= Specs[SolarPanel].PowerOut
	}
	return true
}

func (b *bot) land() {
	p := b.p
	// The ore tile nearest the equator, then the nearest ore-free tile to it.
	ore, ok := b.nearest(p.Grid.CellFromDir(Vec3{X: 1}), 100, func(c Cell, t Tile) bool {
		return t.Ore > 0 && t.Mineral == Iron && math.Abs(p.Dir(c).Y) < 0.3
	})
	if !ok {
		b.t.Fatal("no ore near the equator")
	}
	b.lander, _ = b.nearest(ore, 3, func(c Cell, t Tile) bool { return t.Ore == 0 })
	if err := p.Land(b.lander); err != nil {
		b.t.Fatal(err)
	}
}

func (b *bot) count(t BuildingType) int { return b.p.count(t) }

// countMines counts mines on deposits of mineral m.
func (b *bot) countMines(m Mineral) int {
	n := 0
	for _, bl := range b.p.eco.buildings {
		if bl.Type == Mine && b.p.Tiles[bl.Tile].Mineral == m {
			n++
		}
	}
	return n
}

func (b *bot) metal() float64 { return b.p.Stores()[Metal] }

// afford reports whether the stores can pay cost and keep reserve.
func (b *bot) afford(cost, reserve float64) bool { return b.metal() >= cost+reserve }

// done reports whether the goal called name is complete.
func (b *bot) done(name string) bool { return b.p.goalsDone > GoalIndex(name) }

// mine builds a mine on the nearest deposit of m.
func (b *bot) mine(m Mineral) bool {
	c, ok := b.nearest(b.lander, 80, func(_ Cell, t Tile) bool { return t.Ore > 0 && t.Mineral == m })
	return ok && b.build(Mine, c)
}

// near builds t on a free tile near the colony that satisfies ok.
func (b *bot) near(t BuildingType, maxDist float64, ok func(Cell, Tile) bool) bool {
	c, found := b.nearest(b.lander, maxDist, func(c Cell, tl Tile) bool {
		return tl.Ore == 0 && ok(c, tl)
	})
	return found && b.build(t, c)
}

func anyTile(Cell, Tile) bool { return true }

// act makes one round of decisions.
func (b *bot) act() {
	p := b.p
	// Replace exhausted mines and drills.
	for _, bl := range append([]*Building(nil), p.eco.buildings...) {
		if (bl.Type == Mine && p.Tiles[bl.Tile].Ore == 0) || (bl.Type == IceDrill && p.Tiles[bl.Tile].Ice == 0) {
			_ = p.Demolish(p.Grid.CellAt(bl.Tile))
		}
	}
	// Out of iron mines and metal to build one: sell a heater.
	if b.countMines(Iron) == 0 && b.metal() < Specs[Mine].Cost+1 {
		for _, bl := range p.eco.buildings {
			if bl.Type == HeaterMachine {
				_ = p.Demolish(p.Grid.CellAt(bl.Tile))
				break
			}
		}
	}
	// Save up for the magnetic shield once it's available.
	reserve := 0.0
	if p.Unlocked(MagneticShield) && b.count(MagneticShield) == 0 {
		reserve = Specs[MagneticShield].Cost
	}

	// Industry: one iron mine per smelter, more smelters as the game goes on.
	smelters := 3
	if b.done("Thicken the air") {
		smelters = 8
	}
	for n := b.countMines(Iron); n < max(b.count(Smelter), 1) && b.afford(Specs[Mine].Cost+1, 3*boolF(n > 0)); n = b.countMines(Iron) {
		if !b.mine(Iron) {
			break
		}
	}
	if n := b.count(Smelter); n == 0 && b.afford(Specs[Smelter].Cost+2, 0) || n < smelters && b.afford(25, 0) {
		b.near(Smelter, 24, func(c Cell, _ Tile) bool { return b.distToType(c, HeaterMachine) > 4 })
	}

	// Power for the colony.
	net := p.NetworkOf(p.BuildingAt(b.lander))
	for net.PowerSupply < net.PowerDemand && b.afford(Specs[SolarPanel].Cost+2, 0) {
		t := SolarPanel
		if p.Unlocked(WindTurbine) && p.WindFactor() > 0.5 && b.afford(Specs[WindTurbine].Cost+2, 0) {
			t = WindTurbine
		}
		if !b.near(t, 30, func(c Cell, _ Tile) bool { return b.distToType(c, HeaterMachine) > 1.5 }) {
			break
		}
		net = p.NetworkOf(p.BuildingAt(b.lander))
	}

	// The magnetic shield, as soon as it can be afforded.
	if reserve > 0 && b.afford(Specs[MagneticShield].Cost, 0) {
		b.near(MagneticShield, 20, anyTile)
	}

	// Dust the polar caps with outposts.
	if p.Unlocked(DustSpreader) && b.count(DustSpreader) < 8 && p.Stats().DryIce > 0.05*p.startDryIce && b.afford(Specs[DustSpreader].Cost+Specs[SolarPanel].Cost+4, reserve) {
		if c, ok := b.nearest(b.lander, 200, func(c Cell, t Tile) bool {
			return t.DryIce > 0.2 && b.distToType(c, DustSpreader) > 5
		}); ok {
			b.outpost(DustSpreader, c)
		}
	}
	// Orbital mirrors on what's left of the caps.
	if p.Unlocked(OrbitalMirror) && b.count(OrbitalMirror) < 2 && p.Stats().DryIce > 0.05*p.startDryIce && b.afford(Specs[OrbitalMirror].Cost, reserve) {
		if c, ok := b.nearest(b.lander, 200, func(c Cell, t Tile) bool {
			return t.DryIce > 0.3 && b.distToType(c, OrbitalMirror) > 6
		}); ok {
			p.Build(OrbitalMirror, c)
		}
	}

	// Greenhouse gases: fluorite mines and factories. More late in the
	// game, when CO₂ is drawn down and the planet needs keeping warm.
	if p.Unlocked(GreenhouseFactory) {
		// Keep the planet comfortably warm: more factories when it's cool
		// (especially as plants draw CO₂ down), none past 20 °C.
		avg := p.Stats().AvgTemp
		factories := 2
		switch {
		case avg > 20:
			factories = 0
		case b.done("Thaw") && avg < 12:
			factories = 2 + int((12-avg)/2)
		}
		if n := b.count(GreenhouseFactory); n > factories {
			for _, bl := range p.eco.buildings {
				if bl.Type == GreenhouseFactory {
					_ = p.Demolish(p.Grid.CellAt(bl.Tile))
					break
				}
			}
		}
		if b.count(GreenhouseFactory) < factories && b.afford(Specs[GreenhouseFactory].Cost+5, reserve) {
			if b.countMines(Fluorite) < (b.count(GreenhouseFactory)+2)/2 {
				b.mine(Fluorite)
			} else {
				b.near(GreenhouseFactory, 30, anyTile)
			}
		}
	}

	// A biolab where life is starting.
	if p.Unlocked(Biolab) && b.count(Biolab) == 0 && b.afford(Specs[Biolab].Cost+5, reserve) {
		if c, ok := b.nearest(b.lander, 20, func(_ Cell, t Tile) bool { return t.Life > 0.2 && t.Ore == 0 }); ok {
			b.build(Biolab, c)
		}
	}

	// Oxygen: ice drills feeding electrolysers.
	if p.Unlocked(Electrolyser) && b.done("Super greenhouse") {
		pairs := 2
		if b.done("Shield the air") {
			pairs = 4
		}
		if b.count(Electrolyser) < pairs && b.afford(Specs[Electrolyser].Cost+Specs[IceDrill].Cost+5, reserve) {
			if b.count(IceDrill) <= b.count(Electrolyser) {
				b.near(IceDrill, 40, func(_ Cell, t Tile) bool { return t.Ice > 0.3 })
			} else {
				b.near(Electrolyser, 30, anyTile)
			}
		}
	}

	// Nitrogen: nitrate mines and nitrogen plants, and comets.
	if p.Unlocked(NitrogenPlant) && p.Air.N2 < 12 {
		if b.count(NitrogenPlant) < 4 && b.afford(Specs[NitrogenPlant].Cost+5, reserve) {
			if b.countMines(Nitrate) < (b.count(NitrogenPlant)+1+1)/2 {
				b.mine(Nitrate)
			} else {
				b.near(NitrogenPlant, 30, anyTile)
			}
		}
	}
	if p.Unlocked(Comet) && b.count(Comet) == 0 && p.Air.N2 < 12 && b.afford(Specs[Comet].Cost, reserve) {
		if c, ok := b.nearest(b.lander, 200, func(c Cell, t Tile) bool {
			return t.Kind() == Basin && p.nearestBuilding(p.Grid.Index(c)) > cometClearance+2
		}); ok {
			p.Build(Comet, c)
		}
	}

	// Carbon scrubbers for the last push to breathable air.
	if p.Unlocked(CarbonScrubber) && p.Air.CO2 > 1 && p.Air.O2 > 14 && b.count(CarbonScrubber) < 6 && b.afford(Specs[CarbonScrubber].Cost+5, reserve) {
		b.near(CarbonScrubber, 30, anyTile)
	}

	// Heaters on icy, clean ground away from industry, spaced out.
	heaters := 14
	if b.done("Thaw") {
		heaters = 0 // the planet is warm enough by itself
	}
	for b.count(HeaterMachine) < heaters && b.afford(Specs[HeaterMachine].Cost+2*Specs[SolarPanel].Cost+Specs[Mine].Cost+1, reserve) {
		c, ok := b.nearest(b.lander, 60, func(c Cell, t Tile) bool {
			return t.Ice > 0 && t.Ore == 0 && t.Pollution < 0.02 &&
				b.distToType(c, Mine, Smelter, GreenhouseFactory) > 4 && b.distToType(c, HeaterMachine) > 2.5 &&
				math.Abs(p.Dir(c).Y) < 0.6
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

func boolF(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// TestPlaythrough has a simple bot play from landing to the final goal. It
// fails if any goal can't be reached, or if the game goes implausibly fast
// (which would mean the balance has broken). Run with -v to see the pacing.
func TestPlaythrough(t *testing.T) {
	if testing.Short() {
		t.Skip("long-running")
	}
	p := NewPlanet(25, 1)
	b := &bot{t: t, p: p}
	b.land()
	reached := map[int]uint64{}
	const maxSteps = 250000 // 25,000 days, about 7 hours at 1x
	for step := 0; step < maxSteps && p.GoalsDone() < len(Goals); step++ {
		if step%20 == 0 {
			b.act()
		}
		if testing.Verbose() && step%5000 == 0 {
			st := p.Stats()
			t.Logf("  trace day %5d: %5.1f kPa (CO2 %5.1f N2 %4.1f O2 %5.2f PFC %4.2f) avg %6.1fC caps %3.0f%% water %5.0f sea %4.1f%% life %5.1f%% bldg %3d (fac %d scr %d) metal %4.0f goal %q",
				p.Ticks/10, p.Pressure(), p.Air.CO2, p.Air.N2, p.Air.O2, p.Air.PFC, st.AvgTemp, st.DryIce/p.startDryIce*100, st.Water, st.SeaCover*100, st.LifeCover*100, len(p.eco.buildings), b.count(GreenhouseFactory), b.count(CarbonScrubber), b.metal(), p.GoalSummary())
		}
		before := p.GoalsDone()
		p.Step()
		for g := before; g < p.GoalsDone(); g++ {
			reached[g] = p.Ticks
		}
	}
	for g := range Goals {
		if at, ok := reached[g]; ok {
			t.Logf("%-22s day %5d (%5.1f h at 1x)", Goals[g].Name, at/10, float64(at)/10/3600)
		} else if g == 0 {
			t.Logf("%-22s at landing", Goals[g].Name)
		} else {
			t.Logf("%-22s not reached", Goals[g].Name)
		}
	}
	if p.GoalsDone() < len(Goals) {
		t.Errorf("bot stuck on goal %q after %d days", Goals[p.GoalsDone()].Name, p.Ticks/10)
	}
	// The bot plays fast; people take several times as long. Below two
	// hours for the bot, the game is too short.
	if at, ok := reached[len(Goals)-1]; ok && at < 2*3600*10 {
		t.Errorf("the bot finished on day %d; the game is too short (expected after day 7200)", at/10)
	}
	st := p.Stats()
	t.Logf("end: day %d, %d buildings, metal %.0f, %.1f kPa, avg %.1fC, life %.1f%%",
		p.Ticks/10, len(p.eco.buildings), b.metal(), p.Pressure(), st.AvgTemp, st.LifeCover*100)
}
