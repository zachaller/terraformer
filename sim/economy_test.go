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

func mustLink(t *testing.T, p *Planet, a, b Cell) {
	t.Helper()
	if err := p.Link(a, b); err != nil {
		t.Fatalf("Link(%v, %v): %v", a, b, err)
	}
}

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

func TestLinkRules(t *testing.T) {
	p, lander, ore := economyPlanet(t)
	mustBuild(t, p, Mine, ore)
	if err := p.Link(lander, Cell{Diamond: 4, I: 3, J: 3}); !errors.Is(err, ErrNoBuilding) {
		t.Fatalf("link to empty tile: got %v", err)
	}
	if err := p.Link(lander, lander); !errors.Is(err, ErrSameBuilding) {
		t.Fatalf("self link: got %v", err)
	}
	before := p.Stores()[Metal]
	mustLink(t, p, lander, ore)
	if got, want := p.Stores()[Metal], before-p.LinkCost(lander, ore); got != want {
		t.Fatalf("metal after link = %v, want %v", got, want)
	}
	if err := p.Link(ore, lander); !errors.Is(err, ErrAlreadyLinked) {
		t.Fatalf("duplicate link: got %v", err)
	}
	far := Cell{Diamond: 7, I: 8, J: 8} // other side of the planet
	mustBuild(t, p, SolarPanel, far)
	if err := p.Link(lander, far); err == nil {
		t.Fatal("linked across the planet")
	}
}

func TestNetworksFollowLinks(t *testing.T) {
	p, lander, ore := economyPlanet(t)
	mine := mustBuild(t, p, Mine, ore)
	solar := mustBuild(t, p, SolarPanel, Cell{Diamond: 4, I: 8, J: 10})
	if p.NetworkOf(mine) == p.NetworkOf(p.BuildingAt(lander)) {
		t.Fatal("unlinked buildings share a network")
	}
	mustLink(t, p, lander, ore)
	if p.NetworkOf(mine) != p.NetworkOf(p.BuildingAt(lander)) {
		t.Fatal("linked buildings are in different networks")
	}
	if p.NetworkOf(solar) == p.NetworkOf(mine) {
		t.Fatal("unlinked solar joined the network")
	}
	if err := p.Demolish(ore); err != nil {
		t.Fatal(err)
	}
	if len(p.Links()) != 0 {
		t.Fatal("demolishing a building left its link behind")
	}
	if err := p.Demolish(lander); !errors.Is(err, ErrLander) {
		t.Fatalf("demolishing the lander: got %v", err)
	}
}

func TestProductionChainFillsStores(t *testing.T) {
	// mine -> smelter, both linked to the lander: ore goes to the stores, the
	// smelter draws it back, and metal comes out.
	p, lander, ore := economyPlanet(t)
	smelterCell := Cell{Diamond: 4, I: 8, J: 10}
	mustBuild(t, p, Mine, ore)
	mustBuild(t, p, Smelter, smelterCell)
	mustLink(t, p, lander, ore)
	mustLink(t, p, lander, smelterCell)
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
	// A mine and solar panel linked to each other but not the lander: ore
	// fills the mine's buffer and then mining stops.
	p, _, ore := economyPlanet(t)
	solarCell := Cell{Diamond: 4, I: 10, J: 10}
	mine := mustBuild(t, p, Mine, ore)
	mustBuild(t, p, SolarPanel, solarCell)
	mustLink(t, p, ore, solarCell)
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
	p, lander, ore := economyPlanet(t)
	heaterCell := Cell{Diamond: 4, I: 6, J: 8}
	heater := mustBuild(t, p, HeaterMachine, heaterCell)
	mine := mustBuild(t, p, Mine, ore)
	mustLink(t, p, lander, heaterCell)
	mustLink(t, p, lander, ore)
	p.Step()
	if want := 0.75; math.Abs(heater.Activity-want) > 1e-9 || math.Abs(mine.Activity-want) > 1e-9 {
		t.Fatalf("activity heater %v mine %v, want %v", heater.Activity, mine.Activity, want)
	}
}

func TestUnpoweredMachineIdles(t *testing.T) {
	p, _, _ := economyPlanet(t)
	heater := mustBuild(t, p, HeaterMachine, Cell{Diamond: 4, I: 6, J: 8})
	p.Step()
	if heater.Activity != 0 {
		t.Fatalf("unlinked heater ran: activity %v", heater.Activity)
	}
}

func TestHeaterWarmsTile(t *testing.T) {
	p, lander, _ := economyPlanet(t)
	heaterCell := Cell{Diamond: 4, I: 6, J: 8}
	mustBuild(t, p, HeaterMachine, heaterCell)
	mustLink(t, p, lander, heaterCell)
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

	p, lander, ore := economyPlanet(t)
	mustBuild(t, p, Mine, ore)
	mustLink(t, p, lander, ore)
	for range 500 {
		p.Step()
	}
	if got := p.Tile(ore).Pollution; got < maxLifePollution {
		t.Fatalf("mine tile pollution = %v, want above %v", got, maxLifePollution)
	}
}

func TestEconomyDeterministic(t *testing.T) {
	run := func() *Planet {
		p, lander, ore := economyPlanet(t)
		smelterCell := Cell{Diamond: 4, I: 8, J: 10}
		heaterCell := Cell{Diamond: 4, I: 6, J: 8}
		mustBuild(t, p, Mine, ore)
		mustBuild(t, p, Smelter, smelterCell)
		mustBuild(t, p, HeaterMachine, heaterCell)
		mustLink(t, p, lander, ore)
		mustLink(t, p, lander, smelterCell)
		mustLink(t, p, lander, heaterCell)
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

func TestPylons(t *testing.T) {
	p, lander, _ := economyPlanet(t)
	p.eco.Stores[Metal] = 1000
	unlockAll(p)
	far := Cell{Diamond: 4, I: 8, J: 1} // 7 lattice steps from the lander
	py := Cell{Diamond: 4, I: 8, J: 3}
	mustBuild(t, p, SolarPanel, far)
	if p.NetworkOf(p.BuildingAt(far)).HasDepot {
		t.Fatal("unlinked panel is in the lander's network")
	}
	// A pylon covers the panel, and links to the lander.
	if _, _, covered := p.AutoLink(Pylon, py); !covered {
		t.Fatal("a new pylon next to a panel isn't counted as connected")
	}
	mustBuild(t, p, Pylon, py)
	mustLink(t, p, py, lander)
	if !p.NetworkOf(p.BuildingAt(far)).HasDepot {
		t.Fatal("the pylon didn't connect the panel to the lander")
	}
	// Buildings near a pylon need no link.
	if _, _, covered := p.AutoLink(Smelter, Cell{Diamond: 4, I: 9, J: 3}); !covered {
		t.Error("a tile next to a pylon isn't covered")
	}
}

func TestAutoLinkPrefersColony(t *testing.T) {
	p, lander, _ := economyPlanet(t)
	p.eco.Stores[Metal] = 1000
	lone := Cell{Diamond: 4, I: 12, J: 8}
	mustBuild(t, p, SolarPanel, lone)
	// From a tile nearer the lone panel than the lander, still link to the
	// lander's network.
	c := Cell{Diamond: 4, I: 11, J: 8}
	target, ok, covered := p.AutoLink(Smelter, c)
	if !ok || covered || target != lander {
		t.Errorf("AutoLink = %v, %v, %v; want the lander", target, ok, covered)
	}
	// Out of everyone's reach: nothing.
	if _, ok, covered := p.AutoLink(Smelter, Cell{Diamond: 7, I: 8, J: 8}); ok || covered {
		t.Error("AutoLink found a link from the far side of the planet")
	}
}
