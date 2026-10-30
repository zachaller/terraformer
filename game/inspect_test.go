package game

import (
	"testing"

	"github.com/zachaller/terraformer/sim"
)

func TestLifeNote(t *testing.T) {
	var tl sim.Tile
	if got := lifeNote(tl); got != "" {
		t.Errorf("barren tile: %q, want nothing", got)
	}
	tl.Life, tl.Soil = 1, 0.5
	tl.Bio[sim.Grass], tl.Bio[sim.Forest], tl.Bio[sim.Grazers] = 1, 0.8, 0.5
	want := "A herd of 3 grazers wanders here, and birds circle over the forest. Life here makes a lot of oxygen from CO2 and builds soil."
	if got := lifeNote(tl); got != want {
		t.Errorf("living tile:\n got %q\nwant %q", got, want)
	}
}
