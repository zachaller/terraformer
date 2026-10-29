package sim

import (
	"fmt"
)

// Species are the life forms that follow microbes (Tile.Life). Each needs
// the one before it in its chain to be established on the same tile first,
// so life spreads across the planet in stages.
type Species int

const (
	Moss Species = iota
	Grass
	Shrubs
	Forest
	Plankton
	Fish
	Grazers
	NumSpecies
)

func (s Species) String() string { return SpeciesSpecs[s].Name }

// SpeciesSpec is what a species needs to live on a tile.
type SpeciesSpec struct {
	Name string
	// After is the species that must be established (density at least
	// establishedAt) on a tile before this one can appear there. -1 means
	// microbes; -2 means nothing.
	After   Species
	MinTemp float64
	MinAir  float64 // kPa
	// Aquatic species need standing water; land species need damp ground
	// that isn't flooded.
	Aquatic  bool
	MinWater float64
	// MinN2 and MinO2 are partial pressures (kPa) of nitrogen and oxygen it
	// needs; MinOzone how well shielded from ultraviolet light the surface
	// must be (see Planet.Ozone); MinSoil how developed the soil.
	MinN2, MinO2, MinOzone, MinSoil float64
	Growth                          float64 // logistic growth per step
	// Photosynthesis is how much CO₂ it turns into O₂ at full density,
	// relative to forest.
	Photosynthesis float64
}

const (
	afterMicrobes Species = -1
	afterNothing  Species = -2

	establishedAt  = 0.5  // density at which a species lets the next one in
	standingWater  = 0.2  // water depth that counts as flooded or sea
	maxSpeciesTemp = 45.0 // too hot for any of them
	maxSpeciesPoll = 0.2
	speciesSpread  = 0.002
	speciesEmerge  = 0.0002 // chance per step on a tile ready for a species
	speciesDieback = 0.005
	speciesSeed    = 0.05
)

// SpeciesSpecs is indexed by Species.
var SpeciesSpecs = [NumSpecies]SpeciesSpec{
	Moss:     {Name: "Moss", After: afterMicrobes, MinTemp: 0, MinAir: 1, MinWater: minLifeWater, Growth: 0.0015, Photosynthesis: 0.3},
	Grass:    {Name: "Grass", After: Moss, MinTemp: 5, MinAir: 8, MinWater: minLifeWater, MinN2: 1, MinOzone: 0.3, MinSoil: 0.2, Growth: 0.00125, Photosynthesis: 0.6},
	Shrubs:   {Name: "Shrubs", After: Grass, MinTemp: 8, MinAir: 15, MinWater: 0.03, MinN2: 2, MinOzone: 0.5, MinSoil: 0.4, Growth: 0.001, Photosynthesis: 0.8},
	Forest:   {Name: "Forest", After: Shrubs, MinTemp: 10, MinAir: 20, MinWater: 0.03, MinN2: 3, MinOzone: 0.7, MinSoil: 0.6, Growth: 0.00075, Photosynthesis: 1},
	Plankton: {Name: "Plankton", After: afterNothing, MinTemp: 2, MinAir: 1, Aquatic: true, MinWater: standingWater, Growth: 0.002, Photosynthesis: 0.7},
	Fish:     {Name: "Fish", After: Plankton, MinTemp: 8, MinAir: 10, Aquatic: true, MinWater: standingWater, MinO2: 4, Growth: 0.001},
	Grazers:  {Name: "Grazers", After: Grass, MinTemp: 3, MinAir: 20, MinWater: minLifeWater, MinO2: 10, MinOzone: 0.8, Growth: 0.001},
}

// Requirement is one condition a species needs, for display.
type Requirement struct {
	Label      string
	OK         bool
	Have, Need string
}

// Requirements lists what species s needs on tile t, and whether each is met.
func (p *Planet) Requirements(s Species, t Tile) []Requirement {
	spec := SpeciesSpecs[s]
	var reqs []Requirement
	switch spec.After {
	case afterMicrobes:
		reqs = append(reqs, Requirement{"Microbes established", t.Life >= establishedAt, pct(t.Life), pct(establishedAt)})
	case afterNothing:
	default:
		reqs = append(reqs, Requirement{SpeciesSpecs[spec.After].Name + " established", t.Bio[spec.After] >= establishedAt, pct(t.Bio[spec.After]), pct(establishedAt)})
	}
	reqs = append(reqs, Requirement{"Warm enough", t.Temp > spec.MinTemp && t.Temp < maxSpeciesTemp, fmt.Sprintf("%.1f°", t.Temp), fmt.Sprintf("above %.0f°", spec.MinTemp)})
	if spec.MinAir > 0 {
		reqs = append(reqs, Requirement{"Thick enough air", p.Pressure() >= spec.MinAir, fmt.Sprintf("%.1f kPa", p.Pressure()), fmt.Sprintf("%.0f kPa", spec.MinAir)})
	}
	if spec.MinN2 > 0 {
		reqs = append(reqs, Requirement{"Nitrogen", p.Air.N2 >= spec.MinN2, fmt.Sprintf("%.1f kPa", p.Air.N2), fmt.Sprintf("%.0f kPa", spec.MinN2)})
	}
	if spec.MinO2 > 0 {
		reqs = append(reqs, Requirement{"Oxygen", p.Air.O2 >= spec.MinO2, fmt.Sprintf("%.1f kPa", p.Air.O2), fmt.Sprintf("%.0f kPa", spec.MinO2)})
	}
	if spec.MinOzone > 0 {
		reqs = append(reqs, Requirement{"Ozone shield", p.Ozone() >= spec.MinOzone, pct(p.Ozone()), pct(spec.MinOzone)})
	}
	if spec.MinSoil > 0 {
		reqs = append(reqs, Requirement{"Soil", t.Soil >= spec.MinSoil, pct(t.Soil), pct(spec.MinSoil)})
	}
	if spec.Aquatic {
		reqs = append(reqs, Requirement{"Standing water", t.Water >= spec.MinWater, fmt.Sprintf("%.2f", t.Water), fmt.Sprintf("%.2f", spec.MinWater)})
	} else {
		reqs = append(reqs, Requirement{"Damp, not flooded", t.Water >= spec.MinWater && t.Water < standingWater, fmt.Sprintf("%.2f", t.Water), fmt.Sprintf("%.2f–%.2f", spec.MinWater, standingWater)})
	}
	reqs = append(reqs, Requirement{"Clean enough", t.Pollution < maxSpeciesPoll, fmt.Sprintf("%.2f", t.Pollution), fmt.Sprintf("under %.2f", maxSpeciesPoll)})
	return reqs
}

func pct(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

// canLive reports whether species s can live on tile t (prerequisite aside).
func (p *Planet) canLive(s Species, t *Tile) bool {
	spec := &SpeciesSpecs[s]
	if t.Temp <= spec.MinTemp || t.Temp >= maxSpeciesTemp || p.Pressure() < spec.MinAir || t.Pollution >= maxSpeciesPoll ||
		p.Air.N2 < spec.MinN2 || p.Air.O2 < spec.MinO2 || p.Ozone() < spec.MinOzone || t.Soil < spec.MinSoil {
		return false
	}
	if spec.Aquatic {
		return t.Water >= spec.MinWater
	}
	return t.Water >= spec.MinWater && t.Water < standingWater
}

// established reports whether the species s follows is established on t.
func established(s Species, t *Tile) bool {
	switch after := SpeciesSpecs[s].After; after {
	case afterMicrobes:
		return t.Life >= establishedAt
	case afterNothing:
		return true
	default:
		return t.Bio[after] >= establishedAt
	}
}

// NextSpecies is the next species that could come to tile t: the first in
// its chain (land or water) not yet there, or false if every stage is.
func NextSpecies(t Tile) (Species, bool) {
	aquatic := t.Water >= standingWater
	for s := range NumSpecies {
		if SpeciesSpecs[s].Aquatic == aquatic && t.Bio[s] == 0 {
			return s, true
		}
	}
	return 0, false
}

// stepSpecies grows, spreads, seeds and kills off every species, like
// stepLife does for microbes.
func (p *Planet) stepSpecies() {
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		for s := range NumSpecies {
			l := t.Bio[s]
			if !p.canLive(s, t) {
				p.nextBio[idx][s] = max(l-speciesDieback, 0)
				continue
			}
			if !established(s, t) {
				p.nextBio[idx][s] = l // waiting for the stage before it
				continue
			}
			avg := 0.0
			for _, nb := range p.neighbors[idx] {
				avg += p.Tiles[nb].Bio[s]
			}
			avg /= float64(len(p.neighbors[idx]))
			l += SpeciesSpecs[s].Growth*l*(1-l) + speciesSpread*avg*(1-l)
			if l == 0 && p.chance(idx, int(s)+1) < speciesEmerge*p.seedBoost(idx) {
				l = speciesSeed
			}
			p.nextBio[idx][s] = min(l, 1)
		}
	}
	for idx := range p.Tiles {
		p.Tiles[idx].Bio = p.nextBio[idx]
	}
}

// SpeciesCover is the fraction of tiles where species s has a foothold.
func (p *Planet) SpeciesCover(s Species) float64 {
	n := 0
	for i := range p.Tiles {
		if p.Tiles[i].Bio[s] > 0.05 {
			n++
		}
	}
	return float64(n) / float64(len(p.Tiles))
}

// seedBoost multiplies the chance of life appearing on tile idx: much higher
// near a running biolab.
func (p *Planet) seedBoost(idx int) float64 {
	if p.eco.seeded[idx] {
		return biolabBoost
	}
	return 1
}

const (
	biolabBoost = 15
	// soilRate is how fast life builds soil, per step at full cover.
	soilRate = 0.0001
	// photoK is the O₂ (kPa) one tile of full forest makes from CO₂ per step.
	photoK = 1.5e-8
	// carbonateK is the CO₂ (kPa) one tile of standing water locks into
	// carbonate rock per step, when CO₂ is plentiful.
	carbonateK = 3e-7
)

// stepBiosphere lets life build soil, turns CO₂ into O₂ by photosynthesis,
// and lets seas lock CO₂ away as carbonate rock.
func (p *Planet) stepBiosphere() {
	photo, sea := 0.0, 0.0
	for idx := range p.Tiles {
		t := &p.Tiles[idx]
		build := 0.3*t.Life + t.Bio[Moss] + 0.5*t.Bio[Grass] + 0.3*(t.Bio[Shrubs]+t.Bio[Forest])
		t.Soil += soilRate * build * (1 - t.Soil)
		photo += 0.2 * t.Life
		for s := range NumSpecies {
			photo += t.Bio[s] * SpeciesSpecs[s].Photosynthesis
		}
		sea += clamp01((t.Water - soilRetention) * 4)
	}
	co2 := p.Air.CO2
	made := photoK * photo * co2 / (co2 + 0.5)
	locked := carbonateK * sea * co2 / (co2 + 10)
	made = min(made, co2*0.001)
	locked = min(locked, co2*0.001)
	p.Air.CO2 -= made + locked
	p.Air.O2 += made
	p.photosynthesis, p.carbonates = made, locked
}
