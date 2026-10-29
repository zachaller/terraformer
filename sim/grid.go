// Package sim holds the game's simulation: the planet grid, tiles and rules.
// It has no engine dependencies so it can be tested with `go test` and run
// headless.
package sim

import (
	"math"
	"sync"
)

// The planet surface is a geodesic hex grid (a Goldberg polyhedron): mostly
// hexagonal tiles, plus exactly twelve pentagons, which no hex grid on a
// sphere can avoid. It's built on an icosahedron whose 20 triangular faces
// are each divided into a triangular lattice with N steps along every edge;
// each lattice point is the centre of one tile, and the pentagons are the
// icosahedron's 12 corners.
//
// The faces are grouped into ten diamonds (pairs of faces sharing an edge),
// each holding an N×N patch of lattice points addressed by (I, J), plus the
// two poles, which belong to no diamond:
//
//	diamonds 0–4 (north): corners O = north pole, P = upper ring k,
//	                      Q = upper ring k+1, R = lower ring k
//	diamonds 5–9 (south): corners O = upper ring k+1, P = lower ring k,
//	                      Q = lower ring k+1, R = south pole
//
// where the upper ring's five corners sit at 26.6°N, longitudes 72k, and the
// lower ring's at 26.6°S, longitudes 72k+36. The lattice point (i, j), with i
// and j in [0, N], lies on face (O, P, Q) at O + i/N·(P−O) + j/N·(Q−O) when
// i+j ≤ N, and on face (R, P, Q) otherwise. Points on a diamond's edges are
// shared with its neighbours; each diamond owns those with 0 ≤ i < N and
// 1 ≤ j ≤ N (see LatticeCell).
//
// Within each face, positions are spread with latticeWarp so tiles near the
// icosahedron's corners aren't much smaller than those at face centres, then
// projected onto the sphere from the centre.
//
// game/planet.kage repeats this mapping; keep them in sync.

// Diamonds is the number of diamonds the cells are grouped into.
const Diamonds = 10

// Pole "diamonds": the north and south pole cells.
const (
	NorthPole = Diamonds
	SouthPole = Diamonds + 1
)

// MaxNeighbors is the most neighbours a cell has (pentagons have five).
const MaxNeighbors = 6

// Vec3 is a 3D vector.
type Vec3 struct{ X, Y, Z float64 }

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Scale(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Dot(b Vec3) float64   { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec3) Len() float64         { return math.Sqrt(a.Dot(a)) }
func (a Vec3) Normalize() Vec3      { return a.Scale(1 / a.Len()) }
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

// Cell identifies one tile: a diamond and a lattice point (I, J) in it, or a
// pole (Diamond NorthPole or SouthPole, with I and J zero).
type Cell struct {
	Diamond, I, J int
}

// Grid is a geodesic hex grid with N lattice steps along each icosahedron
// edge.
type Grid struct {
	N int
}

// NumCells is the total number of cells on the planet.
func (g Grid) NumCells() int { return Diamonds*g.N*g.N + 2 }

// Index returns a dense index for c in [0, NumCells).
func (g Grid) Index(c Cell) int {
	switch c.Diamond {
	case NorthPole:
		return Diamonds * g.N * g.N
	case SouthPole:
		return Diamonds*g.N*g.N + 1
	}
	return (c.Diamond*g.N+c.J-1)*g.N + c.I
}

// CellAt is the inverse of Index.
func (g Grid) CellAt(idx int) Cell {
	n := g.N
	switch idx {
	case Diamonds * n * n:
		return Cell{Diamond: NorthPole}
	case Diamonds*n*n + 1:
		return Cell{Diamond: SouthPole}
	}
	return Cell{Diamond: idx / (n * n), I: idx % n, J: (idx/n)%n + 1}
}

// LatticeCell returns the cell at lattice point (i, j) of diamond d, where i
// and j are in [0, N]: points on a diamond's edges belong to it or to a
// neighbouring diamond, or are poles.
func (g Grid) LatticeCell(d, i, j int) Cell {
	n := g.N
	for {
		if d < 5 { // north diamond k = d
			switch {
			case i == 0 && j == 0:
				return Cell{Diamond: NorthPole}
			case j == 0: // edge to the north pole, shared with diamond k−1
				d, i, j = (d+4)%5, 0, i
			case i == n: // edge to lower ring k, shared with south diamond k−1
				d, i = 5+(d+4)%5, 0
			default:
				return Cell{Diamond: d, I: i, J: j}
			}
		} else { // south diamond k = d−5
			k := d - 5
			switch {
			case i == n && j == n:
				return Cell{Diamond: SouthPole}
			case j == 0: // edge to upper ring k+1, shared with north diamond k
				d, j = k, n
			case i == n: // edge to the south pole, shared with south diamond k−1
				d, i, j = 5+(k+4)%5, j, n
			default:
				return Cell{Diamond: d, I: i, J: j}
			}
		}
	}
}

// lattice returns a diamond and lattice point that c is at.
func (g Grid) lattice(c Cell) (d, i, j int) {
	switch c.Diamond {
	case NorthPole:
		return 0, 0, 0
	case SouthPole:
		return 5, g.N, g.N
	}
	return c.Diamond, c.I, c.J
}

// CellFromDir returns the cell containing direction d (need not be unit
// length): the lattice point nearest to it, measured in its face's
// (warped) lattice.
func (g Grid) CellFromDir(d Vec3) Cell {
	dm, i, j := g.latticeCoords(d)
	i0, j0 := math.Floor(i), math.Floor(j)
	fi, fj := i-i0, j-j0
	// The lattice triangle containing (i, j), and the corner it's closest
	// to: the one with the largest barycentric weight.
	var di, dj int
	if fi+fj < 1 {
		switch {
		case fi > fj && fi > 1-fi-fj:
			di = 1
		case fj > 1-fi-fj:
			dj = 1
		}
	} else {
		switch {
		case 1-fj > 1-fi && 1-fj > fi+fj-1:
			di = 1
		case 1-fi > fi+fj-1:
			dj = 1
		default:
			di, dj = 1, 1
		}
	}
	return g.LatticeCell(dm, clampInt(int(i0)+di, 0, g.N), clampInt(int(j0)+dj, 0, g.N))
}

func clampInt(v, lo, hi int) int { return min(max(v, lo), hi) }

// latticeCoords returns the diamond direction d falls in and its continuous
// lattice coordinates there.
func (g Grid) latticeCoords(d Vec3) (dm int, i, j float64) {
	best, upper := math.Inf(-1), false
	for k := range Diamonds {
		c := &diamondCorners[k]
		if v := d.Dot(c.O.Add(c.P).Add(c.Q)); v > best {
			best, dm, upper = v, k, false
		}
		if v := d.Dot(c.R.Add(c.P).Add(c.Q)); v > best {
			best, dm, upper = v, k, true
		}
	}
	c := &diamondCorners[dm]
	a := c.O
	if upper {
		a = c.R
	}
	// Barycentric coordinates of d's projection onto the face's plane.
	ba := d.Dot(c.P.Cross(c.Q))
	bp := a.Dot(d.Cross(c.Q))
	bq := a.Dot(c.P.Cross(d))
	s := ba + bp + bq
	_, wp, wq := latticeUnwarp(ba/s, bp/s, bq/s)
	n := float64(g.N)
	if upper {
		return dm, n * (1 - wq), n * (1 - wp)
	}
	return dm, n * wp, n * wq
}

// DirFromCell returns the unit direction through the centre of c.
func (g Grid) DirFromCell(c Cell) Vec3 {
	d, i, j := g.lattice(c)
	return LatticeDir(d, float64(i)/float64(g.N), float64(j)/float64(g.N))
}

// LatticeDir is the unit direction through point (i, j) of diamond d, with
// i and j as fractions of N, in [0, 1].
func LatticeDir(d int, i, j float64) Vec3 {
	c := &diamondCorners[d]
	a, wa, wp, wq := c.O, 1-i-j, i, j
	if i+j > 1 {
		a, wa, wp, wq = c.R, i+j-1, 1-j, 1-i
	}
	ba, bp, bq := latticeWarp(wa, wp, wq)
	return a.Scale(ba).Add(c.P.Scale(bp)).Add(c.Q.Scale(bq)).Normalize()
}

// latticeWarpC sets latticeWarp's strength; 0.25 makes tile areas the most
// even (the smallest is about 0.83 of the largest; without it, 0.44).
const latticeWarpC = 0.25

// latticeWarp maps evenly spaced barycentric coordinates on a face to the
// planar barycentric coordinates of the point shown there: each weight w
// becomes w − c·w², renormalised. That pushes points towards the face's
// corners, where projection onto the sphere shrinks tiles most. It depends
// only on the weights, symmetrically, so points on a shared edge land in the
// same place from either face.
func latticeWarp(wa, wp, wq float64) (float64, float64, float64) {
	ga, gp, gq := warpG(wa), warpG(wp), warpG(wq)
	s := ga + gp + gq
	return ga / s, gp / s, gq / s
}

func warpG(w float64) float64 { return w - latticeWarpC*w*w }

// latticeUnwarp inverts latticeWarp: it finds weights w summing to 1 with
// warpG(w) = λ·b for some λ, by Newton's method on λ.
func latticeUnwarp(ba, bp, bq float64) (float64, float64, float64) {
	// warpG(w) = y has w = (1 − √(1 − 4cy)) / 2c.
	const c = latticeWarpC
	inv := func(y float64) float64 { return (1 - math.Sqrt(math.Max(1-4*c*y, 0))) / (2 * c) }
	slope := func(y float64) float64 { return 1 / math.Sqrt(math.Max(1-4*c*y, 1e-12)) }
	l := 1 - c/3
	for range 6 {
		h := inv(l*ba) + inv(l*bp) + inv(l*bq) - 1
		dh := ba*slope(l*ba) + bp*slope(l*bp) + bq*slope(l*bq)
		l -= h / dh
	}
	return inv(l * ba), inv(l * bp), inv(l * bq)
}

// Diamond corners, as unit vectors.
type corners struct{ O, P, Q, R Vec3 }

var diamondCorners = func() [Diamonds]corners {
	lat := math.Atan(0.5)
	ring := func(lon, lat float64) Vec3 {
		return Vec3{math.Cos(lat) * math.Cos(lon), math.Sin(lat), math.Cos(lat) * math.Sin(lon)}
	}
	up := func(k int) Vec3 { return ring(float64(k%5)*2*math.Pi/5, lat) }
	low := func(k int) Vec3 { return ring((float64(k%5)+0.5)*2*math.Pi/5, -lat) }
	north, south := Vec3{Y: 1}, Vec3{Y: -1}
	var out [Diamonds]corners
	for k := range 5 {
		out[k] = corners{O: north, P: up(k), Q: up(k + 1), R: low(k)}
		out[5+k] = corners{O: up(k + 1), P: low(k), Q: low(k + 1), R: south}
	}
	return out
}()

// DiamondCorners returns the corners O, P, Q and R of diamond d.
func DiamondCorners(d int) (o, p, q, r Vec3) {
	c := diamondCorners[d]
	return c.O, c.P, c.Q, c.R
}

// topology is the neighbour structure for one grid size, built once.
type topology struct {
	neighbors [][]int32
	tileAngle float64
}

var (
	topoMu    sync.Mutex
	topoCache = map[int]*topology{}
)

func (g Grid) topo() *topology {
	topoMu.Lock()
	defer topoMu.Unlock()
	if t, ok := topoCache[g.N]; ok {
		return t
	}
	n := g.N
	t := &topology{neighbors: make([][]int32, g.NumCells())}
	link := func(a, b int) {
		if a == b {
			return
		}
		for _, x := range t.neighbors[a] {
			if int(x) == b {
				return
			}
		}
		t.neighbors[a] = append(t.neighbors[a], int32(b))
		t.neighbors[b] = append(t.neighbors[b], int32(a))
	}
	// Every edge of the lattice lies within some diamond (including its
	// edges), so linking adjacent lattice points diamond by diamond finds
	// them all.
	for d := range Diamonds {
		for j := 0; j <= n; j++ {
			for i := 0; i <= n; i++ {
				a := g.Index(g.LatticeCell(d, i, j))
				for _, s := range [3][2]int{{1, 0}, {0, 1}, {-1, 1}} {
					ii, jj := i+s[0], j+s[1]
					if ii >= 0 && ii <= n && jj <= n {
						link(a, g.Index(g.LatticeCell(d, ii, jj)))
					}
				}
			}
		}
	}
	sum, count := 0.0, 0
	for a, nbs := range t.neighbors {
		da := g.DirFromCell(g.CellAt(a))
		for _, b := range nbs {
			sum += math.Acos(math.Min(da.Dot(g.DirFromCell(g.CellAt(int(b)))), 1))
			count++
		}
	}
	t.tileAngle = sum / float64(count)
	topoCache[g.N] = t
	return t
}

// Neighbors returns the cells sharing an edge with c: six, or five for the
// twelve pentagons.
func (g Grid) Neighbors(c Cell) []Cell {
	nbs := g.topo().neighbors[g.Index(c)]
	out := make([]Cell, len(nbs))
	for k, nb := range nbs {
		out[k] = g.CellAt(int(nb))
	}
	return out
}

// NeighborIndices is Neighbors by index. The slice is shared: don't modify
// it.
func (g Grid) NeighborIndices(idx int) []int32 { return g.topo().neighbors[idx] }

// TileAngle is the average angle, in radians, between neighbouring tiles'
// centres: the planet's tile spacing.
func (g Grid) TileAngle() float64 { return g.topo().tileAngle }
