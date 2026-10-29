package game

import (
	"math"
	"testing"

	"github.com/zachaller/terraformer/sim"
)

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

func TestTurnTowardsCentresDirection(t *testing.T) {
	c := newCamera()
	c.layout(800, 600)
	d := sim.Vec3{X: 0.6, Y: -0.3, Z: 0.5}.Normalize()
	for range 200 {
		if c.turnTowards(d, 0.2) {
			break
		}
	}
	x, y, visible := c.project(d)
	if !visible || math.Abs(x-c.cx) > 1 || math.Abs(y-c.cy) > 1 {
		t.Fatalf("direction ended at (%.1f, %.1f), want the centre (%.1f, %.1f)", x, y, c.cx, c.cy)
	}
}

func TestAreaCellsFollowNeighbours(t *testing.T) {
	g := sim.Grid{N: 8}
	// A tile on a face seam: its neighbourhood crosses onto another face.
	c := sim.Cell{Face: 4, I: 3, J: 7}
	one := areaCells(g, c, 1)
	if len(one) != 4 {
		t.Fatalf("one step from %v: %d cells, want 4", c, len(one))
	}
	for i, nb := range g.Neighbors(c) {
		if one[i] != nb {
			t.Fatalf("one step from %v = %v, want its neighbours %v", c, one, g.Neighbors(c))
		}
	}
	if two := areaCells(g, c, 2); len(two) != 12 {
		t.Fatalf("two steps from %v: %d cells, want 12", c, len(two))
	}
}
