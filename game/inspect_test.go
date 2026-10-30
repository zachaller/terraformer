package game

import (
	"testing"

	"github.com/zachaller/terraformer/sim"
)

func TestLifeNote(t *testing.T) {
	var tl sim.Tile
	if got := lifeNote(readLife(tl, nil)); got != "" {
		t.Errorf("barren tile: %q, want nothing", got)
	}
	tl.Life, tl.Soil = 1, 0.5
	tl.Bio[sim.Grass], tl.Bio[sim.Forest], tl.Bio[sim.Grazers] = 1, 0.8, 0.5
	want := "A herd of grazers wanders here, and birds circle over the forest. Life here makes a lot of oxygen from CO2 and builds soil."
	if got := lifeNote(readLife(tl, nil)); got != want {
		t.Errorf("living tile:\n got %q\nwant %q", got, want)
	}
}

// TestLifeNoteSteady checks the note doesn't flip as a density wobbles
// around a threshold: birds appear above 40% forest, and stay until it falls
// clearly below.
func TestLifeNoteSteady(t *testing.T) {
	var tl sim.Tile
	tl.Life, tl.Bio[sim.Forest] = 1, 0.41
	s := readLife(tl, nil)
	if !s.birds {
		t.Fatal("no birds at 41% forest")
	}
	for _, f := range []float64{0.39, 0.41, 0.38, 0.4, 0.37} {
		tl.Bio[sim.Forest] = f
		if s = readLife(tl, &s); !s.birds {
			t.Fatalf("birds gone at %.0f%% forest", f*100)
		}
	}
	tl.Bio[sim.Forest] = 0.3
	if s = readLife(tl, &s); s.birds {
		t.Error("birds still there at 30% forest")
	}
	// Oxygen level: steady just below where "a lot" starts once reached.
	tl.Life, tl.Bio = 0, [sim.NumSpecies]float64{}
	tl.Bio[sim.Forest] = 1.6
	if s = readLife(tl, nil); s.oxygen != 3 {
		t.Fatalf("oxygen level %d at 1.6, want 3", s.oxygen)
	}
	tl.Bio[sim.Forest] = 1.45 // just under where "a lot" starts
	if s = readLife(tl, &s); s.oxygen != 3 {
		t.Errorf("oxygen level flipped to %d on a small drop", s.oxygen)
	}
	tl.Bio[sim.Forest] = 1.2
	if s = readLife(tl, &s); s.oxygen != 2 {
		t.Errorf("oxygen level %d after a clear drop, want 2", s.oxygen)
	}
}
