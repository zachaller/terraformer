package sim

import (
	"math"
	"testing"
)

func TestIndexRoundTrip(t *testing.T) {
	g := Grid{N: 8}
	for idx := range g.NumCells() {
		if got := g.Index(g.CellAt(idx)); got != idx {
			t.Fatalf("Index(CellAt(%d)) = %d", idx, got)
		}
	}
}

func TestCellDirRoundTrip(t *testing.T) {
	g := Grid{N: 32}
	for idx := range g.NumCells() {
		c := g.CellAt(idx)
		d := g.DirFromCell(c)
		if math.Abs(d.Len()-1) > 1e-9 {
			t.Fatalf("DirFromCell(%v) not unit length: %v", c, d.Len())
		}
		if got := g.CellFromDir(d); got != c {
			t.Fatalf("CellFromDir(DirFromCell(%v)) = %v", c, got)
		}
	}
}

func TestPolesAndPentagons(t *testing.T) {
	g := Grid{N: 6}
	if got := g.CellFromDir(Vec3{Y: 1}); got.Diamond != NorthPole {
		t.Errorf("CellFromDir(north) = %v", got)
	}
	if got := g.CellFromDir(Vec3{Y: -1}); got.Diamond != SouthPole {
		t.Errorf("CellFromDir(south) = %v", got)
	}
	// Exactly twelve cells, the icosahedron's corners, have five neighbours;
	// the rest have six.
	fives := 0
	for idx := range g.NumCells() {
		switch n := len(g.NeighborIndices(idx)); n {
		case 5:
			fives++
		case 6:
		default:
			t.Fatalf("%v has %d neighbours", g.CellAt(idx), n)
		}
	}
	if fives != 12 {
		t.Errorf("%d pentagons, want 12", fives)
	}
}

func TestLatticeEdgesAgree(t *testing.T) {
	// A lattice point on a diamond's edge is the same place whichever
	// diamond it's addressed from.
	g := Grid{N: 7}
	n := g.N
	for d := range Diamonds {
		for i := 0; i <= n; i++ {
			for j := 0; j <= n; j++ {
				c := g.LatticeCell(d, i, j)
				want := LatticeDir(d, float64(i)/float64(n), float64(j)/float64(n))
				if got := g.DirFromCell(c); got.Sub(want).Len() > 1e-9 {
					t.Fatalf("lattice (%d, %d, %d) is %v at %v, but that cell is at %v", d, i, j, c, want, got)
				}
			}
		}
	}
}

func TestLatticeWarpInverts(t *testing.T) {
	for _, w := range [][3]float64{{1, 0, 0}, {0.2, 0.3, 0.5}, {1.0 / 3, 1.0 / 3, 1.0 / 3}, {0, 0.9, 0.1}} {
		a, p, q := latticeWarp(w[0], w[1], w[2])
		ga, gp, gq := latticeUnwarp(a, p, q)
		if math.Abs(ga-w[0])+math.Abs(gp-w[1])+math.Abs(gq-w[2]) > 1e-9 {
			t.Errorf("latticeUnwarp(latticeWarp(%v)) = %v, %v, %v", w, ga, gp, gq)
		}
	}
}

func TestTileAreasEven(t *testing.T) {
	// Each tile's share of the sphere, estimated from sampling, stays
	// within about 25% of the average.
	g := Grid{N: 6}
	hits := make([]int, g.NumCells())
	const steps = 600
	total := 0.0
	w := make([]float64, g.NumCells())
	for a := range steps {
		z := 1 - 2*(float64(a)+0.5)/steps // even in z: equal-area bands
		r := math.Sqrt(1 - z*z)
		for b := range steps {
			phi := 2 * math.Pi * (float64(b) + 0.5*float64(a%2)) / steps
			idx := g.Index(g.CellFromDir(Vec3{r * math.Cos(phi), z, r * math.Sin(phi)}))
			hits[idx]++
			w[idx]++
			total++
		}
	}
	mean := total / float64(g.NumCells())
	for idx, h := range hits {
		want := 1.0
		if len(g.NeighborIndices(idx)) == 5 {
			want = 5.0 / 6 // pentagons are naturally smaller
		}
		if r := float64(h) / mean / want; r < 0.75 || r > 1.25 {
			t.Errorf("%v covers %.2f of an average tile", g.CellAt(idx), r)
		}
	}
}

func TestEveryCellReachable(t *testing.T) {
	// Sample directions densely and check every cell gets hit, i.e. the
	// projection has no gaps.
	g := Grid{N: 8}
	seen := make([]bool, g.NumCells())
	const steps = 400
	for a := range steps {
		for b := range steps {
			theta := math.Pi * (float64(a) + 0.5) / steps
			phi := 2 * math.Pi * float64(b) / steps
			d := Vec3{math.Sin(theta) * math.Cos(phi), math.Cos(theta), math.Sin(theta) * math.Sin(phi)}
			seen[g.Index(g.CellFromDir(d))] = true
		}
	}
	for idx, ok := range seen {
		if !ok {
			t.Errorf("cell %v never hit", g.CellAt(idx))
		}
	}
}

func TestNeighborsDistinctAndSymmetric(t *testing.T) {
	for _, n := range []int{1, 2, 5, 25} {
		g := Grid{N: n}
		for idx := range g.NumCells() {
			c := g.CellAt(idx)
			nbs := g.Neighbors(c)
			seen := map[Cell]bool{}
			for _, nb := range nbs {
				if nb == c {
					t.Fatalf("N=%d: %v is its own neighbour", n, c)
				}
				if seen[nb] {
					t.Fatalf("N=%d: %v has duplicate neighbour %v", n, c, nb)
				}
				seen[nb] = true
				back := false
				for _, nb2 := range g.Neighbors(nb) {
					back = back || nb2 == c
				}
				if !back {
					t.Fatalf("N=%d: %v neighbours %v but not the reverse", n, c, nb)
				}
			}
		}
	}
}

func TestNeighborsAreClose(t *testing.T) {
	// Neighbours are about one tile apart: never more than 1.5 tile
	// spacings.
	g := Grid{N: 25}
	maxAngle := 1.5 * g.TileAngle()
	for idx := range g.NumCells() {
		c := g.CellAt(idx)
		d := g.DirFromCell(c)
		for _, nb := range g.Neighbors(c) {
			if a := math.Acos(min(d.Dot(g.DirFromCell(nb)), 1)); a > maxAngle {
				t.Fatalf("%v and neighbour %v are %.3f rad apart, max %.3f", c, nb, a, maxAngle)
			}
		}
	}
	if want := math.Atan(2) / 25; math.Abs(g.TileAngle()-want) > 0.1*want {
		t.Errorf("TileAngle() = %v, want about %v", g.TileAngle(), want)
	}
}
