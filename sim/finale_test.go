package sim

import (
	"bytes"
	"testing"
)

// finalePlanet returns a landed planet with every goal up to the final one
// done, and plenty of metal.
func finalePlanet(t *testing.T) (*Planet, Cell) {
	t.Helper()
	p, lander, _ := economyPlanet(t)
	p.goalsDone = GoalIndex(FinalGoal)
	p.eco.Stores[Metal] = 5000
	return p, lander
}

func TestLaunch(t *testing.T) {
	p, lander := finalePlanet(t)
	if err := p.CanBuild(Soletta, near(lander, 2)); err == nil {
		t.Error("an orbital project could be built on a tile")
	}
	if err := p.Launch(Soletta); err != nil {
		t.Fatal(err)
	}
	if err := p.Launch(Soletta); err != ErrLaunched {
		t.Errorf("second launch: %v, want ErrLaunched", err)
	}
	if got := p.Stores()[Metal]; got != 5000-Specs[Soletta].Cost {
		t.Errorf("metal after launch %v", got)
	}
	if p.Machines() != 0 {
		t.Error("an orbital project counts as a machine")
	}

	// The L1 shield keeps the air once it's built.
	if err := p.Launch(L1Shield); err != nil {
		t.Fatal(err)
	}
	if p.MagneticShield() {
		t.Error("the L1 shield works before it's built")
	}
	for range Specs[L1Shield].Countdown {
		p.stepOrbitals()
	}
	if !p.MagneticShield() || p.EscapePerDay() != 0 {
		t.Error("the finished L1 shield doesn't keep the air")
	}
	if w := p.SolettaWarming(); w <= 0 || w >= solettaHeat {
		t.Errorf("Soletta part-built warming %v, want between 0 and %v", w, solettaHeat)
	}

	// Locked before Breathable air.
	q, _, _ := economyPlanet(t)
	q.eco.Stores[Metal] = 5000
	if err := q.Launch(Soletta); err == nil {
		t.Error("Soletta launched before it unlocks")
	}
}

// TestTrial checks the final goal's trial counts days while the planet
// holds with no machines, and starts again when it doesn't.
func TestTrial(t *testing.T) {
	p, lander := finalePlanet(t)
	p.Air = Air{CO2: 0.5, N2: 10, O2: 20}
	for i := range p.Tiles {
		p.Tiles[i].Temp, p.Tiles[i].Life = 15, 1
	}
	for range 100 {
		p.stepOrbitals()
		p.Ticks++
	}
	if d := p.TrialDays(); d != 10 {
		t.Fatalf("trial at %v days after 100 steps, want 10", d)
	}
	if prog := Goals[p.goalsDone].Progress(p); prog != 10.0/trialDays {
		t.Errorf("progress %v", prog)
	}
	mustBuild(t, p, SolarPanel, near(lander, 1))
	p.stepOrbitals()
	if p.TrialDays() != 0 {
		t.Error("the trial kept going with a machine on the planet")
	}
}

func TestLiftOff(t *testing.T) {
	p, _ := finalePlanet(t)
	if err := p.LiftOff(); err == nil {
		t.Fatal("lifted off before the planet was finished")
	}
	p.goalsDone = len(Goals)
	if err := p.LiftOff(); err != nil {
		t.Fatal(err)
	}
	if p.Landed() {
		t.Error("still landed after lift-off")
	}
}

func TestLevels(t *testing.T) {
	easy, hard := NewPlanetLevel(16, 3, 0), NewPlanetLevel(16, 3, 2)
	sum := func(p *Planet) (temp, ice, ore float64) {
		for _, tl := range p.Tiles {
			temp += tl.Temp
			ice += tl.Ice
			ore += tl.Ore
		}
		return
	}
	et, ei, eo := sum(easy)
	ht, hi, ho := sum(hard)
	if ht >= et || hi >= ei || ho >= eo {
		t.Errorf("level 2 not harder: temp %v vs %v, ice %v vs %v, ore %v vs %v", ht, et, hi, ei, ho, eo)
	}
}

// TestSaveFinale checks the finale's state survives a save and load.
func TestSaveFinale(t *testing.T) {
	p := NewPlanetLevel(16, 5, 1)
	if err := p.Land(Cell{Diamond: 4, I: 8, J: 8}); err != nil {
		t.Fatal(err)
	}
	p.goalsDone = GoalIndex(FinalGoal)
	p.eco.Stores[Metal] = 5000
	if err := p.Launch(Soletta); err != nil {
		t.Fatal(err)
	}
	p.stepOrbitals()
	var buf bytes.Buffer
	if err := p.Save(&buf); err != nil {
		t.Fatal(err)
	}
	q, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if q.Level != 1 || q.OrbitalBuilt(Soletta) != p.OrbitalBuilt(Soletta) || q.Built() != p.Built() {
		t.Errorf("loaded level %d, Soletta %v, built %d; want %d, %v, %d", q.Level, q.OrbitalBuilt(Soletta), q.Built(), p.Level, p.OrbitalBuilt(Soletta), p.Built())
	}
}
