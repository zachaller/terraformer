package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/zachaller/terraformer/sim"
)

// Button size and spacing in device-independent pixels; scaled by the
// device scale factor so they stay finger-sized on phones.
const (
	buttonW      = 130 // widest a button gets; narrower on small screens
	buttonH      = 44  // close to Android's 48dp minimum touch target
	buttonGap    = 8
	buttonMargin = 12
	labelW       = 130 // size of the 1x image labels are rendered into
	labelH       = 16
	menuColumns  = 4
)

// tool is what tapping the planet does.
type tool int

const (
	toolLand tool = iota
	toolInspect
	toolLink
	toolDemolish
	toolSolar
	toolMine
	toolSmelter
	toolHeater
)

// menuTools are the tools offered in the build menu, in order.
var menuTools = []tool{toolInspect, toolLink, toolDemolish, toolSolar, toolMine, toolSmelter, toolHeater}

// building returns the building type a tool places, if it places one.
func (t tool) building() (sim.BuildingType, bool) {
	switch t {
	case toolSolar:
		return sim.SolarPanel, true
	case toolMine:
		return sim.Mine, true
	case toolSmelter:
		return sim.Smelter, true
	case toolHeater:
		return sim.HeaterMachine, true
	}
	return 0, false
}

func (t tool) String() string {
	if bt, ok := t.building(); ok {
		return bt.String()
	}
	switch t {
	case toolLand:
		return "Land"
	case toolLink:
		return "Link"
	case toolDemolish:
		return "Demolish"
	}
	return "Inspect"
}

// menuLabel is the tool's build menu label, with its cost if it has one.
func (t tool) menuLabel() string {
	if bt, ok := t.building(); ok {
		return fmt.Sprintf("%s %.0f", bt, sim.Specs[bt].Cost)
	}
	return t.String()
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
	active     bool    // drawn highlighted
	action     func()
}

func (b button) contains(x, y float64) bool {
	return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h
}

// setTool switches tool, closing the menu and cancelling a half-made link.
func (g *Game) setTool(t tool) {
	g.tool = t
	g.menuOpen = false
	g.linkFrom = nil
}

// buttons lays out the control bar along the bottom of the screen and, when
// open, the build menu above it, all inside the safe area. Buttons shrink to
// fit narrow screens.
func (g *Game) buttons() []button {
	s := ebiten.Monitor().DeviceScaleFactor()
	l, _, r, b := currentSafeArea()
	avail := float64(g.w) - l - r - 2*buttonMargin*s
	gap := buttonGap * s
	h := buttonH * s
	y := float64(g.h) - b - h - buttonMargin*s
	row := func(n int) (x, w float64) {
		return l + buttonMargin*s, math.Min(buttonW*s, (avail-float64(n-1)*gap)/float64(n))
	}

	var bs []button
	toolLabel := "Tool: " + g.tool.String()
	if g.menuOpen {
		toolLabel = "Close menu"
	}
	bar := []button{
		{label: toolLabel, active: g.menuOpen, action: func() {
			if g.planet.Landed() {
				g.menuOpen = !g.menuOpen
			} else {
				g.showMessage("Tap a tile to choose your landing site")
			}
		}},
		{label: "View: " + g.view.String(), action: func() { g.view = (g.view + 1) % numViews; g.atlasDirty = true }},
		{label: "Speed: " + speeds[g.speed].label, action: func() { g.speed = (g.speed + 1) % len(speeds) }},
	}
	x, w := row(len(bar))
	for _, bt := range bar {
		bt.x, bt.y, bt.w, bt.h = x, y, w, h
		bs = append(bs, bt)
		x += w + gap
	}

	if g.menuOpen {
		x0, w := row(menuColumns)
		rows := (len(menuTools) + 1 + menuColumns - 1) / menuColumns // +1 for New planet
		for i, t := range menuTools {
			col, rowIdx := i%menuColumns, i/menuColumns
			bs = append(bs, button{
				label:  t.menuLabel(),
				x:      x0 + float64(col)*(w+gap),
				y:      y - float64(rows-rowIdx)*(h+gap),
				w:      w,
				h:      h,
				active: t == g.tool,
				action: func() { g.setTool(t) },
			})
		}
		i := len(menuTools)
		label := "New planet"
		if g.ticks < g.confirmNewUntil {
			label = "Sure? Tap"
		}
		bs = append(bs, button{
			label: label,
			x:     x0 + float64(i%menuColumns)*(w+gap),
			y:     y - float64(rows-i/menuColumns)*(h+gap),
			w:     w,
			h:     h,
			action: func() {
				if g.ticks < g.confirmNewUntil {
					g.confirmNewUntil = 0
					g.newPlanet()
					return
				}
				g.confirmNewUntil = g.ticks + 3*ebiten.TPS()
				g.showMessage("Tap New planet again to abandon this planet and start over")
			},
		})
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
	buttonFill       = color.RGBA{20, 24, 40, 220}
	buttonFillActive = color.RGBA{60, 80, 140, 230}
	buttonBorder     = color.RGBA{120, 140, 200, 255}
)

func (g *Game) drawButtons(screen *ebiten.Image) {
	s := ebiten.Monitor().DeviceScaleFactor()
	if g.label == nil {
		g.label = ebiten.NewImage(labelW, labelH)
	}
	for _, b := range g.buttons() {
		fill := buttonFill
		if b.active {
			fill = buttonFillActive
		}
		vector.FillRect(screen, float32(b.x), float32(b.y), float32(b.w), float32(b.h), fill, false)
		vector.StrokeRect(screen, float32(b.x), float32(b.y), float32(b.w), float32(b.h), float32(s), buttonBorder, false)
		// The debug font is fixed-size (6px per character), so render the
		// label at 1x and scale it: s normally, less if it wouldn't fit.
		g.label.Clear()
		ebitenutil.DebugPrintAt(g.label, b.label, 8, 0)
		ls := math.Min(s, b.w/float64(6*len(b.label)+16))
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(ls, ls)
		op.GeoM.Translate(b.x, b.y+(b.h-labelH*ls)/2)
		screen.DrawImage(g.label, op)
	}
}
