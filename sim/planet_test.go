package sim

import (
	"math"
	"testing"
)

func TestNewPlanetDeterministic(t *testing.T) {
	a, b := NewPlanet(16, 42), NewPlanet(16, 42)
	for i := range a.Tiles {
		if a.Tiles[i] != b.Tiles[i] {
			t.Fatalf("tile %d differs between runs with the same seed", i)
		}
	}
}

func TestNewPlanetSeedsDiffer(t *testing.T) {
	a, b := NewPlanet(16, 1), NewPlanet(16, 2)
	same := 0
	for i := range a.Tiles {
		if a.Tiles[i] == b.Tiles[i] {
			same++
		}
	}
	if same == len(a.Tiles) {
		t.Fatal("different seeds produced identical planets")
	}
}

func TestNewPlanetHasVariety(t *testing.T) {
	p := NewPlanet(32, 7)
	counts := map[TileKind]int{}
	for _, tile := range p.Tiles {
		counts[tile.Kind()]++
	}
	for _, k := range []TileKind{Basin, Plains, Highlands, Peaks} {
		if counts[k] == 0 {
			t.Errorf("no %v tiles generated", k)
		}
	}
}

func TestNewPlanetStartsFrozenAndLifeless(t *testing.T) {
	p := NewPlanet(32, 7)
	for i, tile := range p.Tiles {
		if tile.Temp >= 0 {
			t.Fatalf("tile %d starts above freezing: %.1f°C", i, tile.Temp)
		}
		if tile.Water != 0 || tile.Life != 0 {
			t.Fatalf("tile %d starts with water or life: %+v", i, tile)
		}
	}
	// Poles colder than the equator.
	pole := p.Tile(p.Grid.CellFromDir(Vec3{Y: 1}))
	equator := p.Tile(p.Grid.CellFromDir(Vec3{X: 1}))
	if pole.Temp >= equator.Temp {
		t.Fatalf("pole %.1f°C not colder than equator %.1f°C", pole.Temp, equator.Temp)
	}
}

func TestStepDeterministic(t *testing.T) {
	a, b := NewPlanet(16, 3), NewPlanet(16, 3)
	for _, p := range []*Planet{a, b} {
		p.AddHeat(Cell{Diamond: 4, I: 8, J: 8}, 80)
		for range 500 {
			p.Step()
		}
	}
	for i := range a.Tiles {
		if a.Tiles[i] != b.Tiles[i] {
			t.Fatalf("tile %d differs after identical runs", i)
		}
	}
	if a.Air != b.Air {
		t.Fatal("atmosphere differs after identical runs")
	}
}

func TestWaterConserved(t *testing.T) {
	p := NewPlanet(16, 5)
	p.Air.CO2 = 3 // thick enough for liquid water, rain and snow
	for f := range Diamonds {
		p.AddHeat(Cell{Diamond: f, I: 8, J: 8}, maxHeat)
	}
	total := func() float64 {
		s := p.Stats()
		return s.Ice + s.Water + p.Vapour
	}
	before := total()
	for range 2000 {
		p.Step()
	}
	if after := total(); math.Abs(after-before) > 1e-6*before {
		t.Fatalf("ice+water+vapour went from %v to %v", before, after)
	}
	if p.Stats().Water == 0 {
		t.Fatal("heating melted no ice")
	}
	for i, tile := range p.Tiles {
		if tile.Water < 0 || tile.Ice < 0 {
			t.Fatalf("tile %d has negative water or ice: %+v", i, tile)
		}
	}
}

func TestHeatMeltsIceAndThickensAtmosphere(t *testing.T) {
	p := NewPlanet(16, 5)
	c := p.Grid.CellFromDir(Vec3{X: 1})
	iceBefore := p.Tile(c).Ice
	p.AddHeat(c, maxHeat)
	for range 1000 {
		p.Step()
	}
	tile := p.Tile(c)
	if tile.Temp <= 0 {
		t.Fatalf("heated tile still frozen: %.1f°C", tile.Temp)
	}
	if tile.Ice >= iceBefore {
		t.Fatalf("heated tile's ice didn't melt: %v -> %v", iceBefore, tile.Ice)
	}
	if p.Pressure() <= startAir.Pressure() {
		t.Fatalf("melting released no CO₂: atmosphere %v kPa", p.Pressure())
	}
}

func TestWaterFlowsDownhill(t *testing.T) {
	p := NewPlanet(8, 1)
	a := Cell{Diamond: 4, I: 3, J: 3}
	b := Cell{Diamond: 4, I: 4, J: 3}
	ia, ib := p.Grid.Index(a), p.Grid.Index(b)
	for i := range p.Tiles {
		p.Tiles[i].Height, p.Tiles[i].Ice, p.Tiles[i].Water = 0.5, 0, 0
	}
	p.Tiles[ib].Height = -0.5
	p.Tiles[ia].Water = 1
	p.stepWater()
	if p.Tiles[ib].Water <= 0 {
		t.Fatal("no water flowed to the lower neighbour")
	}
	if p.Tiles[ia].Water >= 1 {
		t.Fatal("water didn't leave the higher tile")
	}
}

func TestLifeAppearsWhenHabitableAndDiesWhenNot(t *testing.T) {
	p := NewPlanet(8, 1)
	for i := range p.Tiles {
		p.Tiles[i].Temp, p.Tiles[i].Water = 20, 0.5
	}
	for range 2000 {
		p.stepLife()
		p.Ticks++
	}
	if cover := p.Stats().LifeCover; cover < 0.9 {
		t.Fatalf("life covers only %.0f%% of a warm, wet planet", cover*100)
	}
	for i := range p.Tiles {
		p.Tiles[i].Temp = -50
	}
	for range 200 {
		p.stepLife()
	}
	if cover := p.Stats().LifeCover; cover != 0 {
		t.Fatalf("life survives on %.0f%% of a frozen planet", cover*100)
	}
}

func TestHeatedBasinGrowsLife(t *testing.T) {
	// End to end: in air thick enough for liquid water, heating a basin
	// melts its permafrost, and microbes appear.
	p := NewPlanet(25, 7)
	p.Air.CO2 = 2
	var basin Cell
	found := false
	for idx, tile := range p.Tiles {
		c := p.Grid.CellAt(idx)
		if tile.Kind() == Basin && p.Grid.DirFromCell(c).Y < 0.3 && p.Grid.DirFromCell(c).Y > -0.3 {
			basin, found = c, true
			break
		}
	}
	if !found {
		t.Fatal("no equatorial basin on this planet")
	}
	for range 3 {
		p.AddHeat(basin, 30)
	}
	for range 3000 {
		p.Step()
	}
	if p.Stats().LifeCover == 0 {
		t.Fatal("no life appeared after heating an equatorial basin")
	}
}

func TestHeatRings(t *testing.T) {
	// The estimate is close to what a heater actually does on the planet:
	// about +30, +19, +6 and +2 °C for heat 70 (measured with AddHeat
	// over 2000 steps).
	got := (Grid{N: 25}).HeatRings(70, 3)
	for r, want := range []float64{30, 19, 6, 2} {
		if math.Abs(got[r]-want) > 0.2*want+0.5 {
			t.Errorf("ring %d: %.1f °C, want about %.0f", r, got[r], want)
		}
	}
}
