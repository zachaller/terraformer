package game

import (
	"math"
	"testing"

	"github.com/zachaller/terraformer/sim"
)

// project is the inverse of pick: where planet direction d appears on screen,
// and whether it faces the viewer.
func (c *camera) project(d sim.Vec3) (x, y float64, visible bool) {
	vx, vy, vz := c.rot[0].Dot(d), c.rot[1].Dot(d), c.rot[2].Dot(d)
	return c.cx + vx*c.radius, c.cy - vy*c.radius, vz > 0
}

func TestPickInvertsProject(t *testing.T) {
	g := sim.Grid{N: 16}
	c := newCamera()
	c.layout(800, 600)
	c.drag(37, -12)
	c.drag(-150, 80)

	checked := 0
	for idx := range g.NumCells() {
		cell := g.CellAt(idx)
		x, y, visible := c.project(g.DirFromCell(cell))
		if !visible {
			continue
		}
		got, ok := c.pick(g, x, y)
		if !ok || got != cell {
			t.Fatalf("pick(project(%v)) = %v, %v", cell, got, ok)
		}
		checked++
	}
	if checked < g.NumCells()/3 {
		t.Fatalf("only %d cells visible, expected roughly half", checked)
	}
}

func TestPickOffPlanet(t *testing.T) {
	c := newCamera()
	c.layout(800, 600)
	if _, ok := c.pick(sim.Grid{N: 16}, 5, 5); ok {
		t.Fatal("pick in the corner of the screen hit the planet")
	}
}

func TestDragFollowsPointer(t *testing.T) {
	c := newCamera()
	c.layout(800, 600)
	front := c.rot[2] // planet point currently facing the viewer
	c.drag(40, 0)
	x, y, _ := c.project(front)
	if x <= c.cx || math.Abs(y-c.cy) > 1e-6 {
		t.Fatalf("after dragging right, front point is at (%.1f, %.1f); want right of centre (%.1f, %.1f)", x, y, c.cx, c.cy)
	}
	c.drag(0, 40)
	_, y, _ = c.project(front)
	if y <= c.cy {
		t.Fatalf("after dragging down, front point y = %.1f; want below centre %.1f", y, c.cy)
	}
}

func TestRotationStaysOrthonormal(t *testing.T) {
	c := newCamera()
	c.layout(800, 600)
	for i := range 10000 {
		c.drag(float64(i%7)-3, float64(i%5)-2)
	}
	for i := range 3 {
		for j := range 3 {
			want := 0.0
			if i == j {
				want = 1
			}
			if got := c.rot[i].Dot(c.rot[j]); math.Abs(got-want) > 1e-9 {
				t.Fatalf("rot[%d]·rot[%d] = %v, want %v", i, j, got, want)
			}
		}
	}
}
