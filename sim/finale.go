package sim

import (
	"errors"
	"fmt"
)

// The finale: a living world that keeps itself alive. Two orbital projects
// replace the machines a terraformed planet depends on (the Soletta's light
// replaces the greenhouse factories' warmth, the L1 shield the surface
// shield's power), and the last goal asks the player to remove every machine
// and see the planet hold for trialDays on its own.

const (
	// solettaHeat is the most warming, °C, the finished Soletta adds at the
	// equator (60% of it at the poles).
	solettaHeat = 40.0
	// solettaTarget is the average temperature, °C, the Soletta's mirrors
	// tilt to hold the planet at, and solettaGain how quickly they adjust.
	solettaTarget = 15.0
	solettaGain   = 0.0005
	// trialDays is how long the planet must hold on its own.
	trialDays = 1000
	// trialLife is the share of the planet that must stay alive.
	trialLife = 0.4
)

// orbital is a project launched into orbit, and how many steps it has
// been building.
type orbital struct {
	Type     BuildingType
	Progress int
}

var (
	ErrNotOrbital = errors.New("That's built on the planet, not launched")
	ErrLaunched   = errors.New("That's already been launched")
)

// CanLaunch reports whether orbital project t can be launched now.
func (p *Planet) CanLaunch(t BuildingType) error {
	spec := Specs[t]
	switch {
	case !spec.Orbital:
		return ErrNotOrbital
	case !p.Landed():
		return ErrNotLanded
	case !p.Unlocked(t):
		return fmt.Errorf("%s unlocks after the goal %q", spec.Name, spec.UnlockedBy)
	case p.Launched(t):
		return ErrLaunched
	case p.eco.Stores[Metal] < spec.Cost:
		return fmt.Errorf("%s needs %.0f metal; the stores have %.1f", spec.Name, spec.Cost, p.eco.Stores[Metal])
	}
	return nil
}

// Launch starts building orbital project t, paying for it from the stores.
func (p *Planet) Launch(t BuildingType) error {
	if err := p.CanLaunch(t); err != nil {
		return err
	}
	p.eco.Stores[Metal] -= Specs[t].Cost
	p.built++
	p.orbitals = append(p.orbitals, orbital{Type: t})
	return nil
}

// Launched reports whether orbital project t has been launched.
func (p *Planet) Launched(t BuildingType) bool {
	_, ok := p.orbitalProgress(t)
	return ok
}

// OrbitalBuilt is how far orbital project t has been built, from 0 (not
// launched) to 1 (working).
func (p *Planet) OrbitalBuilt(t BuildingType) float64 {
	n, _ := p.orbitalProgress(t)
	return min(float64(n)/float64(Specs[t].Countdown), 1)
}

func (p *Planet) orbitalProgress(t BuildingType) (int, bool) {
	for _, o := range p.orbitals {
		if o.Type == t {
			return o.Progress, true
		}
	}
	return 0, false
}

// SolettaWarming is the warming, °C, the Soletta adds at the equator now:
// it grows as the ring is built, and its mirrors tilt to send down only as
// much light as the planet needs to stay near solettaTarget.
func (p *Planet) SolettaWarming() float64 {
	return solettaHeat * p.OrbitalBuilt(Soletta) * p.solettaPower
}

// adjustSoletta tilts the Soletta's mirrors towards holding the average
// temperature at solettaTarget.
func (p *Planet) adjustSoletta() {
	if !p.Launched(Soletta) {
		return
	}
	p.solettaPower += solettaGain * (solettaTarget - p.Stats().AvgTemp)
	p.solettaPower = min(max(p.solettaPower, 0), 1)
}

// stepOrbitals builds the launched projects, and runs the final goal's
// trial.
func (p *Planet) stepOrbitals() {
	for i := range p.orbitals {
		if o := &p.orbitals[i]; o.Progress < Specs[o.Type].Countdown {
			o.Progress++
		}
	}
	p.adjustSoletta()
	if p.goalsDone == GoalIndex(FinalGoal) && p.WorldHolds() {
		if p.trialStart == 0 {
			p.trialStart = p.Ticks + 1
		}
	} else {
		p.trialStart = 0
	}
}

// Machines is how many machines are on the planet: every building but the
// lander. Orbital projects don't count.
func (p *Planet) Machines() int {
	return len(p.eco.buildings) - p.count(Lander)
}

// Condition is one thing the final goal's trial needs, and whether it's met.
type Condition struct {
	Name string
	Met  bool
}

// WorldConditions are what the planet must keep up, with no machines,
// through the final goal's trial.
func (p *Planet) WorldConditions() []Condition {
	st := p.Stats()
	return []Condition{
		{"No machines left", p.Machines() == 0},
		{"Breathable air", p.Air.O2 >= 16 && p.Air.CO2 <= 1 && p.Pressure() >= 25},
		{"Average above 5°C", st.AvgTemp >= 5},
		{"Life on 40% of the planet", st.LifeCover >= trialLife},
	}
}

// WorldHolds reports whether every WorldCondition is met.
func (p *Planet) WorldHolds() bool {
	for _, c := range p.WorldConditions() {
		if !c.Met {
			return false
		}
	}
	return true
}

// TrialDays is how many days the planet has held on its own in the final
// goal's trial so far.
func (p *Planet) TrialDays() float64 {
	if p.trialStart == 0 {
		return 0
	}
	return float64(p.Ticks+1-p.trialStart) / stepsPerDay
}

// FinalGoal is the last goal's name.
const FinalGoal = "A world that runs itself"

// Warmth is one source of the planet's warmth: how many degrees it adds on
// average, and whether it lasts without machines.
type Warmth struct {
	Name    string
	Degrees float64
	// Lasting sources keep working with every machine removed; the rest
	// fade or stop without them.
	Lasting bool
	Note    string
}

// Warmths is what keeps the planet warm, source by source.
func (p *Planet) Warmths() []Warmth {
	gh := p.Greenhouse()
	out := []Warmth{
		{"CO2", gh.CO2, true, "natural"},
		{"Water vapour", gh.Vapour, true, "natural"},
		{"Thicker air", gh.Broadening, true, "natural"},
		{"Super-greenhouse gas", gh.PFC, false, "fades without factories"},
	}
	if heat := p.machineHeat(); heat > 0.05 {
		out = append(out, Warmth{"Heaters and mirrors", heat, false, "stop without machines"})
	}
	if p.Launched(Soletta) {
		note := fmt.Sprintf("permanent · at %.0f%%", p.solettaPower*100)
		if p.OrbitalBuilt(Soletta) < 1 {
			note = "building"
		}
		out = append(out, Warmth{"Soletta", p.solettaAverage(), true, note})
	}
	return out
}

// machineHeat is the average warming, °C, from buildings across the planet.
func (p *Planet) machineHeat() float64 {
	total := 0.0
	for idx := range p.Tiles {
		total += p.TotalHeat(idx)
	}
	return total / float64(len(p.Tiles))
}

// solettaAverage is the Soletta's warming now, averaged over the planet.
func (p *Planet) solettaAverage() float64 {
	return p.SolettaWarming() * p.solettaSpread()
}

// solettaSpread is how much of the Soletta's equatorial warming the planet
// gets on average.
func (p *Planet) solettaSpread() float64 {
	total := 0.0
	for idx := range p.Tiles {
		total += 0.6 + 0.4*p.cosLat[idx]
	}
	return total / float64(len(p.Tiles))
}

// WithoutMachines is roughly where the average temperature would settle if
// every machine were removed now: without what fades or stops, and with
// the Soletta finished if it's building. It's an estimate: losing warmth
// also freezes seas and brightens the ground, which cools it further.
func (p *Planet) WithoutMachines() float64 {
	t := p.Stats().AvgTemp
	for _, w := range p.Warmths() {
		if !w.Lasting {
			t -= w.Degrees
		}
	}
	if p.Launched(Soletta) {
		// The Soletta can turn up to its full strength to make up for what's
		// lost, but tilts back past solettaTarget.
		spare := solettaHeat*p.solettaSpread() - p.solettaAverage()
		if t < solettaTarget {
			t = min(t+spare, solettaTarget)
		}
	}
	return t
}

// AirKept says what stops the air escaping to space: "L1 shield" (lasting),
// "Magnetic shield" (needs power), or "" if nothing does.
func (p *Planet) AirKept() string {
	switch {
	case p.OrbitalBuilt(L1Shield) >= 1:
		return "L1 shield"
	case p.MagneticShield():
		return "Magnetic shield"
	}
	return ""
}

// LiftOff takes the lander off the planet once every goal is done: the
// colony's work is finished. Building again means landing again.
func (p *Planet) LiftOff() error {
	if p.goalsDone < len(Goals) {
		return errors.New("The planet isn't finished yet")
	}
	for _, b := range p.eco.buildings {
		if b.Type == Lander {
			p.remove(b)
			p.liftedOff = true
			return nil
		}
	}
	return errors.New("The lander has already lifted off")
}

// LiftedOff reports whether the colony's lander has left the finished
// planet (even if a new one has landed since).
func (p *Planet) LiftedOff() bool { return p.liftedOff }

// LanderCell is where the lander stands, if it does.
func (p *Planet) LanderCell() (Cell, bool) {
	for _, b := range p.eco.buildings {
		if b.Type == Lander {
			return p.Grid.CellAt(b.Tile), true
		}
	}
	return Cell{}, false
}

// Built is how many buildings have been built on the planet, all told.
func (p *Planet) Built() int { return p.built }

// WonOnDay is the day the final goal was completed, or 0 if it hasn't been.
func (p *Planet) WonOnDay() int { return int(p.wonAt / stepsPerDay) }
