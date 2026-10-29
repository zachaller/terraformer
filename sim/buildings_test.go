package sim

import (
	"errors"
	"testing"
)

// unlockAll marks every goal done, so every building can be built.
func unlockAll(p *Planet) { p.goalsDone = len(Goals) }

func TestUnlockedByNamesAGoal(t *testing.T) {
	for bt, s := range Specs {
		if s.UnlockedBy != "" && GoalIndex(s.UnlockedBy) < 0 {
			t.Errorf("%v unlocks after %q, which isn't a goal", BuildingType(bt), s.UnlockedBy)
		}
	}
}

func TestLockedBuildings(t *testing.T) {
	p, _, _ := economyPlanet(t)
	p.eco.Stores[Metal] = 1000
	if _, err := p.Build(Electrolyser, Cell{Diamond: 4, I: 6, J: 6}); err == nil {
		t.Fatal("built an electrolyser before it unlocked")
	}
	unlockAll(p)
	mustBuild(t, p, Electrolyser, Cell{Diamond: 4, I: 6, J: 6})
}

func TestIceDrillFeedsElectrolyser(t *testing.T) {
	p, lander, _ := economyPlanet(t)
	unlockAll(p)
	p.eco.Stores[Metal] = 1000
	drill := Cell{Diamond: 4, I: 6, J: 8}
	p.Tiles[p.Grid.Index(drill)].Ice = 1
	el := Cell{Diamond: 4, I: 7, J: 7}
	mustBuild(t, p, IceDrill, drill)
	mustBuild(t, p, Electrolyser, el)
	for _, c := range []Cell{{Diamond: 4, I: 9, J: 9}, {Diamond: 4, I: 9, J: 7}, {Diamond: 4, I: 7, J: 9}} {
		mustBuild(t, p, SolarPanel, c)
		mustLink(t, p, lander, c)
	}
	mustLink(t, p, lander, drill)
	mustLink(t, p, lander, el)
	o2 := p.Air.O2
	for range 200 {
		p.Step()
	}
	if p.Air.O2 <= o2 {
		t.Fatalf("electrolyser made no oxygen (%.4f → %.4f kPa)", o2, p.Air.O2)
	}
	if p.Tile(drill).Ice >= 1 {
		t.Fatal("ice drill took no ice")
	}
}

func TestSiteRules(t *testing.T) {
	p, _, _ := economyPlanet(t)
	unlockAll(p)
	p.eco.Stores[Metal] = 1000
	c := Cell{Diamond: 4, I: 5, J: 5}
	p.Tiles[p.Grid.Index(c)].Ice = 0
	p.Tiles[p.Grid.Index(c)].Volcanic = false
	if _, err := p.Build(IceDrill, c); !errors.Is(err, ErrNoIce) {
		t.Errorf("ice drill off ice: got %v", err)
	}
	if _, err := p.Build(Geothermal, c); !errors.Is(err, ErrNotVolcanic) {
		t.Errorf("geothermal off a volcano: got %v", err)
	}
	if _, err := p.Build(Comet, Cell{Diamond: 4, I: 9, J: 8}); err == nil {
		t.Error("aimed a comet next to the colony")
	}
	mustBuild(t, p, MagneticShield, c)
	if _, err := p.Build(MagneticShield, Cell{Diamond: 4, I: 4, J: 4}); !errors.Is(err, ErrUnique) {
		t.Errorf("second shield: got %v", err)
	}
}

func TestWindNeedsAir(t *testing.T) {
	p, _, _ := economyPlanet(t)
	if p.WindFactor() != 0 {
		t.Errorf("wind turbines turn in %.2f kPa", p.Pressure())
	}
	p.Air.N2 = 50
	if p.WindFactor() != 1 {
		t.Errorf("wind factor %.2f in 50 kPa, want 1", p.WindFactor())
	}
}

func TestCometImpact(t *testing.T) {
	p, _, _ := economyPlanet(t)
	unlockAll(p)
	p.eco.Stores[Metal] = 1000
	target := Cell{Diamond: 1, I: 8, J: 8}
	mustBuild(t, p, Comet, target)
	n2, ice := p.Air.N2, p.Tile(target).Ice
	for range Specs[Comet].Countdown + 1 {
		p.Step()
	}
	if p.BuildingAt(target) != nil {
		t.Fatal("the comet is still on its way")
	}
	if p.Air.N2 < n2+cometN2*0.9 || p.Tile(target).Ice <= ice {
		t.Errorf("impact brought too little: N2 %.2f → %.2f, ice %.2f → %.2f", n2, p.Air.N2, ice, p.Tile(target).Ice)
	}
}

func TestDustSpreaderDarkensIce(t *testing.T) {
	p, lander, _ := economyPlanet(t)
	unlockAll(p)
	p.eco.Stores[Metal] = 1000
	c := Cell{Diamond: 4, I: 6, J: 8}
	for _, idx := range p.ringsAround(p.Grid.Index(c), 2)[1] {
		p.Tiles[idx].Ice = 1
	}
	mustBuild(t, p, DustSpreader, c)
	mustLink(t, p, lander, c)
	for range 100 {
		p.Step()
	}
	nb := p.ringsAround(p.Grid.Index(c), 1)[1][0]
	if d := p.Tiles[nb].Dust; d <= 0 {
		t.Errorf("no dust on the ice next to the spreader")
	}
	if a := Albedo(&p.Tiles[nb]); a >= 0.5 {
		t.Errorf("dusty ice still reflects %.2f of sunlight", a)
	}
}
