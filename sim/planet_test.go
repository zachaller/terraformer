package sim

import "testing"

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
		counts[tile.Kind]++
	}
	for _, k := range []TileKind{Basin, Plains, Highlands, Peaks, IceCap} {
		if counts[k] == 0 {
			t.Errorf("no %v tiles generated", k)
		}
	}
}
