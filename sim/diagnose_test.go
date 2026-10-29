package sim

import "testing"

func TestDiagnose(t *testing.T) {
	p, lander, ore := economyPlanet(t)
	smelterCell := Cell{Face: 4, I: 8, J: 10}
	heaterCell := Cell{Face: 4, I: 6, J: 8}

	// Unlinked heater: no power.
	heater := mustBuild(t, p, HeaterMachine, heaterCell)
	p.Step()
	if got := p.Diagnose(heater).Kind; got != StatusNoPower {
		t.Fatalf("unlinked heater: %v, want NoPower", got)
	}

	// Linked with a mine: 3 supply, 4 demand.
	mine := mustBuild(t, p, Mine, ore)
	mustLink(t, p, lander, heaterCell)
	mustLink(t, p, lander, ore)
	p.Step()
	st := p.Diagnose(heater)
	if st.Kind != StatusLowPower || st.Supply != 3 || st.Demand != 4 {
		t.Fatalf("short network: %+v, want LowPower 3/4", st)
	}

	// Smelter with no ore anywhere: waiting for ore. (Power is short too, but
	// missing input is what stops it.)
	if err := p.Demolish(ore); err != nil {
		t.Fatal(err)
	}
	p.eco.Stores[Metal] += 50
	p.eco.Stores[Ore] = 0
	smelter := mustBuild(t, p, Smelter, smelterCell)
	mustLink(t, p, lander, smelterCell)
	p.Step()
	if st := p.Diagnose(smelter); st.Kind != StatusNoInput || st.Resource != Ore {
		t.Fatalf("starved smelter: %+v, want NoInput Ore", st)
	}

	// Unlinked mine on an exhausted deposit: depleted.
	_ = mine
	mine2 := mustBuild(t, p, Mine, ore)
	p.Tiles[p.Grid.Index(ore)].Ore = 0
	p.Step()
	if got := p.Diagnose(mine2).Kind; got != StatusDepleted {
		t.Fatalf("empty deposit: %v, want Depleted", got)
	}
}

func TestDiagnoseOutputFull(t *testing.T) {
	p, _, ore := economyPlanet(t)
	solarCell := Cell{Face: 4, I: 10, J: 10}
	mine := mustBuild(t, p, Mine, ore)
	mustBuild(t, p, SolarPanel, solarCell)
	mustLink(t, p, ore, solarCell)
	for range 100 {
		p.Step()
	}
	if st := p.Diagnose(mine); st.Kind != StatusOutputFull || st.Resource != Ore {
		t.Fatalf("mine with nowhere to send ore: %+v, want OutputFull Ore", st)
	}
}

func TestDiagnoseRunning(t *testing.T) {
	p, lander, ore := economyPlanet(t)
	mine := mustBuild(t, p, Mine, ore)
	mustLink(t, p, lander, ore)
	p.Step()
	if got := p.Diagnose(mine).Kind; got != StatusRunning {
		t.Fatalf("powered mine linked to lander: %v, want Running", got)
	}
	if got := p.Diagnose(p.BuildingAt(lander)).Kind; got != StatusRunning {
		t.Fatalf("lander: %v, want Running", got)
	}
}

func TestNearestPolluter(t *testing.T) {
	p, _, ore := economyPlanet(t)
	mustBuild(t, p, Mine, ore)
	mustBuild(t, p, SolarPanel, Cell{Face: 4, I: 9, J: 8})
	b, d := p.NearestPolluter(Cell{Face: 4, I: 8, J: 8}, 4)
	if b == nil || b.Type != Mine || d < 1.5 || d > 2.5 {
		t.Fatalf("nearest polluter = %v at %.1f, want the mine ~2 tiles away", b, d)
	}
	if b, _ := p.NearestPolluter(Cell{Face: 1, I: 8, J: 8}, 4); b != nil {
		t.Fatal("found a polluter on the far side of the planet")
	}
}
