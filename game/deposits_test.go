package game

import (
	"testing"

	"github.com/zachaller/terraformer/sim"
)

// TestMoveMine checks a mine on a dug-out patch moves to a free deposit of
// its own mineral, and that a tap next to a deposit snaps to it.
func TestMoveMine(t *testing.T) {
	g := &Game{opts: Options{GridSize: 16}}
	g.setPlanet(sim.NewPlanet(16, 9))
	p := g.planet
	lander := sim.Cell{Diamond: 4, I: 8, J: 8}
	for i := range p.Tiles {
		p.Tiles[i].Ore = 0
	}
	old := sim.Cell{Diamond: 4, I: 10, J: 8}
	dest := sim.Cell{Diamond: 4, I: 8, J: 10}
	for _, c := range []sim.Cell{old, dest} {
		p.Tiles[p.Grid.Index(c)].Ore, p.Tiles[p.Grid.Index(c)].Mineral = 100, sim.Iron
	}
	if err := p.Land(lander); err != nil {
		t.Fatal(err)
	}
	if got := g.snapToDeposit(sim.Cell{Diamond: 4, I: 11, J: 8}); got != old {
		t.Errorf("tap next to a deposit snapped to %v, want %v", got, old)
	}
	if _, err := p.Build(sim.Mine, old); err != nil {
		t.Fatal(err)
	}
	p.Tiles[p.Grid.Index(old)].Ore = 0
	g.moveMine(old)
	if p.BuildingAt(old) != nil {
		t.Error("the old mine is still there")
	}
	if b := p.BuildingAt(dest); b == nil || b.Type != sim.Mine {
		t.Errorf("no mine on the free deposit at %v", dest)
	}
}
