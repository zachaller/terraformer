package sim

import (
	"bytes"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p, lander, ore := economyPlanet(t)
	smelterCell := Cell{Diamond: 4, I: 8, J: 10}
	heaterCell := Cell{Diamond: 4, I: 6, J: 8}
	mustBuild(t, p, Mine, ore)
	mustBuild(t, p, Smelter, smelterCell)
	mustBuild(t, p, HeaterMachine, heaterCell)
	mustLink(t, p, lander, ore)
	mustLink(t, p, lander, smelterCell)
	mustLink(t, p, lander, heaterCell)
	for range 300 {
		p.Step()
	}

	var buf bytes.Buffer
	if err := p.Save(&buf); err != nil {
		t.Fatal(err)
	}
	q, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// The loaded planet must carry on exactly as the original would.
	for range 300 {
		p.Step()
		q.Step()
	}
	for i := range p.Tiles {
		if p.Tiles[i] != q.Tiles[i] {
			t.Fatalf("tile %d differs after loading", i)
		}
	}
	if p.Stores() != q.Stores() || p.Atmosphere != q.Atmosphere || p.Ticks != q.Ticks || p.GoalsDone() != q.GoalsDone() {
		t.Fatal("planet state differs after loading")
	}
	if len(p.Buildings()) != len(q.Buildings()) || len(p.Links()) != len(q.Links()) {
		t.Fatal("buildings or links differ after loading")
	}
}

func TestLoadRejectsGarbage(t *testing.T) {
	if _, err := Load(bytes.NewReader([]byte("not a save"))); err == nil {
		t.Fatal("loaded garbage")
	}
}
