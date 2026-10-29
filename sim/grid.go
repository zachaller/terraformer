// Package sim holds the game's simulation: the planet grid, tiles and rules.
// It has no engine dependencies so it can be tested with `go test` and run
// headless.
package sim

import "math"

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
// game/planet.kage repeats this mapping; keep the two in sync.

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

func (g Grid) coord(t float64) int {
	k := int(math.Floor((t + 1) / 2 * float64(g.N)))
	return min(max(k, 0), g.N-1)
}

func faceUV(d Vec3) (face int, u, v float64) {
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

func dirFromFaceUV(face int, u, v float64) Vec3 {
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
