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

	sublimateRate = 0.05   // share of a tile's liquid that evaporates per step below the triple point
	boilRate      = 0.002  // water boiled off per °C above boiling, per step
	regolithRate  = 0.0001 // share of the gap to equilibrium regolith CO₂ moves per step
	evapRate      = 0.0005 // share of standing water that evaporates per step at 15 °C
	landEvapRate  = 0.0005 // share of the ground's moisture that evaporates per step at 15 °C
	localRain     = 0.7    // share of rain that falls back where it evaporated
	precipRate    = 0.02   // share of the air's water vapour that falls per step

	// albedoK is °C of warming per unit less sunlight reflected, relative
	// to refAlbedo (dusty plains).
	albedoK   = 45.0
	refAlbedo = 0.25
)

// Step advances the simulation by one tick. It is deterministic: the same
// planet state always produces the same next state.
func (p *Planet) Step() {
	p.stepEconomy()
	p.stepAir()
	p.stepTemperature()
	p.stepIce()
	p.stepDryIce()
	p.stepRegolith()
	p.stepWaterCycle()
	p.stepGlaciers()
	p.stepWater()
	p.stepPollution()
	p.stepLife()
	p.stepSpecies()
	p.stepBiosphere()
	p.Ticks++
	p.checkGoals()
}

// equilibrium is the temperature tile idx settles towards without machines:
// warmest at the equator (less so under thick air, which carries heat
// towards the poles), warmer with more greenhouse gases, colder up high,
// and colder where the surface is bright and reflects sunlight away.
func (p *Planet) equilibrium(idx int) float64 {
	t := &p.Tiles[idx]
	s := p.heatShare
	sun := 70 * ((1-0.5*s)*p.cosLat[idx] + 0.5*s*math.Pi/4)
	return -85 + sun + p.greenhouse - 15*max(t.Height, 0) + albedoK*(refAlbedo-Albedo(t))
}

var terrainAlbedo = [...]float64{Basin: 0.16, Plains: 0.25, Highlands: 0.27, Peaks: 0.3}

// Albedo is the share of sunlight tile t reflects: dark basalt basins and
// open water absorb most of it, ice reflects half (less when dusty), and
// plants darken the ground.
func Albedo(t *Tile) float64 {
	a := terrainAlbedo[t.Kind()]
	veg := max(t.Bio[Moss]*0.5, max(t.Bio[Grass], max(t.Bio[Shrubs], t.Bio[Forest])))
	a += (0.14 - a) * veg
	a += (0.07 - a) * clamp01((t.Water-soilRetention)*4)
	a += (0.5 - 0.3*t.Dust - a) * clamp01(t.Ice*2+t.DryIce*2)
	return a
}

func clamp01(v float64) float64 { return min(max(v, 0), 1) }

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

// stepIce melts and freezes water. Below the triple point there's no
// liquid: warmed ice sublimates into the air instead, and any liquid left
// evaporates. Above the boiling point, water boils away.
func (p *Planet) stepIce() {
	liquid := p.Pressure() >= TriplePoint
	boil := p.BoilingPoint()
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		switch {
		case t.Temp > 0 && t.Ice > 0:
			m := min(t.Ice, meltRate*t.Temp)
			t.Ice -= m
			if liquid {
				t.Water += m
			} else {
				p.Vapour += m
			}
		case t.Temp < 0 && t.Water > 0:
			f := min(t.Water, freezeRate*-t.Temp)
			t.Water -= f
			t.Ice += f
		}
		if t.Water > 0 && (!liquid || t.Temp > boil) {
			e := t.Water * sublimateRate
			if liquid {
				e = min(t.Water, boilRate*(t.Temp-boil))
			}
			t.Water -= e
			p.Vapour += e
		}
	}
}

// stepDryIce keeps the polar caps and the air in balance, as on Mars: the
// caps hold the air's CO₂ at the pressure where it would freeze at their
// average temperature (see CapCO2). Warmer caps release dry ice until the
// air reaches it (the warmest parts first); extra CO₂ from elsewhere slowly
// freezes back onto them.
func (p *Planet) stepDryIce() {
	capT, total := p.capTemp()
	if total < 1e-9 {
		return
	}
	gap := p.capCO2(capT) - p.Air.CO2
	var units float64 // dry ice to sublimate (negative: to freeze)
	if gap > 0 {
		units = min(capRate*gap/p.co2PerDryIce, total)
	} else {
		units = capRate * 0.1 * gap / p.co2PerDryIce
	}
	// Share it out: sublimation from the warmest dry ice first, freezing
	// in proportion to what's there.
	weight := func(t *Tile) float64 {
		if units > 0 {
			return t.DryIce * math.Exp((t.Temp-capT)/8)
		}
		return t.DryIce
	}
	sum := 0.0
	for idx := range p.Tiles {
		if t := &p.Tiles[idx]; t.DryIce > 0 {
			sum += weight(t)
		}
	}
	done := 0.0
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		if t.DryIce <= 0 {
			continue
		}
		d := min(units*weight(t)/sum, t.DryIce)
		t.DryIce -= d
		done += d
	}
	p.Air.CO2 += done * p.co2PerDryIce
}

// capTemp returns the polar caps' average temperature, weighted by dry ice,
// and how much dry ice there is.
func (p *Planet) capTemp() (float64, float64) {
	temp, total := 0.0, 0.0
	for i := range p.Tiles {
		t := &p.Tiles[i]
		if t.DryIce > 0 {
			temp += t.Temp * t.DryIce
			total += t.DryIce
		}
	}
	if total == 0 {
		return 0, 0
	}
	return temp / total, total
}

// CapCO2 is the CO₂ pressure (kPa) the polar caps hold the air at, from
// their temperature: the inverse of the frost point. It's 0 once the caps
// are gone.
func (p *Planet) CapCO2() float64 {
	t, total := p.capTemp()
	if total == 0 {
		return 0
	}
	return p.capCO2(t)
}

func (p *Planet) capCO2(temp float64) float64 {
	k := temp - p.frostShift + 273.15
	return 101.3 * math.Exp(3165*(1/194.7-1/k))
}

const capRate = 0.00025 // share of the gap between the air and the caps' balance closed per step

// stepRegolith releases CO₂ from the ground as tiles warm above their
// starting temperature, and lets it soak back slowly as they cool.
func (p *Planet) stepRegolith() {
	gained := 0.0
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		eq := clamp01(1 - (t.Temp-p.baseTemp[idx]-regolithThreshold)/regolithSpan)
		switch {
		case t.Adsorbed > eq:
			d := regolithRate * (t.Adsorbed - eq)
			t.Adsorbed -= d
			gained += d
		case t.Adsorbed < eq && p.Air.CO2 > 0.5:
			d := regolithRate * 0.1 * (eq - t.Adsorbed)
			t.Adsorbed += d
			gained -= d
		}
	}
	p.Air.CO2 += gained * p.regolithPerTile
}

// stepWaterCycle evaporates water (faster when warm; standing water much
// faster than damp ground) into the air, and lets vapour fall again as rain,
// or as snow where it's freezing. Most falls near where it rose, as over
// Earth's oceans; the rest spreads over the planet, more on high ground.
// Below the triple point it can only fall as frost on freezing ground.
func (p *Planet) stepWaterCycle() {
	liquid := p.Pressure() >= TriplePoint
	evap := 0.0
	clear(p.evaporated)
	if liquid {
		for idx := range p.Tiles {
			t := &p.Tiles[idx]
			if t.Water <= 0 || t.Temp <= 0 {
				continue
			}
			warm := math.Exp(0.07 * (t.Temp - 15))
			free := max(t.Water-soilRetention, 0)
			e := min(free*0.05, evapRate*free*warm) + landEvapRate*min(t.Water, soilRetention)*warm
			e = min(e, t.Water)
			t.Water -= e
			p.Vapour += e
			p.evaporated[idx] = e
			evap += e
		}
	}
	fall := p.Vapour * precipRate
	local := 0.0
	if evap > 0 {
		local = fall * localRain
	}
	total := 0.0
	for idx := range p.Tiles {
		total += p.rainWeight(idx, liquid)
	}
	if total == 0 && local == 0 {
		return
	}
	spread := fall - local
	if total == 0 {
		spread, local = 0, fall
	}
	for idx := range p.Tiles {
		amount := 0.0
		if w := p.rainWeight(idx, liquid); w > 0 {
			amount += spread * w / total
		}
		if local > 0 {
			amount += local * p.evaporated[idx] / evap
		}
		if amount == 0 {
			continue
		}
		if t := &p.Tiles[idx]; t.Temp < 0 {
			t.Ice += amount
		} else {
			t.Water += amount
		}
	}
	p.Vapour -= fall
}

// rainWeight is tile idx's share of rain or snow: more where it's warm
// (warm air holds more moisture; the poles are deserts) and on high ground.
func (p *Planet) rainWeight(idx int, liquid bool) float64 {
	t := &p.Tiles[idx]
	if !liquid && t.Temp >= 0 {
		return 0
	}
	return (1 + max(t.Height, 0)) * math.Exp(0.05*min(t.Temp, 30))
}

// stepGlaciers lets thick ice creep towards thinner ice next to it, so ice
// sheets spread out to where it's warm enough to melt them instead of
// trapping the planet's water at the poles.
func (p *Planet) stepGlaciers() {
	clear(p.flow)
	for a := range p.Tiles {
		ta := &p.Tiles[a]
		if ta.Ice <= glacierMin {
			continue
		}
		for _, nb := range p.neighbors[a] {
			if d := ta.Ice - p.Tiles[nb].Ice; d > 0 {
				f := min(d*glacierRate, (ta.Ice-glacierMin)/float64(len(p.neighbors[a])))
				p.flow[a] -= f
				p.flow[nb] += f
			}
		}
	}
	for idx := range p.Tiles {
		p.Tiles[idx].Ice = max(p.Tiles[idx].Ice+p.flow[idx], 0)
	}
}

const (
	glacierMin  = 1.6   // ice thinner than this doesn't flow (thicker than the starting caps)
	glacierRate = 0.002 // share of the difference in thickness that flows per step
)

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
			if ta.Water <= soilRetention && tb.Water <= soilRetention {
				continue // neither has water free to flow
			}
			diff := (ta.Height*elevScale + ta.Water) - (tb.Height*elevScale + tb.Water)
			var f float64
			if diff > 0 {
				f = min(diff*flowRate, freeWater(ta)*maxFlowShare)
			} else {
				f = -min(-diff*flowRate, freeWater(tb)*maxFlowShare)
			}
			p.flow[a] -= f
			p.flow[b] += f
		}
	}
	for idx := range p.Tiles {
		p.Tiles[idx].Water = max(p.Tiles[idx].Water+p.flow[idx], 0)
	}
}

func freeWater(t *Tile) float64 { return max(t.Water-soilRetention, 0) }

// TotalHeat is the warming on tile idx from heaters this step, plus any
// fixed extra heat set with AddHeat.
func (p *Planet) TotalHeat(idx int) float64 {
	return p.Tiles[idx].Heat + p.eco.machineHeat[idx]
}

// stepPollution lets pollution drift to neighbours and slowly break down.
func (p *Planet) stepPollution() {
	clear(p.flow)
	for idx := range p.Tiles {
		if p.Tiles[idx].Pollution == 0 {
			continue
		}
		out := p.Tiles[idx].Pollution * pollutionSpread / float64(len(p.neighbors[idx]))
		for _, nb := range p.neighbors[idx] {
			p.flow[nb] += out
			p.flow[idx] -= out
		}
	}
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		t.Pollution = max(t.Pollution+p.flow[idx]-pollutionDecay, 0)
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
			p.nextLife[idx] = max(l-lifeDieback, 0)
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
		p.nextLife[idx] = min(l, 1)
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

// HeatRings estimates how much warmer a heater putting out heat keeps its
// own tile (ring 0) and each ring of tiles around it, out to rings, once
// temperatures have settled. Each tile settles where stepTemperature's pull
// towards its target (raised by heat on the heater's tile and by half that
// on its neighbours) balances the pull towards its neighbours' average.
// Feedbacks such as melting ice aren't included.
func (g Grid) HeatRings(heat float64, rings int) []float64 {
	// A tile well away from the pentagons, and the patch of tiles around
	// it, far enough out that the warming there is negligible.
	c := g.Index(g.LatticeCell(0, g.N/2, g.N/2))
	ring := map[int]int{c: 0}
	frontier := []int{c}
	for r := 1; r <= rings+8; r++ {
		var next []int
		for _, f := range frontier {
			for _, nb := range g.NeighborIndices(f) {
				if _, ok := ring[int(nb)]; !ok {
					ring[int(nb)] = r
					next = append(next, int(nb))
				}
			}
		}
		frontier = next
	}
	x, nx := map[int]float64{}, map[int]float64{}
	for range 400 {
		for idx, r := range ring {
			h := 0.0
			switch r {
			case 0:
				h = heat
			case 1:
				h = heat / 2
			}
			nbs := g.NeighborIndices(idx)
			avg := 0.0
			for _, nb := range nbs {
				avg += x[int(nb)]
			}
			avg /= float64(len(nbs))
			nx[idx] = (tempRelax*h + tempDiffusion*avg) / (tempRelax + tempDiffusion)
		}
		x, nx = nx, x
	}
	sum, count := make([]float64, rings+1), make([]float64, rings+1)
	for idx, r := range ring {
		if r <= rings {
			sum[r] += x[idx]
			count[r]++
		}
	}
	for r := range sum {
		sum[r] /= count[r]
	}
	return sum
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
	t.Heat = min(t.Heat+amount, maxHeat)
}

// Stats summarises the whole planet.
type Stats struct {
	AvgTemp    float64
	Ice, Water float64 // totals
	LifeCover  float64 // fraction of tiles with any life
	LifeMean   float64 // average life coverage over all tiles
	DryIce     float64 // total dry ice
	SeaCover   float64 // fraction of tiles under standing water
}

func (p *Planet) Stats() Stats {
	var s Stats
	alive, wet := 0, 0
	for i := range p.Tiles {
		t := &p.Tiles[i]
		s.AvgTemp += t.Temp
		s.Ice += t.Ice
		s.Water += t.Water
		s.DryIce += t.DryIce
		if t.Life > 0 {
			alive++
		}
		if t.Water >= standingWater {
			wet++
		}
		s.LifeMean += t.Life
	}
	n := float64(len(p.Tiles))
	s.AvgTemp /= n
	s.LifeCover = float64(alive) / n
	s.LifeMean /= n
	s.SeaCover = float64(wet) / n
	return s
}
