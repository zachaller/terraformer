// Package sim holds the game's simulation: the planet grid, tiles and rules.
// It has no engine dependencies so it can be tested with `go test` and run
// headless.
package sim

import (
	"math"
	"math/cmplx"
)

// The planet surface is a cube-sphere: the six faces of a cube, each an N×N
// grid, projected onto a unit sphere. Each face has two tangent axes (u, v)
// in [-1, 1]:
//
//	face 0 (+X): u = -z, v =  y
//	face 1 (-X): u =  z, v =  y
//	face 2 (+Y): u =  x, v = -z
//	face 3 (-Y): u =  x, v =  z
//	face 4 (+Z): u =  x, v =  y
//	face 5 (-Z): u = -x, v =  y
//
// The (u, v) grid isn't evenly spaced on the flat cube face: it uses the
// conformal cubed-sphere mapping of Rančić, Purser and Mesinger (1996), which
// keeps tiles square (no shearing) everywhere and makes grid lines cross the
// seams between faces at right angles, so rows run straight on from one face
// to the next. On its own, though, it shrinks tiles towards the eight cube
// corners to almost nothing (the corner tile would have about a ninth of the
// area of a face-centre one), so the grid is also stretched radially around
// each corner (cornerWarp). That keeps rows meeting seams at right angles,
// and brings the corner tile up to about 70% of a face-centre tile, at the
// cost of some skew (up to about 25°) in the tiles nearest the corners.
//
// game/planet.kage repeats this mapping (faceUV and dirFromFaceUV); keep
// them in sync.

// Faces is the number of cube faces.
const Faces = 6

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

// Cell identifies one tile: a cube face and a column (I) and row (J) on it.
type Cell struct {
	Face, I, J int
}

// Grid is a cube-sphere with N×N cells per face.
type Grid struct {
	N int
}

// NumCells is the total number of cells on the planet.
func (g Grid) NumCells() int { return Faces * g.N * g.N }

// Index returns a dense index for c in [0, NumCells).
func (g Grid) Index(c Cell) int { return (c.Face*g.N+c.J)*g.N + c.I }

// CellAt is the inverse of Index.
func (g Grid) CellAt(idx int) Cell {
	i := idx % g.N
	j := (idx / g.N) % g.N
	return Cell{Face: idx / (g.N * g.N), I: i, J: j}
}

// CellFromDir returns the cell containing direction d (need not be unit length).
func (g Grid) CellFromDir(d Vec3) Cell {
	face, u, v := faceUV(d)
	return Cell{Face: face, I: g.coord(u), J: g.coord(v)}
}

// DirFromCell returns the unit direction through the centre of c.
func (g Grid) DirFromCell(c Cell) Vec3 {
	u := (float64(c.I)+0.5)/float64(g.N)*2 - 1
	v := (float64(c.J)+0.5)/float64(g.N)*2 - 1
	return dirFromFaceUV(c.Face, u, v).Normalize()
}

// Neighbors returns the four cells sharing an edge with c, in the order
// +I, -I, +J, -J.
func (g Grid) Neighbors(c Cell) [4]Cell {
	steps := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	var out [4]Cell
	for k, s := range steps {
		out[k] = g.Wrap(c.Face, c.I+s[0], c.J+s[1])
	}
	return out
}

// Wrap returns the cell at column i, row j of face, where i and j may run up
// to a cell past the face's edges. The grid is symmetric about the plane of
// every cube edge, so the cell just past an edge is the mirror image, across
// that edge's plane, of the cell just inside it.
func (g Grid) Wrap(face, i, j int) Cell {
	if i >= 0 && i < g.N && j >= 0 && j < g.N {
		return Cell{Face: face, I: i, J: j}
	}
	eu, ev, en := faceAxes(face)
	d := g.DirFromCell(Cell{Face: face, I: mirror(i, g.N), J: mirror(j, g.N)})
	if i < 0 || i >= g.N {
		d = reflect(d, edgeNormal(eu, en, i < 0))
	}
	if j < 0 || j >= g.N {
		d = reflect(d, edgeNormal(ev, en, j < 0))
	}
	return g.CellFromDir(d)
}

// mirror maps a column or row one past a face edge onto the one just inside.
func mirror(k, n int) int {
	if k < 0 {
		return -1 - k
	}
	if k >= n {
		return 2*n - 1 - k
	}
	return k
}

// edgeNormal is the normal of the plane through the face edge at the low
// (axis = -1) or high (+1) end of axis e on the face with normal en.
func edgeNormal(e, en Vec3, low bool) Vec3 {
	if low {
		return e.Add(en)
	}
	return e.Sub(en)
}

func reflect(d, n Vec3) Vec3 { return d.Sub(n.Scale(2 * d.Dot(n) / n.Dot(n))) }

func (g Grid) coord(t float64) int {
	k := int(math.Floor((t + 1) / 2 * float64(g.N)))
	return min(max(k, 0), g.N-1)
}

// Rančić et al. (1996) map one eighth of a face, the triangle between the
// face centre, an edge midpoint and a corner, through a Taylor series about
// the corner: confA (their Table B1) maps the square's side, in Z = z⁴ where
// z is the position relative to the corner, to the sphere's side, in W = w³
// where w is a stereographic coordinate turned so the corner is at 0. The
// powers open the corner's 90° (square) and 120° (sphere) angles out flat, so
// the series converges quickly. confB is its inverse (series reversion of
// confA; TestConformalSeries checks it). Twelve terms are accurate to about
// 1e-8.
var (
	confA = [...]float64{1.47713062600964, -0.38183510510174, -0.05573058001191, -0.00895883606818, -0.00791315785221, -0.00486625437708, -0.00329251751279, -0.00235481488325, -0.00175870527475, -0.00135681133278, -0.00107459847699, -0.00086944475948}
	confB = [...]float64{0.67698819751738992, 0.11847293456554467, 0.053171781346679235, 0.02965810434051968, 0.019124473040277028, 0.01342565621117456, 0.0099887332318051442, 0.0077486899640560845, 0.0062034697988811935, 0.0050901087488291518, 0.0042598118432780737, 0.0036230895607660438}

	// The Möbius map between the stereographic coordinate (centre of the
	// face at 0) and one with the corner (1, 1) at 0.
	confRa = math.Sqrt(3) - 1
	confCb = complex(-1, 1)
	confCc = complex(confRa/2, 0) * confCb
)

// series evaluates c[0]·x + c[1]·x² + ….
func series(c []float64, x complex128) complex128 {
	s := complex(0, 0)
	for k := len(c) - 1; k >= 0; k-- {
		s = (s + complex(c[k], 0)) * x
	}
	return s
}

// octant folds (u, v) into the triangle 0 ≤ b ≤ a, returning what's needed
// to unfold the result.
func octant(u, v float64) (a, b float64, swap bool) {
	a, b = math.Abs(u), math.Abs(v)
	if b > a {
		return b, a, true
	}
	return a, b, false
}

func unfold(a, b, u, v float64, swap bool) (float64, float64) {
	if swap {
		a, b = b, a
	}
	return math.Copysign(a, u), math.Copysign(b, v)
}

func faceUV(d Vec3) (face int, u, v float64) {
	face, x, y := cubeFaceUV(d)
	a, b, swap := octant(x, y)
	// Stereographic coordinate of the point, then the corner-centred one.
	w := complex(a, b) / complex(1+math.Sqrt(1+a*a+b*b), 0)
	w = (w*confCb + complex(confRa, 0)) / (1 - w*confCc)
	z := 2 * cmplx.Pow(series(confB[:], -w*w*w), 0.25)
	// z is (1-a) + i(1-b) up to a quarter turn, which the folding undoes.
	p, q := math.Abs(real(z)), math.Abs(imag(z))
	a, b = cornerUnwarp(1-min(p, q), 1-max(p, q))
	u, v = unfold(a, b, x, y, swap)
	return face, u, v
}

// dirFromFaceUV is the inverse of faceUV: a direction (not unit length)
// through (u, v) on face. u and v must be in [-1, 1].
func dirFromFaceUV(face int, u, v float64) Vec3 {
	a, b, swap := octant(u, v)
	a, b = cornerWarp(a, b)
	z := complex(1-a, 1-b) / 2
	w := series(confA[:], z*z*z*z)
	w = cmplx.Exp(complex(0, math.Pi/6)) * cmplx.Pow(w*1i, 1.0/3)
	w = (w - complex(confRa, 0)) / (confCb + confCc*w)
	x, y := unfold(real(w), imag(w), u, v, swap)
	eu, ev, en := faceAxes(face)
	// Inverse stereographic projection, scaled by (1 + |w|²)/2.
	return eu.Scale(x).Add(ev.Scale(y)).Add(en.Scale((1 - x*x - y*y) / 2))
}

// cornerGamma sets how strongly cornerWarp enlarges tiles near cube corners:
// 1 leaves the conformal grid alone; 4/3 cancels its shrinking exactly at
// the corner; more makes corner tiles larger still.
const cornerGamma = 1.5

// cornerWarp maps a grid position (a, b), folded so that 0 ≤ b ≤ a ≤ 1,
// to the conformal position it shows. Within distance 1 of the corner (1, 1)
// it moves points away from the corner along the line to it, by a radius
// map whose inverse is cornerSigma; further out it does nothing. Moving
// points only along lines through the corner keeps the face's edges in
// place and the grid symmetric, so rows still cross seams at right angles.
func cornerWarp(a, b float64) (float64, float64) {
	p, q := 1-a, 1-b
	t := math.Hypot(p, q)
	if t >= 1 || t == 0 {
		return a, b
	}
	// Invert cornerSigma by Newton's method.
	x := math.Pow(t, 1/cornerGamma)
	for range 6 {
		x -= (cornerSigma(x) - t) / cornerSigmaSlope(x)
	}
	k := x / t
	return 1 - p*k, 1 - q*k
}

// cornerUnwarp is the inverse of cornerWarp.
func cornerUnwarp(a, b float64) (float64, float64) {
	p, q := 1-a, 1-b
	t := math.Hypot(p, q)
	if t >= 1 || t == 0 {
		return a, b
	}
	k := cornerSigma(t) / t
	return 1 - p*k, 1 - q*k
}

// cornerSigma maps distance from the corner on the conformal grid to
// distance on the tile grid: about x^γ near the corner, and matching x (in
// value and slope) at 1.
func cornerSigma(x float64) float64 {
	return math.Pow(x, cornerGamma) * (1 + (cornerGamma-1)*(1-x))
}

func cornerSigmaSlope(x float64) float64 {
	const g = cornerGamma
	return math.Pow(x, g-1) * (g*g - (g*g-1)*x)
}

// faceAxes returns the directions of increasing u and v on face, and its
// normal.
func faceAxes(face int) (eu, ev, en Vec3) {
	en = cubeDir(face, 0, 0)
	return cubeDir(face, 1, 0).Sub(en), cubeDir(face, 0, 1).Sub(en), en
}

// cubeFaceUV projects d onto the cube: its face and position (u, v) on it,
// each in [-1, 1].
func cubeFaceUV(d Vec3) (face int, u, v float64) {
	ax, ay, az := math.Abs(d.X), math.Abs(d.Y), math.Abs(d.Z)
	switch {
	case ax >= ay && ax >= az:
		if d.X > 0 {
			return 0, -d.Z / ax, d.Y / ax
		}
		return 1, d.Z / ax, d.Y / ax
	case ay >= az:
		if d.Y > 0 {
			return 2, d.X / ay, -d.Z / ay
		}
		return 3, d.X / ay, d.Z / ay
	default:
		if d.Z > 0 {
			return 4, d.X / az, d.Y / az
		}
		return 5, -d.X / az, d.Y / az
	}
}

// cubeDir is the inverse of cubeFaceUV: the point on the cube for (face, u, v).
func cubeDir(face int, u, v float64) Vec3 {
	switch face {
	case 0:
		return Vec3{1, v, -u}
	case 1:
		return Vec3{-1, v, u}
	case 2:
		return Vec3{u, 1, -v}
	case 3:
		return Vec3{u, -1, v}
	case 4:
		return Vec3{u, v, 1}
	default:
		return Vec3{-u, v, -1}
	}
}
