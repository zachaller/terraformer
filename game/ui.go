package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Button size and spacing in device-independent pixels; scaled by the
// device scale factor so they stay finger-sized on phones.
const (
	buttonW      = 130
	buttonH      = 28
	buttonGap    = 8
	buttonMargin = 12
)

// tool is what tapping the planet does.
type tool int

const (
	toolInspect tool = iota
	toolHeat
)

func (t tool) String() string {
	if t == toolHeat {
		return "Heater"
	}
	return "Inspect"
}

var speeds = []struct {
	label string
	steps float64 // simulation steps per second
}{
	{"Paused", 0},
	{"1x", 10},
	{"4x", 40},
}

type button struct {
	label      string
	x, y, w, h float64 // screen pixels
	action     func()
}

func (b button) contains(x, y float64) bool {
	return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h
}

// buttons lays out the control bar along the bottom of the screen.
func (g *Game) buttons() []button {
	s := ebiten.Monitor().DeviceScaleFactor()
	w, h := buttonW*s, buttonH*s
	y := float64(g.h) - h - buttonMargin*s
	x := buttonMargin * s
	defs := []struct {
		label  string
		action func()
	}{
		{"Tool: " + g.tool.String(), func() { g.tool = 1 - g.tool }},
		{"View: " + g.view.String(), func() { g.view = (g.view + 1) % numViews; g.atlasDirty = true }},
		{"Speed: " + speeds[g.speed].label, func() { g.speed = (g.speed + 1) % len(speeds) }},
	}
	bs := make([]button, len(defs))
	for i, d := range defs {
		bs[i] = button{label: d.label, x: x, y: y, w: w, h: h, action: d.action}
		x += w + buttonGap*s
	}
	return bs
}

// tapButton runs the action of the button at (x, y), if any.
func (g *Game) tapButton(x, y float64) bool {
	for _, b := range g.buttons() {
		if b.contains(x, y) {
			b.action()
			return true
		}
	}
	return false
}

var (
	buttonFill   = color.RGBA{20, 24, 40, 200}
	buttonBorder = color.RGBA{120, 140, 200, 255}
)

func (g *Game) drawButtons(screen *ebiten.Image) {
	s := ebiten.Monitor().DeviceScaleFactor()
	if g.label == nil {
		g.label = ebiten.NewImage(buttonW, buttonH)
	}
	for _, b := range g.buttons() {
		vector.FillRect(screen, float32(b.x), float32(b.y), float32(b.w), float32(b.h), buttonFill, false)
		vector.StrokeRect(screen, float32(b.x), float32(b.y), float32(b.w), float32(b.h), float32(s), buttonBorder, false)
		// The debug font is fixed-size, so render the label at 1x and scale it.
		g.label.Clear()
		ebitenutil.DebugPrintAt(g.label, b.label, 8, 6)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(s, s)
		op.GeoM.Translate(b.x, b.y)
		screen.DrawImage(g.label, op)
	}
}
