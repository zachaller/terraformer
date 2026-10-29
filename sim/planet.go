package sim

import "math"

// TileKind is the surface type of a tile.
type TileKind uint8

const (
	Basin TileKind = iota
	Plains
	Highlands
	Peaks
	IceCap
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
	case IceCap:
		return "Ice cap"
	}
	return "Unknown"
}

// Tile is the state of one cell.
type Tile struct {
	Kind TileKind
	// Height is terrain elevation, roughly in [-1, 1].
	Height float64
}

// Planet is a whole world: its grid and the tile in every cell.
type Planet struct {
	Grid  Grid
	Seed  uint64
	Tiles []Tile // indexed by Grid.Index
}

// NewPlanet generates a barren starting planet from seed. The same seed and
// size always give the same planet.
func NewPlanet(n int, seed uint64) *Planet {
	g := Grid{N: n}
	p := &Planet{Grid: g, Seed: seed, Tiles: make([]Tile, g.NumCells())}
	for idx := range p.Tiles {
		d := g.DirFromCell(g.CellAt(idx))
		h := fbm(seed, d.Scale(2.2), 5)
		p.Tiles[idx] = Tile{Kind: classify(seed, d, h), Height: h}
	}
	return p
}

// Tile returns the tile at c.
func (p *Planet) Tile(c Cell) Tile { return p.Tiles[p.Grid.Index(c)] }

func classify(seed uint64, d Vec3, h float64) TileKind {
	// Polar caps with a ragged edge; higher ground freezes further from the pole.
	edge := 0.82 - 0.08*fbm(seed^0xC3, d.Scale(4), 3) - 0.15*math.Max(h, 0)
	switch {
	case math.Abs(d.Y) > edge:
		return IceCap
	case h < -0.15:
		return Basin
	case h < 0.1:
		return Plains
	case h < 0.3:
		return Highlands
	default:
		return Peaks
	}
}
