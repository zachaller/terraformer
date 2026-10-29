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

// Tile is the state of one cell.
type Tile struct {
	// Height is terrain elevation, roughly in [-1, 1].
	Height float64
	// Temp is surface temperature in °C.
	Temp float64
	// Heat is extra warming from machines, in °C added to the tile's
	// equilibrium temperature.
	Heat float64
	// Ice and Water are amounts of frozen and liquid water, as a depth.
	Ice, Water float64
	// Life is microbial coverage in [0, 1].
	Life float64
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
	Grid  Grid
	Seed  uint64
	Tiles []Tile // indexed by Grid.Index

	// Atmosphere is surface pressure in kPa (Earth is about 101).
	Atmosphere float64
	// Ticks is the number of simulation steps run so far.
	Ticks uint64

	neighbors [][4]int32 // indices of each tile's four neighbours
	cosLat    []float64  // cosine of each tile's latitude
	// co2PerIce is how much atmosphere (kPa) melting one unit of ice
	// releases, scaled so melting all the starting ice adds maxReleasedCO2.
	co2PerIce float64

	// Scratch buffers reused by Step.
	nextTemp, nextLife, flow []float64
}

const (
	startAtmosphere = 0.6 // kPa, about Mars today
	maxReleasedCO2  = 300 // kPa released if every bit of starting ice melts
)

// NewPlanet generates a frozen, barren starting planet from seed. The same
// seed and size always give the same planet.
func NewPlanet(n int, seed uint64) *Planet {
	g := Grid{N: n}
	cells := g.NumCells()
	p := &Planet{
		Grid:       g,
		Seed:       seed,
		Tiles:      make([]Tile, cells),
		Atmosphere: startAtmosphere,
		neighbors:  make([][4]int32, cells),
		cosLat:     make([]float64, cells),
		nextTemp:   make([]float64, cells),
		nextLife:   make([]float64, cells),
		flow:       make([]float64, cells),
	}
	totalIce := 0.0
	for idx := range p.Tiles {
		c := g.CellAt(idx)
		for k, nb := range g.Neighbors(c) {
			p.neighbors[idx][k] = int32(g.Index(nb))
		}
		d := g.DirFromCell(c)
		p.cosLat[idx] = math.Sqrt(1 - d.Y*d.Y)

		h := fbm(seed, d.Scale(2.2), 5)
		t := Tile{Height: h, Ice: startingIce(seed, d, h)}
		p.Tiles[idx] = t
		totalIce += t.Ice
	}
	p.co2PerIce = maxReleasedCO2 / totalIce
	for idx := range p.Tiles {
		p.Tiles[idx].Temp = p.equilibrium(idx)
	}
	return p
}

// Tile returns the tile at c.
func (p *Planet) Tile(c Cell) Tile { return p.Tiles[p.Grid.Index(c)] }

// startingIce places polar caps with a ragged edge, snow on peaks, and a
// little permafrost everywhere else (most in low basins).
func startingIce(seed uint64, d Vec3, h float64) float64 {
	edge := 0.82 - 0.08*fbm(seed^0xC3, d.Scale(4), 3) - 0.15*math.Max(h, 0)
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
