package sim

import "math"

// StatusKind says how a building is doing, and if it's held back, why.
type StatusKind int

const (
	StatusRunning    StatusKind = iota // working at full rate
	StatusLowPower                     // running slower: its network is short of power
	StatusNoPower                      // stopped: its network has no power at all
	StatusNoInput                      // stopped or slowed: waiting for an input resource
	StatusOutputFull                   // stopped: nowhere for its output to go
	StatusDepleted                     // stopped: its deposit (or ice) is used up
	StatusNoWind                       // a wind turbine in air too thin to turn it
	StatusIncoming                     // a comet on its way
)

// Status is a building's current condition, from the most recent step.
type Status struct {
	Kind     StatusKind
	Resource Resource // the input or output involved, for NoInput and OutputFull
	// Supply and Demand are the power in the building's network.
	Supply, Demand float64
}

// Diagnose explains how b is doing. When several things hold it back, it
// reports the one the player should fix first.
func (p *Planet) Diagnose(b *Building) Status {
	s := b.Spec()
	n := p.NetworkOf(b)
	st := Status{Supply: n.PowerSupply, Demand: n.PowerDemand}
	power := n.PowerSatisfaction()
	switch {
	case s.Countdown > 0:
		st.Kind = StatusIncoming
		return st
	case s.Site == DepositSite && p.Tiles[b.Tile].Ore <= 0, s.Site == IceSite && p.Tiles[b.Tile].Ice <= 0:
		st.Kind = StatusDepleted
		return st
	case s.Wind && p.WindFactor() < 0.05:
		st.Kind = StatusNoWind
		return st
	case s.PowerIn > 0 && power == 0:
		st.Kind = StatusNoPower
		return st
	}
	for r := range NumResources {
		if in := s.Inputs[r]; in > 0 && b.In[r] < in*power*0.999 {
			st.Kind, st.Resource = StatusNoInput, Resource(r)
			return st
		}
	}
	out := p.Output(b)
	for r := range NumResources {
		if out := out[r]; out > 0 && b.Out[r]+out*power > bufferCap*1.0001 {
			st.Kind, st.Resource = StatusOutputFull, Resource(r)
			return st
		}
	}
	if s.PowerIn > 0 && power < 1 {
		st.Kind = StatusLowPower
	}
	return st
}

// NearestPolluter returns the closest polluting building to c within
// maxDist tiles, and its distance, or nil.
func (p *Planet) NearestPolluter(c Cell, maxDist float64) (*Building, float64) {
	var best *Building
	bestD := math.Inf(1)
	for _, b := range p.eco.buildings {
		if b.Spec().Pollution == 0 {
			continue
		}
		if d := p.LinkLength(c, p.Grid.CellAt(b.Tile)); d <= maxDist && d < bestD {
			best, bestD = b, d
		}
	}
	return best, bestD
}

// Limits on where microbes can live; see Habitable.
const (
	MinLifeTemp      = minLifeTemp
	MaxLifeTemp      = maxLifeTemp
	MinLifeWater     = minLifeWater
	MaxLifePollution = maxLifePollution
)
