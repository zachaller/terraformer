package game

import (
	"math"

	"github.com/zachaller/terraformer/sim"
)

const (
	minZoom = 0.5
	maxZoom = 10
	// spinDecay is how much rotation momentum survives each tick after release.
	spinDecay = 0.92
)

// camera holds the planet's orientation and on-screen size.
//
// rot rotates planet space into view space (x right, y up, z towards the
// viewer). Its rows are the view axes expressed in planet space.
type camera struct {
	rot  [3]sim.Vec3
	zoom float64

	// Set every frame from the screen size.
	cx, cy, radius float64

	// Rotation momentum in screen pixels per tick.
	spinX, spinY float64
}

func newCamera() camera {
	c := camera{
		rot:  [3]sim.Vec3{{X: 1}, {Y: 1}, {Z: 1}},
		zoom: 1,
	}
	// Tilt slightly so a pole is in view.
	c.rotate(0, 0.35)
	return c
}

func (c *camera) layout(w, h int) { c.layoutIn(0, 0, float64(w), float64(h)) }

// layoutIn centres the planet in the given screen rectangle.
func (c *camera) layoutIn(x, y, w, h float64) {
	c.cx, c.cy = x+w/2, y+h/2
	c.radius = 0.4 * math.Min(w, h) * c.zoom
}

// drag rotates the planet so the surface under the pointer follows a drag of
// (dx, dy) screen pixels.
func (c *camera) drag(dx, dy float64) {
	c.spinX, c.spinY = dx, dy
	c.rotateByPixels(dx, dy)
}

// coast applies leftover momentum after the pointer is released.
func (c *camera) coast() {
	c.spinX *= spinDecay
	c.spinY *= spinDecay
	if math.Abs(c.spinX)+math.Abs(c.spinY) < 0.01 {
		c.spinX, c.spinY = 0, 0
		return
	}
	c.rotateByPixels(c.spinX, c.spinY)
}

func (c *camera) stop() { c.spinX, c.spinY = 0, 0 }

// turnTowards rotates the planet a step (a fraction of the remaining angle)
// towards showing planet direction d at the centre of the view. It reports
// whether d is already there.
func (c *camera) turnTowards(d sim.Vec3, fraction float64) bool {
	vx, vy, vz := c.rot[0].Dot(d), c.rot[1].Dot(d), c.rot[2].Dot(d)
	yaw, pitch := -math.Atan2(vx, vz), math.Atan2(vy, vz)
	if math.Abs(yaw)+math.Abs(pitch) < 0.002 {
		return true
	}
	c.stop()
	c.rotate(yaw*fraction, pitch*fraction)
	return false
}

func (c *camera) zoomBy(f float64) {
	c.zoom = math.Min(math.Max(c.zoom*f, minZoom), maxZoom)
}

func (c *camera) rotateByPixels(dx, dy float64) {
	if c.radius == 0 {
		return
	}
	c.rotate(dx/c.radius, dy/c.radius)
}

// rotate turns the planet by yaw radians about the view's y axis and pitch
// radians about its x axis (positive pitch moves the front surface down).
func (c *camera) rotate(yaw, pitch float64) {
	sy, cy := math.Sincos(yaw)
	sp, cp := math.Sincos(pitch)
	// New view axes in terms of old ones: R' = Rx(pitch) · Ry(yaw) · R.
	ry := [3][3]float64{{cy, 0, sy}, {0, 1, 0}, {-sy, 0, cy}}
	rx := [3][3]float64{{1, 0, 0}, {0, cp, -sp}, {0, sp, cp}}
	m := mul3(rx, ry)
	var out [3]sim.Vec3
	for i := range 3 {
		out[i] = c.rot[0].Scale(m[i][0]).Add(c.rot[1].Scale(m[i][1])).Add(c.rot[2].Scale(m[i][2]))
	}
	c.rot = orthonormalize(out)
}

// pick returns the cell under screen point (x, y), if the point is on the planet.
func (c *camera) pick(g sim.Grid, x, y float64) (sim.Cell, bool) {
	d, ok := c.pickDir(x, y)
	if !ok {
		return sim.Cell{}, false
	}
	return g.CellFromDir(d), true
}

// pickDir returns the planet direction under screen point (x, y), if the
// point is on the planet.
func (c *camera) pickDir(x, y float64) (sim.Vec3, bool) {
	qx := (x - c.cx) / c.radius
	qy := -(y - c.cy) / c.radius
	r2 := qx*qx + qy*qy
	if r2 > 1 {
		return sim.Vec3{}, false
	}
	qz := math.Sqrt(1 - r2)
	return c.rot[0].Scale(qx).Add(c.rot[1].Scale(qy)).Add(c.rot[2].Scale(qz)), true
}

// project is the inverse of pick: where planet direction d appears on screen,
// and whether it faces the viewer.
func (c *camera) project(d sim.Vec3) (x, y float64, visible bool) {
	vx, vy, vz := c.rot[0].Dot(d), c.rot[1].Dot(d), c.rot[2].Dot(d)
	return c.cx + vx*c.radius, c.cy - vy*c.radius, vz > 0
}

func mul3(a, b [3][3]float64) [3][3]float64 {
	var m [3][3]float64
	for i := range 3 {
		for j := range 3 {
			for k := range 3 {
				m[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return m
}

// orthonormalize removes floating-point drift that builds up over many rotations.
func orthonormalize(r [3]sim.Vec3) [3]sim.Vec3 {
	x := r[0].Normalize()
	y := r[1].Sub(x.Scale(x.Dot(r[1]))).Normalize()
	return [3]sim.Vec3{x, y, x.Cross(y)}
}
