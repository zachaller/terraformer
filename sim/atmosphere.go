package sim

import "math"

// Air is the atmosphere's composition, as partial pressures in kPa (PFC in
// Pa, since tiny amounts matter). Earth's air is about 79 kPa N₂, 21 kPa
// O₂ and 0.04 kPa CO₂; Mars today has about 0.6 kPa, nearly all CO₂.
type Air struct {
	CO2 float64 // carbon dioxide: the main greenhouse gas at first
	N2  float64 // nitrogen: inert, but plants need it and it adds pressure
	O2  float64 // oxygen: forms an ozone layer, and animals breathe it
	PFC float64 // perfluorocarbons, in Pa: factory-made super-greenhouse gases
}

// Pressure is total surface pressure in kPa.
func (a Air) Pressure() float64 { return a.CO2 + a.N2 + a.O2 + a.PFC/1000 }

// Pressure is the planet's total surface pressure in kPa.
func (p *Planet) Pressure() float64 { return p.Air.Pressure() }

// Physical constants and tuning for the atmosphere. Timescales are
// compressed so a planet can be terraformed in hours rather than centuries,
// but the mechanisms, and roughly the thresholds, are real.
const (
	// TriplePoint is the pressure, kPa, below which water can't be liquid:
	// ice warmed above 0 °C sublimates straight to vapour.
	TriplePoint = 0.62

	co2GreenhouseK = 7.0  // °C per doubling of (1 + CO₂ kPa)
	pfcGreenhouseK = 12.0 // °C per doubling of (1 + PFC Pa)
	broadeningK    = 1.5  // °C per doubling of (1 + pressure/10): thicker air traps more heat
	vapourK        = 10.0 // °C of warming at full humidity
	// vapourHalf is the global vapour (total water depth) at which humidity
	// is half its maximum.
	vapourHalf = 20.0

	pfcDecay   = 4e-5   // share of PFCs broken down by sunlight per step
	escapeRate = 2.5e-6 // share of the air lost to space per step without a magnetic shield

	// Ozone forms from oxygen; at OzoneFullO2 kPa of O₂ the layer fully
	// shields the surface from ultraviolet light.
	OzoneFullO2 = 2.0
)

// Greenhouse is the planet's warming from its atmosphere, in °C, by cause.
type Greenhouse struct {
	CO2, PFC, Broadening, Vapour float64
}

// Total is the overall greenhouse warming in °C.
func (g Greenhouse) Total() float64 { return g.CO2 + g.PFC + g.Broadening + g.Vapour }

// Greenhouse works out the planet's current greenhouse warming.
func (p *Planet) Greenhouse() Greenhouse {
	return Greenhouse{
		CO2:        co2GreenhouseK * math.Log2(1+p.Air.CO2),
		PFC:        pfcGreenhouseK * math.Log2(1+p.Air.PFC),
		Broadening: broadeningK * math.Log2(1+p.Pressure()/10),
		Vapour:     vapourK * p.Humidity(),
	}
}

// Humidity is how moist the air is, from 0 to 1, from the water vapour in it.
func (p *Planet) Humidity() float64 { return p.Vapour / (p.Vapour + vapourHalf) }

// Ozone is how well the ozone layer shields the surface from ultraviolet
// light, from 0 (none) to 1.
func (p *Planet) Ozone() float64 { return min(p.Air.O2/OzoneFullO2, 1) }

// BoilingPoint is the temperature, °C, at which water boils at the current
// pressure (Clausius–Clapeyron): 100 °C at Earth's 101 kPa, 0 °C at the
// triple point.
func (p *Planet) BoilingPoint() float64 { return boilingPoint(p.Pressure()) }

func boilingPoint(kPa float64) float64 {
	if kPa <= TriplePoint {
		return 0
	}
	// 1/T = 1/373 K − (R/L)·ln(P/101.3 kPa), with L/R ≈ 4900 K for water.
	return 1/(1/373.15-math.Log(kPa/101.3)/4900) - 273.15
}

// FrostPoint is the temperature, °C, below which CO₂ freezes out of the air
// as dry ice, and above which dry ice sublimates. It rises with the CO₂
// pressure (−78.5 °C at Earth's sea-level pressure, about −125 °C on Mars).
// The game's temperatures run warmer than real Mars, so it's shifted to
// keep the polar caps where they'd really be: just stable.
func (p *Planet) FrostPoint() float64 { return frostPoint(p.Air.CO2, p.frostShift) }

func frostPoint(co2, shift float64) float64 {
	co2 = max(co2, 0.01)
	return 1/(1/194.7-math.Log(co2/101.3)/3165) - 273.15 + shift
}

// MagneticShield reports whether a magnetic shield is running at full
// power, which stops the air escaping to space.
func (p *Planet) MagneticShield() bool {
	for _, b := range p.eco.buildings {
		if b.Spec().Shield && b.Activity >= 0.999 {
			return true
		}
	}
	return false
}

// EscapePerDay is how much air, kPa, the planet is losing to space per day.
func (p *Planet) EscapePerDay() float64 {
	if p.MagneticShield() {
		return 0
	}
	return p.Pressure() * escapeRate * stepsPerDay
}

// stepsPerDay is how many simulation steps make an in-game day.
const stepsPerDay = 10

// stepAir updates the atmosphere for one step: PFCs break down, and without
// a shield a little of everything escapes to space.
func (p *Planet) stepAir() {
	p.Air.PFC *= 1 - pfcDecay
	if !p.MagneticShield() {
		k := 1 - escapeRate
		p.Air.CO2 *= k
		p.Air.N2 *= k
		p.Air.O2 *= k
	}
	p.updateClimate()
}

// updateClimate caches the planet-wide numbers each tile's temperature
// depends on: the greenhouse warming, and the share of the equator's heat
// the air carries towards the poles (more in thicker air).
func (p *Planet) updateClimate() {
	p.greenhouse = p.Greenhouse().Total()
	pr := p.Pressure()
	p.heatShare = pr / (pr + 60)
}

// DryIceLeft is the share of the starting polar dry ice still frozen.
func (p *Planet) DryIceLeft() float64 {
	if p.startDryIce == 0 {
		return 0
	}
	return p.Stats().DryIce / p.startDryIce
}
