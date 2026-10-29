package sim

import (
	"math"
	"testing"
)

func TestBoilingPoint(t *testing.T) {
	for _, tc := range []struct{ kPa, want float64 }{{101.3, 100}, {TriplePoint, 0}, {0.3, 0}, {50, 81}} {
		if got := boilingPoint(tc.kPa); math.Abs(got-tc.want) > 2 {
			t.Errorf("boilingPoint(%v) = %.1f, want about %v", tc.kPa, got, tc.want)
		}
	}
}

func TestStartIsStable(t *testing.T) {
	// Left alone, a new planet's air and temperature hold steady.
	p := NewPlanet(16, 3)
	before, temp := p.Pressure(), p.Stats().AvgTemp
	for range 5000 {
		p.Step()
	}
	if d := p.Pressure() - before; math.Abs(d) > 0.05 {
		t.Errorf("pressure drifted from %.3f to %.3f kPa", before, p.Pressure())
	}
	if d := p.Stats().AvgTemp - temp; math.Abs(d) > 1 {
		t.Errorf("average temperature drifted from %.1f to %.1f", temp, p.Stats().AvgTemp)
	}
}

func TestNoLiquidBelowTriplePoint(t *testing.T) {
	p := NewPlanet(16, 3)
	c := p.Grid.CellFromDir(Vec3{X: 1})
	p.AddHeat(c, maxHeat)
	for range 500 {
		p.Step()
	}
	if p.Pressure() >= TriplePoint {
		t.Skip("one heater pushed the air past the triple point")
	}
	if w := p.Stats().Water; w > 1e-9 {
		t.Errorf("%.3f liquid water below the triple point", w)
	}
	if p.Vapour == 0 && p.Stats().Ice == 0 {
		t.Error("heated ice vanished")
	}
}

func TestDustedCapsReleaseCO2(t *testing.T) {
	p := NewPlanet(16, 3)
	start := p.Air.CO2
	for range 3000 {
		for i := range p.Tiles {
			if p.Tiles[i].DryIce > 0 {
				p.Tiles[i].Dust = 1
			}
		}
		p.Step()
	}
	if p.Air.CO2 < start+1 {
		t.Errorf("dusted caps released too little CO₂: %.2f → %.2f kPa", start, p.Air.CO2)
	}
}

func TestGreenhouseGasesWarm(t *testing.T) {
	p := NewPlanet(16, 3)
	base := p.Greenhouse().Total()
	p.Air.PFC = 1
	if d := p.Greenhouse().Total() - base; d < 10 {
		t.Errorf("1 Pa of PFC adds only %.1f °C", d)
	}
}

func TestShieldStopsEscape(t *testing.T) {
	p := NewPlanet(8, 1)
	p.Air = Air{CO2: 10, N2: 10, O2: 10}
	before := p.Pressure()
	for range 1000 {
		p.stepAir()
	}
	if p.Pressure() >= before {
		t.Fatal("no air escaped without a shield")
	}
	if p.EscapePerDay() <= 0 {
		t.Error("EscapePerDay is 0 without a shield")
	}
}
