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
	// solettaHeat is the warming, °C, the finished Soletta adds at the
	// equator (60% of it at the poles).
	solettaHeat = 40.0
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
// it grows as the ring is built.
func (p *Planet) SolettaWarming() float64 {
	return solettaHeat * p.OrbitalBuilt(Soletta)
}

// stepOrbitals builds the launched projects, and runs the final goal's
// trial.
func (p *Planet) stepOrbitals() {
	for i := range p.orbitals {
		if o := &p.orbitals[i]; o.Progress < Specs[o.Type].Countdown {
			o.Progress++
		}
	}
	if p.goalsDone == GoalIndex(finalGoal) && p.WorldHolds() {
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

// finalGoal is the last goal's name.
const finalGoal = "A world that runs itself"
