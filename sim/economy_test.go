package sim

import (
	"errors"
	"math"
	"testing"
)

// economyPlanet returns a planet with the lander at the centre of face 4 and
// an ore deposit two tiles east of it.
func economyPlanet(t *testing.T) (p *Planet, lander, ore Cell) {
	t.Helper()
	p = NewPlanet(16, 1)
	lander = Cell{Diamond: 4, I: 8, J: 8}
	ore = Cell{Diamond: 4, I: 10, J: 8}
	for i := range p.Tiles {
		p.Tiles[i].Ore = 0
	}
	p.Tiles[p.Grid.Index(ore)].Ore = 100
	if err := p.Land(lander); err != nil {
		t.Fatal(err)
	}
	return p, lander, ore
}

func mustBuild(t *testing.T, p *Planet, bt BuildingType, c Cell) *Building {
	t.Helper()
	b, err := p.Build(bt, c)
	if err != nil {
		t.Fatalf("Build(%v, %v): %v", bt, c, err)
	}
	return b
}

// remote returns a tile far from the colony, with an iron deposit, where
// buildings aren't connected to the lander.
func remote(p *Planet) Cell {
	c := Cell{Diamond: 1, I: 8, J: 8}
	p.Tiles[p.Grid.Index(c)].Ore = 100
	p.Tiles[p.Grid.Index(c)].Mineral = Iron
	return c
}

// near returns the tile i steps along the lattice from c, in the same
// diamond.
func near(c Cell, i int) Cell { return Cell{Diamond: c.Diamond, I: c.I + i, J: c.J} }

func TestBuildRules(t *testing.T) {
	p := NewPlanet(16, 1)
	c := Cell{Diamond: 4, I: 8, J: 8}
	if _, err := p.Build(SolarPanel, c); !errors.Is(err, ErrNotLanded) {
		t.Fatalf("building before landing: got %v, want ErrNotLanded", err)
	}
	p, lander, ore := economyPlanet(t)
	if got := p.Stores()[Metal]; got != startingMetal {
		t.Fatalf("starting metal = %v, want %v", got, startingMetal)
	}
	if err := p.Land(ore); !errors.Is(err, ErrAlreadyLanded) {
		t.Fatalf("second landing: got %v", err)
	}
	if _, err := p.Build(SolarPanel, lander); !errors.Is(err, ErrOccupied) {
		t.Fatalf("building on the lander: got %v", err)
	}
	if _, err := p.Build(Mine, Cell{Diamond: 4, I: 8, J: 10}); !errors.Is(err, ErrNoOre) {
		t.Fatalf("mine off a deposit: got %v", err)
	}
	mustBuild(t, p, Mine, ore)
	if got := p.Stores()[Metal]; got != startingMetal-Specs[Mine].Cost {
		t.Fatalf("metal after building a mine = %v", got)
	}
	// Spend the rest, then check we can't overspend.
	for i := 0; p.Stores()[Metal] >= Specs[SolarPanel].Cost; i++ {
		mustBuild(t, p, SolarPanel, Cell{Diamond: 4, I: i, J: 2})
	}
	if _, err := p.Build(SolarPanel, Cell{Diamond: 4, I: 0, J: 4}); err == nil {
		t.Fatal("built with too little metal")
	}
}

func TestHubsConnect(t *testing.T) {
	p, lander, ore := economyPlanet(t)
	p.eco.Stores[Metal] = 1000
	unlockAll(p)
	// Within the lander's cover: connected without anything else.
	mine := mustBuild(t, p, Mine, ore)
	if p.NetworkOf(mine) != p.NetworkOf(p.BuildingAt(lander)) {
		t.Fatal("a mine two tiles from the lander isn't connected")
	}
	// Far away: on its own.
	r := remote(p)
	solar := mustBuild(t, p, SolarPanel, r)
	if p.NetworkOf(solar).HasDepot {
		t.Fatal("a panel across the planet is in the lander's network")
	}
	// A pylon next to it connects it to nothing else...
	py := near(r, 1)
	mustBuild(t, p, Pylon, py)
	if p.NetworkOf(solar) != p.NetworkOf(p.BuildingAt(py)) || p.NetworkOf(solar).HasDepot {
		t.Fatal("a remote pylon didn't connect only its neighbour")
	}
	if len(p.Spokes()) == 0 {
		t.Fatal("no spokes from the pylons")
	}
	if err := p.Demolish(lander); !errors.Is(err, ErrLander) {
		t.Fatalf("demolishing the lander: got %v", err)
	}
}

func TestPylonChain(t *testing.T) {
	p, lander, _ := economyPlanet(t)
	p.eco.Stores[Metal] = 1000
	// A panel 7 tiles out is beyond the lander's cover...
	far := Cell{Diamond: 4, I: 8, J: 1}
	solar := mustBuild(t, p, SolarPanel, far)
	if p.NetworkOf(solar).HasDepot {
		t.Fatal("a panel 7 tiles out is connected")
	}
	// ...until a pylon within reach of the lander covers it.
	py := Cell{Diamond: 4, I: 8, J: 3}
	if _, ok, colony, _ := p.Connection(Pylon, py); !ok || !colony {
		t.Fatal("a pylon 5 tiles from the lander wouldn't connect to it")
	}
	mustBuild(t, p, Pylon, py)
	if !p.NetworkOf(solar).HasDepot {
		t.Fatal("the pylon didn't connect the panel to the colony")
	}
	if len(p.HubLinks()) != 1 {
		t.Fatalf("hub links = %v, want just pylon–lander", p.HubLinks())
	}
	via, ok, colony, _ := p.Connection(Smelter, Cell{Diamond: 4, I: 9, J: 2})
	if !ok || !colony || via != py {
		t.Errorf("Connection = %v, %v, %v; want the pylon", via, ok, colony)
	}
	_ = lander
}

func TestProductionChainFillsStores(t *testing.T) {
	// mine -> smelter, both linked to the lander: ore goes to the stores, the
	// smelter draws it back, and metal comes out.
	p, lander, ore := economyPlanet(t)
	smelterCell := Cell{Diamond: 4, I: 8, J: 10}
	mustBuild(t, p, Mine, ore)
	mustBuild(t, p, Smelter, smelterCell)
	metal := p.Stores()[Metal]
	for range 200 {
		p.Step()
	}
	if got := p.Stores()[Metal]; got <= metal {
		t.Fatalf("metal didn't grow: %v -> %v", metal, got)
	}
	if got := p.Tile(ore).Ore; got >= 100 {
		t.Fatalf("deposit not mined: %v", got)
	}
	// Lander power (3) exactly covers mine (1) + smelter (2).
	if s := p.NetworkOf(p.BuildingAt(lander)).PowerSatisfaction(); s != 1 {
		t.Fatalf("power satisfaction = %v, want 1", s)
	}
}

func TestOutputWithoutDepotStalls(t *testing.T) {
	// A mine, solar panel and pylon far from the lander: ore fills the
	// mine's buffer and then mining stops.
	p, _, _ := economyPlanet(t)
	p.eco.Stores[Metal] = 1000
	ore := remote(p)
	mine := mustBuild(t, p, Mine, ore)
	mustBuild(t, p, SolarPanel, near(ore, 1))
	mustBuild(t, p, Pylon, near(ore, -1))
	for range 200 {
		p.Step()
	}
	if mine.Out[Ore] != bufferCap {
		t.Fatalf("mine output buffer = %v, want full (%v)", mine.Out[Ore], bufferCap)
	}
	if mine.Activity != 0 {
		t.Fatalf("mine still running with a full buffer: activity %v", mine.Activity)
	}
	if got := 100 - p.Tile(ore).Ore; math.Abs(got-bufferCap) > 1e-9 {
		t.Fatalf("mined %v ore, want exactly the buffer's worth", got)
	}
}

func TestPowerShortageSlowsMachines(t *testing.T) {
	// Lander gives 3 power; a heater (3) plus a mine (1) want 4.
	p, _, ore := economyPlanet(t)
	heaterCell := Cell{Diamond: 4, I: 6, J: 8}
	heater := mustBuild(t, p, HeaterMachine, heaterCell)
	mine := mustBuild(t, p, Mine, ore)
	p.Step()
	if want := 0.75; math.Abs(heater.Activity-want) > 1e-9 || math.Abs(mine.Activity-want) > 1e-9 {
		t.Fatalf("activity heater %v mine %v, want %v", heater.Activity, mine.Activity, want)
	}
}

func TestUnpoweredMachineIdles(t *testing.T) {
	p, _, _ := economyPlanet(t)
	heater := mustBuild(t, p, HeaterMachine, remote(p))
	p.Step()
	if heater.Activity != 0 {
		t.Fatalf("unlinked heater ran: activity %v", heater.Activity)
	}
}

func TestHeaterWarmsTile(t *testing.T) {
	p, _, _ := economyPlanet(t)
	heaterCell := Cell{Diamond: 4, I: 6, J: 8}
	mustBuild(t, p, HeaterMachine, heaterCell)
	before := p.Tile(heaterCell).Temp
	for range 300 {
		p.Step()
	}
	if got := p.Tile(heaterCell).Temp; got < before+25 {
		t.Fatalf("heater warmed its tile from %.1f to only %.1f°C", before, got)
	}
}

func TestPollutionBlocksLife(t *testing.T) {
	tile := Tile{Temp: 20, Water: 0.5}
	if !Habitable(tile) {
		t.Fatal("clean warm wet tile not habitable")
	}
	tile.Pollution = 1
	if Habitable(tile) {
		t.Fatal("heavily polluted tile is habitable")
	}

	p, _, ore := economyPlanet(t)
	mustBuild(t, p, Mine, ore)
	for range 500 {
		p.Step()
	}
	if got := p.Tile(ore).Pollution; got < maxLifePollution {
		t.Fatalf("mine tile pollution = %v, want above %v", got, maxLifePollution)
	}
}

func TestEconomyDeterministic(t *testing.T) {
	run := func() *Planet {
		p, _, ore := economyPlanet(t)
		smelterCell := Cell{Diamond: 4, I: 8, J: 10}
		heaterCell := Cell{Diamond: 4, I: 6, J: 8}
		mustBuild(t, p, Mine, ore)
		mustBuild(t, p, Smelter, smelterCell)
		mustBuild(t, p, HeaterMachine, heaterCell)
		for range 1000 {
			p.Step()
		}
		return p
	}
	a, b := run(), run()
	for i := range a.Tiles {
		if a.Tiles[i] != b.Tiles[i] {
			t.Fatalf("tile %d differs after identical runs", i)
		}
	}
	if a.Stores() != b.Stores() {
		t.Fatalf("stores differ: %v vs %v", a.Stores(), b.Stores())
	}
}

func TestOreDepositsExist(t *testing.T) {
	p := NewPlanet(32, 7)
	n := 0
	for _, tile := range p.Tiles {
		if tile.Ore > 0 {
			n++
		}
	}
	if frac := float64(n) / float64(len(p.Tiles)); frac < 0.03 || frac > 0.25 {
		t.Fatalf("ore on %.1f%% of tiles, want roughly a tenth", frac*100)
	}
}

func TestHubCapacity(t *testing.T) {
	p, _, _ := economyPlanet(t)
	p.eco.Stores[Metal] = 1000
	c := remote(p)
	py := mustBuild(t, p, Pylon, c)
	// Fill the pylon with panels next to it: each takes about 1 load.
	var panels []*Building
	for _, idx := range p.ringsAround(p.Grid.Index(c), 2)[1] {
		panels = append(panels, mustBuild(t, p, SolarPanel, p.Grid.CellAt(idx)))
	}
	for _, idx := range p.ringsAround(p.Grid.Index(c), 2)[2] {
		panels = append(panels, mustBuild(t, p, SolarPanel, p.Grid.CellAt(idx)))
	}
	load, capacity := p.HubLoad(py)
	if load > capacity+1e-9 {
		t.Fatalf("pylon load %.1f over capacity %.1f", load, capacity)
	}
	connected, full := 0, 0
	for _, b := range panels {
		if p.NetworkOf(b) == p.NetworkOf(py) {
			connected++
		} else if p.HubsFull(b) {
			full++
		}
	}
	if connected == 0 || full == 0 || connected+full != len(panels) {
		t.Fatalf("%d panels connected, %d left out as full, of %d", connected, full, len(panels))
	}
	// The nearest ring is connected first.
	for _, b := range panels[:6] {
		if p.NetworkOf(b) != p.NetworkOf(py) {
			t.Fatal("a panel next to the pylon was left out for a further one")
		}
	}
	// Another pylon picks up the rest.
	if _, ok, _, full := p.Connection(SolarPanel, p.Grid.CellAt(panels[len(panels)-1].Tile)); ok || !full {
		t.Error("Connection doesn't report the pylon as full")
	}
}

// TestMineWorksPatch checks a mine moves on to the deposit tiles next to it,
// of its own mineral, once its own tile is dug out, and only counts as empty
// when the whole patch is.
func TestMineWorksPatch(t *testing.T) {
	p, _, ore := economyPlanet(t)
	next := near(ore, 1)
	other := Cell{Diamond: 4, I: 10, J: 9}
	oi, ni, xi := p.Grid.Index(ore), p.Grid.Index(next), p.Grid.Index(other)
	p.Tiles[oi].Ore, p.Tiles[oi].Mineral = 0.3, Iron
	p.Tiles[ni].Ore, p.Tiles[ni].Mineral = 0.3, Iron
	p.Tiles[xi].Ore, p.Tiles[xi].Mineral = 5, Fluorite
	if got := p.PatchOre(ore); math.Abs(got-0.6) > 1e-9 {
		t.Fatalf("PatchOre = %v, want 0.6 (the iron only)", got)
	}
	m := mustBuild(t, p, Mine, ore)
	for range 40 {
		p.Step()
	}
	if p.Tiles[ni].Ore > 0 {
		t.Errorf("neighbouring iron not dug: %v left", p.Tiles[ni].Ore)
	}
	if p.Tiles[xi].Ore != 5 {
		t.Errorf("fluorite next door was dug: %v left", p.Tiles[xi].Ore)
	}
	if got := p.Diagnose(m).Kind; got != StatusDepleted {
		t.Errorf("dug-out patch: %v, want Depleted", got)
	}
}

// TestExtractorPumpsSea checks a water extractor can go on a sea with no
// ice and pumps its water into the stores, and can't go on dry, ice-free
// ground.
func TestExtractorPumpsSea(t *testing.T) {
	p, lander, _ := economyPlanet(t)
	p.goalsDone = GoalIndex("Warm the ground") + 1
	sea := Cell{Diamond: lander.Diamond, I: lander.I + 1, J: lander.J + 1}
	dry := Cell{Diamond: lander.Diamond, I: lander.I - 1, J: lander.J + 1}
	for _, c := range []Cell{sea, dry} {
		tl := &p.Tiles[p.Grid.Index(c)]
		tl.Ice, tl.Water = 0, 0
	}
	p.Tiles[p.Grid.Index(sea)].Water = 0.5
	if _, err := p.Build(WaterExtractor, dry); !errors.Is(err, ErrNoWater) {
		t.Fatalf("extractor on dry ground: got %v, want ErrNoWater", err)
	}
	p.eco.Stores[Metal] += 50
	mustBuild(t, p, WaterExtractor, sea)
	before := p.Stores()[Water]
	for range 20 {
		p.Step()
	}
	if got := p.Stores()[Water]; got <= before {
		t.Errorf("stores water %v, want more than %v", got, before)
	}
	if got := p.Tiles[p.Grid.Index(sea)].Water; got >= 0.5 {
		t.Errorf("sea still %v deep, want pumped below 0.5", got)
	}
}
