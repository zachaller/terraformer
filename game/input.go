package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// tapSlop is how far, in pixels, a pointer may move and still count as a tap.
const tapSlop = 8

// gestures is one tick's worth of interpreted input.
type gestures struct {
	dragX, dragY float64
	dragging     bool
	zoom         float64 // multiplicative; 1 means no change
	tap          bool
	tapX, tapY   float64

	// press is set on the tick a single pointer goes down, at (pressX,
	// pressY); release on the tick it lifts, at (releaseX, releaseY).
	// While it's down, (curX, curY) is where it is.
	press, release     bool
	pressX, pressY     float64
	releaseX, releaseY float64
	curX, curY         float64
	// pinching is set while two fingers are down.
	pinching bool
}

// pointerInput turns mouse and touch input into drag, pinch and tap gestures,
// so the rest of the game doesn't care which one it's running with.
type pointerInput struct {
	down         bool
	lastX, lastY float64
	moved        float64
	// pinched is set once a second finger lands, so lifting back to one
	// finger neither jumps the drag nor counts as a tap.
	pinched   bool
	pinchDist float64
	touches   []ebiten.TouchID
}

func (p *pointerInput) update() gestures {
	gs := gestures{zoom: 1}

	_, wy := ebiten.Wheel()
	if wy != 0 {
		gs.zoom *= math.Pow(1.1, wy)
	}

	p.touches = ebiten.AppendTouchIDs(p.touches[:0])
	if len(p.touches) >= 2 {
		ax, ay := ebiten.TouchPositionF(p.touches[0])
		bx, by := ebiten.TouchPositionF(p.touches[1])
		d := math.Hypot(ax-bx, ay-by)
		if p.pinchDist > 0 && d > 0 {
			gs.zoom *= d / p.pinchDist
		}
		p.pinchDist = d
		p.pinched = true
		p.down = false
		gs.pinching = true
		return gs
	}
	p.pinchDist = 0

	var x, y float64
	pressed := false
	switch {
	case len(p.touches) == 1:
		x, y = ebiten.TouchPositionF(p.touches[0])
		pressed = true
	case ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft):
		cx, cy := ebiten.CursorPosition()
		x, y = float64(cx), float64(cy)
		pressed = true
	}

	switch {
	case pressed && !p.down:
		p.down = true
		p.lastX, p.lastY = x, y
		p.moved = 0
		gs.dragging = true
		gs.press, gs.pressX, gs.pressY = true, x, y
		gs.curX, gs.curY = x, y
	case pressed:
		gs.dragX, gs.dragY = x-p.lastX, y-p.lastY
		gs.dragging = true
		p.moved += math.Hypot(gs.dragX, gs.dragY)
		p.lastX, p.lastY = x, y
		gs.curX, gs.curY = x, y
	case p.down:
		p.down = false
		gs.release, gs.releaseX, gs.releaseY = true, p.lastX, p.lastY
		if !p.pinched && p.moved < tapSlop {
			gs.tap, gs.tapX, gs.tapY = true, p.lastX, p.lastY
		}
	}
	if !pressed {
		p.pinched = false
	}
	return gs
}
