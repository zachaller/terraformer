package sim

import "math"

// TileKind is a tile's terrain type, from its elevation.
type TileKind uint8

const (
	Basin TileKind = iota
	Plains
	Highlands
	Peaks
)

func (k TileKind) String() string {
	switch k {
	case Basin:
		return "Basin"
	case Plains:
		return "Plains"
	case Highlands:
		return "Highlands"
	case Peaks:
		return "Peaks"
	}
	return "Unknown"
}

// Mineral is what a tile's deposit is made of.
type Mineral uint8

const (
	Iron     Mineral = iota // smelted into metal
	Fluorite                // made into greenhouse gases
	Nitrate                 // cracked into nitrogen
)

func (m Mineral) String() string {
	switch m {
	case Iron:
		return "Iron ore"
	case Fluorite:
		return "Fluorite"
	case Nitrate:
		return "Nitrate"
	}
	return "?"
}

// Resource is the resource a mine on this mineral produces.
func (m Mineral) Resource() Resource {
	switch m {
	case Fluorite:
		return FluoriteRes
	case Nitrate:
		return NitrateRes
	}
	return Ore
}

// Tile is the state of one cell.
type Tile struct {
	// Height is terrain elevation, roughly in [-1, 1].
	Height float64
	// Temp is surface temperature in °C.
	Temp float64
	// Heat is fixed extra warming set with AddHeat (used for demos and
	// tests), in °C added to the tile's equilibrium temperature. Heater
	// buildings add their warmth separately; see Planet.TotalHeat.
	Heat float64
	// Ice and Water are amounts of frozen and liquid water, as a depth.
	Ice, Water float64
	// DryIce is frozen CO₂ (polar caps), as a depth.
	DryIce float64
	// Adsorbed is the share, 0 to 1, of the tile's regolith CO₂ still held
	// in the ground. Warming drives it out into the air.
	Adsorbed float64
	// Dust darkens ice, from 0 to 1, so it absorbs more sunlight.
	Dust float64
	// Soil is how developed the soil is, from 0 (bare regolith) to 1.
	Soil float64
	// Life is microbial coverage in [0, 1].
	Life float64
	// Ore is the size of the tile's mineral deposit (of Mineral), dug out by
	// mines; 0 means none.
	Ore     float64
	Mineral Mineral
	// Volcanic tiles can host geothermal plants.
	Volcanic bool
	// Pollution from industry; life can't survive where it's high.
	Pollution float64
	// Bio is the density, 0 to 1, of each species that follows microbes.
	Bio [NumSpecies]float64
}

// Kind classifies the tile's terrain by elevation.
func (t Tile) Kind() TileKind {
	switch {
	case t.Height < -0.15:
		return Basin
	case t.Height < 0.1:
		return Plains
	case t.Height < 0.3:
		return Highlands
	default:
		return Peaks
	}
}

// Planet is a whole world: its grid, every tile, and planet-wide state.
type Planet struct {
	Grid Grid
	Seed uint64
	// Level is how hard the planet is: 0 for the first, one more for each
	// planet brought to life before it.
	Level int
	Tiles []Tile // indexed by Grid.Index

	// Air is the atmosphere's composition.
	Air Air
	// Vapour is the water held in the air, as a total depth summed over
	// tiles, which falls again as rain or snow.
	Vapour float64
	// Ticks is the number of simulation steps run so far.
	Ticks uint64

	neighbors [][]int32 // indices of each tile's neighbours (shared with the Grid)
	dirs      []Vec3    // unit direction through each tile's centre
	cosLat    []float64 // cosine of each tile's latitude
	// baseTemp is each tile's temperature when the planet was generated;
	// regolith releases CO₂ as tiles warm above it.
	baseTemp []float64
	// co2PerDryIce is the CO₂ (kPa) one unit of dry ice holds, and
	// regolithPerTile the CO₂ (kPa) each tile's regolith holds at the start.
	co2PerDryIce, regolithPerTile float64
	// greenhouse is this step's greenhouse warming, °C, and heatShare how
	// much heat the air carries from equator to poles; see updateClimate.
	greenhouse, heatShare float64
	// frostShift raises the CO₂ frost point to the game's temperature
	// scale; see FrostPoint.
	frostShift float64

	eco economy
	// goalsDone is how many of Goals have been completed, in order.
	goalsDone int
	// startAvgTemp is the average temperature at generation, for the Thaw
	// goal's progress.
	startAvgTemp float64
	// startDryIce is the total dry ice at generation.
	startDryIce float64
	// photosynthesis, respiration, carbonates and outgassing are the CO₂
	// (kPa) plants turned into O₂, life burnt back into CO₂, seas locked
	// away and volcanoes breathed out, last step.
	photosynthesis, respiration, carbonates, outgassing float64
	// volcanoes is how many volcanic tiles there are.
	volcanoes int
	// orbitals are the projects launched into orbit.
	orbitals []orbital
	// trialStart is the step (plus one) the final goal's trial began, or 0
	// while it isn't running.
	trialStart uint64
	// built counts buildings built, and wonAt is the step the final goal
	// was completed (0 before), for the closing stats.
	built int
	wonAt uint64
	// liftedOff is set once the lander has left the finished planet.
	liftedOff bool
	// solettaPower is how far the Soletta's mirrors are turned towards the
	// planet, from 0 to 1.
	solettaPower float64
	// impacts counts comets that have struck, for goals.
	impacts int

	// Scratch buffers reused by Step.
	nextTemp, nextLife, flow, evaporated []float64
	nextBio                              [][NumSpecies]float64
}

// settleSteps is how many temperature steps a new planet's climate runs to
// settle before play.
const settleSteps = 600

// The starting atmosphere, roughly Mars today but a little thinner, so the
// first hurdle is raising the pressure past water's triple point.
var startAir = Air{CO2: 0.45, N2: 0.012, O2: 0.0006}

const (
	// totalDryIceCO2 is the CO₂ (kPa) the polar caps hold, and
	// totalRegolithCO2 what the regolith holds, released as it warms.
	totalDryIceCO2   = 15
	totalRegolithCO2 = 45
	// A tile's regolith starts releasing CO₂ once it's regolithThreshold °C
	// warmer than at the start, and has released it all regolithSpan °C
	// beyond that.
	regolithThreshold = 8
	regolithSpan      = 60
)

// NewPlanet generates a frozen, barren starting planet from seed. The same
// seed and size always give the same planet.
func NewPlanet(n int, seed uint64) *Planet { return NewPlanetLevel(n, seed, 0) }

// NewPlanetLevel is NewPlanet at a difficulty level: each level makes the
// planet colder, with less ice and smaller, sparser deposits.
func NewPlanetLevel(n int, seed uint64, level int) *Planet {
	g := Grid{N: n}
	cells := g.NumCells()
	level = max(level, 0)
	p := &Planet{
		Grid:       g,
		Seed:       seed,
		Level:      level,
		Tiles:      make([]Tile, cells),
		Air:        startAir,
		neighbors:  make([][]int32, cells),
		dirs:       make([]Vec3, cells),
		cosLat:     make([]float64, cells),
		baseTemp:   make([]float64, cells),
		nextTemp:   make([]float64, cells),
		nextLife:   make([]float64, cells),
		flow:       make([]float64, cells),
		evaporated: make([]float64, cells),
		nextBio:    make([][NumSpecies]float64, cells),
		eco:        newEconomy(cells),
	}
	totalDry := 0.0
	for idx := range p.Tiles {
		c := g.CellAt(idx)
		p.neighbors[idx] = g.NeighborIndices(idx)
		d := g.DirFromCell(c)
		p.dirs[idx] = d
		p.cosLat[idx] = math.Sqrt(1 - d.Y*d.Y)

		h := fbm(seed, d.Scale(2.2), 5)
		t := Tile{Height: h, Ice: startingIce(seed, d, h) * p.levelScale(0.12), DryIce: startingDryIce(seed, d, h), Adsorbed: 1}
		t.Ore, t.Mineral = startingDeposit(seed, d, h)
		if t.Ore > 0 && level > 0 {
			// Sparser deposits: drop some outright, shrink the rest.
			if fbm(seed^0xD15, d.Scale(9), 2) < -0.6+0.15*float64(level) {
				t.Ore = 0
			}
			t.Ore *= p.levelScale(0.12)
		}
		t.Volcanic = h > 0.12 && fbm(seed^0x7A11, d.Scale(5), 3) > 0.48
		if t.Volcanic {
			p.volcanoes++
		}
		p.Tiles[idx] = t
		totalDry += t.DryIce
	}
	p.co2PerDryIce = totalDryIceCO2 / max(totalDry, 1e-9)
	p.regolithPerTile = totalRegolithCO2 / float64(cells)
	// Settle temperatures (including heat spreading) and the air into
	// balance with the polar caps before play.
	p.updateClimate()
	for idx := range p.Tiles {
		p.Tiles[idx].Temp = p.equilibrium(idx)
	}
	for range settleSteps {
		p.stepTemperature()
	}
	// Real dry ice would be far colder than the game's caps, so the frost
	// point is shifted to match: the caps hold exactly the starting CO₂.
	capT, _ := p.capTemp()
	p.frostShift = capT - frostPoint(startAir.CO2, 0)
	for idx := range p.Tiles {
		p.baseTemp[idx] = p.Tiles[idx].Temp
	}
	st := p.Stats()
	p.startAvgTemp, p.startDryIce = st.AvgTemp, st.DryIce
	return p
}

// Dir is the unit direction through the centre of c, like Grid.DirFromCell
// but precomputed.
func (p *Planet) Dir(c Cell) Vec3 { return p.dirs[p.Grid.Index(c)] }

// Tile returns the tile at c.
func (p *Planet) Tile(c Cell) Tile { return p.Tiles[p.Grid.Index(c)] }

// startingDeposit scatters mineral deposits: iron ore over roughly a tenth
// of the planet, and rarer fluorite (in the uplands) and nitrates (in the
// lowlands).
func startingDeposit(seed uint64, d Vec3, h float64) (float64, Mineral) {
	if n := fbm(seed^0x0BE, d.Scale(6), 3); n >= 0.3 {
		return 100 + 400*(n-0.3), Iron
	}
	if n := fbm(seed^0xF1, d.Scale(7), 3); n >= 0.38 && h > -0.05 {
		return 150 + 600*(n-0.38), Fluorite
	}
	if n := fbm(seed^0x17A, d.Scale(7), 3); n >= 0.36 && h < 0.05 {
		return 150 + 600*(n-0.36), Nitrate
	}
	return 0, Iron
}

// startingDryIce puts frozen CO₂ in the middle of the polar caps.
func startingDryIce(seed uint64, d Vec3, h float64) float64 {
	edge := 0.95 - 0.04*fbm(seed^0xD1, d.Scale(4), 3) - 0.05*max(h, 0)
	if math.Abs(d.Y) > edge {
		return 1
	}
	return 0
}

// startingIce places polar caps with a ragged edge, snow on peaks, and a
// little permafrost everywhere else (most in low basins).
// levelScale is how much of something a planet has at its level, losing
// share per level, down to half.
func (p *Planet) levelScale(share float64) float64 {
	return max(1-share*float64(p.Level), 0.5)
}

func startingIce(seed uint64, d Vec3, h float64) float64 {
	edge := 0.82 - 0.08*fbm(seed^0xC3, d.Scale(4), 3) - 0.15*max(h, 0)
	switch {
	case math.Abs(d.Y) > edge:
		return 1.5
	case h >= 0.35:
		return 0.4
	case h < -0.15:
		return 0.3
	case h < 0.1:
		return 0.12
	default:
		return 0.08
	}
}
