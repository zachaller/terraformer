package sim

import "math"

// Tuning constants, per simulation step.
const (
	tempRelax     = 0.02 // how fast a tile approaches its equilibrium temperature
	tempDiffusion = 0.08 // how fast heat spreads to neighbours
	meltRate      = 0.002
	freezeRate    = 0.002
	flowRate      = 0.2
	maxFlowShare  = 1.0 / MaxNeighbors // most of a tile's free water that can leave to one neighbour
	soilRetention = 0.05               // water the ground holds as moisture instead of letting it flow
	elevScale     = 2.0                // converts Height into the same units as water depth

	lifeGrowth       = 0.01
	lifeSpread       = 0.02
	lifeDieback      = 0.01
	lifeEmerge       = 0.0005 // chance per step that life appears on a habitable, lifeless tile
	lifeSeed         = 0.05   // coverage when life first appears
	minLifeWater     = 0.01
	minLifeTemp      = -5.0
	maxLifeTemp      = 60.0
	maxLifePollution = 0.25

	pollutionSpread = 0.08  // share that drifts away to neighbours per step
	pollutionDecay  = 0.001 // cleaned up naturally per step
)

// Step advances the simulation by one tick. It is deterministic: the same
// planet state always produces the same next state.
func (p *Planet) Step() {
	p.stepEconomy()
	p.stepTemperature()
	p.stepIce()
	p.stepWater()
	p.stepPollution()
	p.stepLife()
	p.stepSpecies()
	p.Ticks++
	p.checkGoals()
}

// equilibrium is the temperature tile idx settles towards without machines:
// warmest at the equator, warmer with a thicker atmosphere (greenhouse
// effect), colder up high and where ice reflects sunlight away.
func (p *Planet) equilibrium(idx int) float64 {
	t := &p.Tiles[idx]
	return -85 + 70*p.cosLat[idx] +
		7*math.Log2(1+p.Atmosphere) -
		15*math.Max(t.Height, 0) -
		8*math.Min(t.Ice*3, 1)
}

func (p *Planet) stepTemperature() {
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		avg := 0.0
		for _, nb := range p.neighbors[idx] {
			avg += p.Tiles[nb].Temp
		}
		avg /= float64(len(p.neighbors[idx]))
		target := p.equilibrium(idx) + p.TotalHeat(idx)
		p.nextTemp[idx] = t.Temp + tempRelax*(target-t.Temp) + tempDiffusion*(avg-t.Temp)
	}
	for idx := range p.Tiles {
		p.Tiles[idx].Temp = p.nextTemp[idx]
	}
}

// stepIce melts and freezes water. The ice holds frozen CO₂, so melting
// thickens the atmosphere, which warms the whole planet: a feedback loop.
func (p *Planet) stepIce() {
	released := 0.0
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		switch {
		case t.Temp > 0 && t.Ice > 0:
			m := math.Min(t.Ice, meltRate*t.Temp)
			t.Ice -= m
			t.Water += m
			released += m
		case t.Temp < 0 && t.Water > 0:
			f := math.Min(t.Water, freezeRate*-t.Temp)
			t.Water -= f
			t.Ice += f
		}
	}
	p.Atmosphere += released * p.co2PerIce
}

// stepWater moves liquid water downhill. The ground keeps up to
// soilRetention as moisture; only water above that flows. Each neighbouring
// pair exchanges water once, and no tile gives away more than maxFlowShare of
// its free water to any one neighbour, so water is conserved and never goes
// negative.
func (p *Planet) stepWater() {
	clear(p.flow)
	for a := range p.Tiles {
		ta := &p.Tiles[a]
		for _, nb := range p.neighbors[a] {
			b := int(nb)
			if b <= a {
				continue
			}
			tb := &p.Tiles[b]
			diff := (ta.Height*elevScale + ta.Water) - (tb.Height*elevScale + tb.Water)
			var f float64
			if diff > 0 {
				f = math.Min(diff*flowRate, freeWater(ta)*maxFlowShare)
			} else {
				f = -math.Min(-diff*flowRate, freeWater(tb)*maxFlowShare)
			}
			p.flow[a] -= f
			p.flow[b] += f
		}
	}
	for idx := range p.Tiles {
		p.Tiles[idx].Water = math.Max(p.Tiles[idx].Water+p.flow[idx], 0)
	}
}

func freeWater(t *Tile) float64 { return math.Max(t.Water-soilRetention, 0) }

// TotalHeat is the warming on tile idx from heaters this step, plus any
// fixed extra heat set with AddHeat.
func (p *Planet) TotalHeat(idx int) float64 {
	return p.Tiles[idx].Heat + p.eco.machineHeat[idx]
}

// stepPollution lets pollution drift to neighbours and slowly break down.
func (p *Planet) stepPollution() {
	clear(p.flow)
	for idx := range p.Tiles {
		out := p.Tiles[idx].Pollution * pollutionSpread / float64(len(p.neighbors[idx]))
		for _, nb := range p.neighbors[idx] {
			p.flow[nb] += out
			p.flow[idx] -= out
		}
	}
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		t.Pollution = math.Max(t.Pollution+p.flow[idx]-pollutionDecay, 0)
	}
}

// Habitable reports whether microbes can live on t.
func Habitable(t Tile) bool {
	return t.Temp > minLifeTemp && t.Temp < maxLifeTemp && t.Water > minLifeWater &&
		t.Pollution < maxLifePollution
}

// stepLife grows microbes where conditions suit them, spreads them to
// neighbours, lets them appear spontaneously, and kills them off where
// conditions turn bad.
func (p *Planet) stepLife() {
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		l := t.Life
		if !Habitable(*t) {
			p.nextLife[idx] = math.Max(l-lifeDieback, 0)
			continue
		}
		avg := 0.0
		for _, nb := range p.neighbors[idx] {
			avg += p.Tiles[nb].Life
		}
		avg /= float64(len(p.neighbors[idx]))
		l += lifeGrowth*l*(1-l) + lifeSpread*avg*(1-l)
		if l == 0 && p.chance(idx, 0) < lifeEmerge {
			l = lifeSeed
		}
		p.nextLife[idx] = math.Min(l, 1)
	}
	for idx := range p.Tiles {
		p.Tiles[idx].Life = p.nextLife[idx]
	}
}

// chance returns a deterministic pseudo-random number in [0, 1) for tile idx
// on the current tick. Different salts give independent streams.
func (p *Planet) chance(idx, salt int) float64 {
	return (hash3(p.Seed^uint64(salt)*0x9E3779B97F4A7C15, int64(p.Ticks), int64(idx), 0x11fe) + 1) / 2
}

// AddHeat warms the tile at c and, by half as much, its neighbours, as a
// heater would. Heat is capped at maxHeat per tile.
func (p *Planet) AddHeat(c Cell, amount float64) {
	idx := p.Grid.Index(c)
	p.addHeat(idx, amount)
	for _, nb := range p.neighbors[idx] {
		p.addHeat(int(nb), amount/2)
	}
}

const maxHeat = 100

func (p *Planet) addHeat(idx int, amount float64) {
	t := &p.Tiles[idx]
	t.Heat = math.Min(t.Heat+amount, maxHeat)
}

// Stats summarises the whole planet.
type Stats struct {
	AvgTemp    float64
	Ice, Water float64 // totals
	LifeCover  float64 // fraction of tiles with any life
	LifeMean   float64 // average life coverage over all tiles
}

func (p *Planet) Stats() Stats {
	var s Stats
	alive := 0
	for _, t := range p.Tiles {
		s.AvgTemp += t.Temp
		s.Ice += t.Ice
		s.Water += t.Water
		if t.Life > 0 {
			alive++
		}
		s.LifeMean += t.Life
	}
	n := float64(len(p.Tiles))
	s.AvgTemp /= n
	s.LifeCover = float64(alive) / n
	s.LifeMean /= n
	return s
}
