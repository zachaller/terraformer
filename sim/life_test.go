package sim

import "testing"

// lushPlanet returns a small planet with warm, damp, clean land everywhere,
// established microbes, and the given atmosphere.
func lushPlanet(air float64) *Planet {
	p := NewPlanet(6, 1) // about as many tiles around as a cube-sphere of 8
	p.Atmosphere = air
	for i := range p.Tiles {
		t := &p.Tiles[i]
		t.Temp, t.Water, t.Ice, t.Life, t.Pollution = 20, 0.1, 0, 1, 0
	}
	return p
}

func TestSuccessionOnLand(t *testing.T) {
	p := lushPlanet(100)
	for range 7000 {
		p.stepSpecies()
		p.Ticks++
	}
	for _, s := range []Species{Moss, Grass, Shrubs, Forest} {
		if c := p.SpeciesCover(s); c < 0.9 {
			t.Errorf("%v covers %.0f%% of a lush planet, want nearly all", s, c*100)
		}
	}
	if c := p.SpeciesCover(Plankton); c != 0 {
		t.Errorf("plankton grew on dry land: %.0f%%", c*100)
	}
}

func TestSuccessionNeedsAir(t *testing.T) {
	p := lushPlanet(5) // enough for moss, not grass
	for range 4000 {
		p.stepSpecies()
		p.Ticks++
	}
	if p.SpeciesCover(Moss) < 0.9 {
		t.Fatal("moss didn't spread")
	}
	if c := p.SpeciesCover(Grass); c != 0 {
		t.Fatalf("grass grew in 5 kPa air: %.0f%%", c*100)
	}
}

func TestSuccessionNeedsPredecessor(t *testing.T) {
	p := lushPlanet(100)
	for i := range p.Tiles {
		p.Tiles[i].Life = 0.2 // microbes not established
	}
	for range 2000 {
		p.stepSpecies()
		p.Ticks++
	}
	if c := p.SpeciesCover(Moss); c != 0 {
		t.Fatalf("moss grew without established microbes: %.0f%%", c*100)
	}
}

func TestSeaLife(t *testing.T) {
	p := lushPlanet(50)
	for i := range p.Tiles {
		p.Tiles[i].Water = 1
	}
	for range 4000 {
		p.stepSpecies()
		p.Ticks++
	}
	if p.SpeciesCover(Plankton) < 0.9 || p.SpeciesCover(Fish) < 0.9 {
		t.Fatalf("sea life: plankton %.0f%%, fish %.0f%%", p.SpeciesCover(Plankton)*100, p.SpeciesCover(Fish)*100)
	}
	if p.SpeciesCover(Moss) != 0 {
		t.Fatal("moss grew under water")
	}
}

func TestSpeciesDieWhenFrozen(t *testing.T) {
	p := lushPlanet(100)
	for range 3000 {
		p.stepSpecies()
		p.Ticks++
	}
	for i := range p.Tiles {
		p.Tiles[i].Temp = -30
	}
	for range 300 {
		p.stepSpecies()
	}
	for s := range NumSpecies {
		if c := p.SpeciesCover(s); c != 0 {
			t.Errorf("%v survives a deep freeze on %.0f%% of tiles", s, c*100)
		}
	}
}

func TestRequirementsAndNext(t *testing.T) {
	p := lushPlanet(0.6)
	tile := p.Tiles[0]
	s, ok := NextSpecies(tile)
	if !ok || s != Moss {
		t.Fatalf("next species on fresh land = %v, %v; want Moss", s, ok)
	}
	var air *Requirement
	for _, r := range p.Requirements(Moss, tile) {
		if r.Label == "Thick enough air" {
			air = &r
		}
	}
	if air == nil || air.OK {
		t.Fatalf("moss air requirement at 0.6 kPa: %+v, want unmet", air)
	}
	tile.Water = 1
	if s, _ := NextSpecies(tile); s != Plankton {
		t.Fatalf("next species in water = %v, want Plankton", s)
	}
}
