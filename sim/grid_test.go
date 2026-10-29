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

func TestCellFromDirAxes(t *testing.T) {
	g := Grid{N: 4}
	for _, tc := range []struct {
		d    Vec3
		face int
	}{
		{Vec3{1, 0.1, 0.2}, 0},
		{Vec3{-1, 0.1, 0.2}, 1},
		{Vec3{0.1, 1, 0.2}, 2},
		{Vec3{0.1, -1, 0.2}, 3},
		{Vec3{0.1, 0.2, 1}, 4},
		{Vec3{0.1, 0.2, -1}, 5},
	} {
		if got := g.CellFromDir(tc.d).Face; got != tc.face {
			t.Errorf("CellFromDir(%v).Face = %d, want %d", tc.d, got, tc.face)
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
	for _, n := range []int{1, 2, 5, 32} {
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
	// Neighbours, including across face seams, should be about one cell
	// apart: never more than ~2 cell widths (in radians) away.
	g := Grid{N: 32}
	maxAngle := 2 * (math.Pi / 2) / float64(g.N)
	for idx := range g.NumCells() {
		c := g.CellAt(idx)
		d := g.DirFromCell(c)
		for _, nb := range g.Neighbors(c) {
			if a := math.Acos(min(d.Dot(g.DirFromCell(nb)), 1)); a > maxAngle {
				t.Fatalf("%v and neighbour %v are %.3f rad apart, max %.3f", c, nb, a, maxAngle)
			}
		}
	}
}
