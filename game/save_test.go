package game

import (
	"testing"

	"github.com/zachaller/terraformer/sim"
)

func TestSaveAndReload(t *testing.T) {
	SetSaveDir(t.TempDir())
	defer SetSaveDir("")

	g := &Game{opts: Options{GridSize: 16}}
	g.setPlanet(sim.NewPlanet(16, 9))
	if err := g.planet.Land(sim.Cell{Diamond: 4, I: 8, J: 8}); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		g.planet.Step()
	}
	if err := g.SaveNow(); err != nil {
		t.Fatal(err)
	}

	h := &Game{opts: Options{GridSize: 16}}
	h.setPlanet(sim.NewPlanet(16, 1))
	h.loadSave()
	if h.planet.Seed != 9 || h.planet.Ticks != g.planet.Ticks || !h.planet.Landed() {
		t.Fatalf("reloaded planet: seed %d ticks %d landed %v", h.planet.Seed, h.planet.Ticks, h.planet.Landed())
	}
	if h.tool != toolInspect {
		t.Fatalf("tool after loading a landed planet = %v, want Inspect", h.tool)
	}

	h.newPlanet()
	k := &Game{opts: Options{GridSize: 16}}
	k.setPlanet(sim.NewPlanet(16, 1))
	k.loadSave()
	if k.planet.Landed() {
		t.Fatal("New planet left the old save behind")
	}
}

func TestSavingDisabledForDemos(t *testing.T) {
	SetSaveDir(t.TempDir())
	defer SetSaveDir("")
	for _, o := range []Options{{DemoColony: true}, {DemoSteps: 10}, {ScreenshotPath: "x.png"}, {NoSave: true}} {
		if (&Game{opts: o}).savingEnabled() {
			t.Errorf("saving enabled for %+v", o)
		}
	}
}
