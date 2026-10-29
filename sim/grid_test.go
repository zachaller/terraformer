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
