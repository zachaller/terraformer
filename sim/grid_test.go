package sim

import (
	"math"
	"math/cmplx"
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

func TestConformalSeries(t *testing.T) {
	// confB inverts confA.
	for _, w := range []complex128{0.01, 0.1i, -0.2 + 0.1i, 0.3} {
		if got := series(confA[:], series(confB[:], w)); cmplx.Abs(got-w) > 1e-6 {
			t.Errorf("A(B(%v)) = %v", w, got)
		}
	}
	// The face centre, edge midpoints and corners land where they should,
	// and face edges lie on the planes of the cube's edges.
	for _, tc := range []struct {
		u, v float64
		want Vec3
	}{
		{0, 0, Vec3{0, 0, 1}},
		{1, 0, Vec3{1, 0, 1}},
		{0, -1, Vec3{0, -1, 1}},
		{1, 1, Vec3{1, 1, 1}},
		{-1, 1, Vec3{-1, 1, 1}},
	} {
		got := dirFromFaceUV(4, tc.u, tc.v).Normalize()
		if got.Sub(tc.want.Normalize()).Len() > 1e-9 {
			t.Errorf("dirFromFaceUV(4, %v, %v) = %v, want %v", tc.u, tc.v, got, tc.want.Normalize())
		}
	}
	for k := 0; k <= 20; k++ {
		d := dirFromFaceUV(4, 1, float64(k)/10-1)
		if math.Abs(d.X-d.Z) > 1e-9 {
			t.Errorf("edge point %v off the plane x = z", d)
		}
	}
	// faceUV inverts dirFromFaceUV.
	for i := range 19 {
		for j := range 19 {
			u, v := float64(i+1)/10-1, float64(j+1)/10-1
			for face := range Faces {
				f, gu, gv := faceUV(dirFromFaceUV(face, u, v))
				if f != face || math.Abs(gu-u) > 1e-6 || math.Abs(gv-v) > 1e-6 {
					t.Errorf("faceUV(dirFromFaceUV(%d, %v, %v)) = %d, %v, %v", face, u, v, f, gu, gv)
				}
			}
		}
	}
}

func TestConformalRightAngles(t *testing.T) {
	// Grid lines cross at right angles everywhere outside the corner warp,
	// including across seams.
	g := Grid{N: 16}
	for idx := range g.NumCells() {
		c := g.CellAt(idx)
		nb := g.Neighbors(c)
		p := g.DirFromCell(c)
		du := g.DirFromCell(nb[0]).Sub(g.DirFromCell(nb[1]))
		dv := g.DirFromCell(nb[2]).Sub(g.DirFromCell(nb[3]))
		if cos := math.Abs(du.Dot(dv)) / du.Len() / dv.Len(); cos > 0.05 && !nearCorner(g, c) {
			t.Errorf("cell %v at %v: grid lines meet at cos %.3f", c, p, cos)
		}
	}
}

// nearCorner reports whether c is within cornerWarp's reach of a cube
// corner, or on a neighbouring face just past it.
func nearCorner(g Grid, c Cell) bool {
	from := func(k int) float64 {
		t := (float64(k)+0.5)/float64(g.N)*2 - 1
		return 1 - math.Abs(t) - 2/float64(g.N)
	}
	return math.Hypot(max(from(c.I), 0), max(from(c.J), 0)) < 1
}

func TestCornerWarp(t *testing.T) {
	for i := range 21 {
		for j := range i + 1 {
			a, b := float64(i)/20, float64(j)/20
			wa, wb := cornerWarp(a, b)
			if ua, ub := cornerUnwarp(wa, wb); math.Abs(ua-a) > 1e-9 || math.Abs(ub-b) > 1e-9 {
				t.Errorf("cornerUnwarp(cornerWarp(%v, %v)) = %v, %v", a, b, ua, ub)
			}
			if wb > wa+1e-12 || wa > 1 || wb < 0 {
				t.Errorf("cornerWarp(%v, %v) = %v, %v leaves the triangle", a, b, wa, wb)
			}
		}
	}
	// Tile areas stay within a factor of about 2 of each other.
	g := Grid{N: 32}
	area := func(i, j int) float64 {
		p := func(i, j int) Vec3 {
			return dirFromFaceUV(4, float64(i)/16-1, float64(j)/16-1).Normalize()
		}
		a, b, c, d := p(i, j), p(i+1, j), p(i, j+1), p(i+1, j+1)
		return b.Sub(a).Cross(c.Sub(a)).Len() + b.Sub(d).Cross(c.Sub(d)).Len()
	}
	centre := area(g.N/2, g.N/2)
	for i := range g.N {
		for j := range g.N {
			if r := area(i, j) / centre; r < 0.55 || r > 1.2 {
				t.Errorf("tile %d,%d has %.2f times the area of a centre tile", i, j, r)
			}
		}
	}
}
