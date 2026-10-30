package game

import (
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// UI colours, matching the UX mocks.
var (
	colText     = rgba(0xE8EDF5)
	colBody     = rgba(0xB8C3D4)
	colMuted    = rgba(0x93A0B4)
	colIcon     = rgba(0xC9D3E3)
	colChip     = rgba(0x141B26)
	colSheet    = rgba(0x10161F)
	colCard     = rgba(0x1A2230)
	colBorder   = rgba(0x263142)
	colActiveBg = rgba(0x22324A)
	colActive   = rgba(0x5AA9FF)
	colTrack    = rgba(0x263142)
	colAmber    = rgba(0xF2A541)
	colAmberBg  = rgba(0x241A0E)
	colAmberLn  = rgba(0x6B4A1E)
	colAmberTx  = rgba(0xFFE2B8)
	colOnAmber  = rgba(0x1B1306)
	colTeal     = rgba(0x3FD0B5)
	colTealBg   = rgba(0x13302B)
	colTealTx   = rgba(0x7EE6D2)
	colBlue     = rgba(0x9CC4FF)
	colRed      = rgba(0xFF8A7A)
	colRedBg    = rgba(0x2A1414)
	colRedLn    = rgba(0x6A2E28)
)

func rgba(hex uint32) color.RGBA {
	return color.RGBA{uint8(hex >> 16), uint8(hex >> 8), uint8(hex), 255}
}

func withAlpha(c color.RGBA, a uint8) color.RGBA {
	// Colours are premultiplied.
	f := float64(a) / 255
	return color.RGBA{uint8(float64(c.R) * f), uint8(float64(c.G) * f), uint8(float64(c.B) * f), a}
}

// ui is an immediate-mode UI pass. The same layout code runs in three modes:
// drawing, hit testing a tap, and measuring (neither), so what's drawn and
// what's tappable can never disagree. Coordinates are in device-independent
// pixels (dp), scaled by s to screen pixels.
type ui struct {
	g   *Game
	dst *ebiten.Image // nil unless drawing
	s   float64

	tapping    bool
	tapX, tapY float64 // in dp
	hit        bool
	action     func()

	measuring bool // laying out only: no drawing, no hits
}

func (u *ui) drawing() bool { return u.dst != nil && !u.measuring }

// area registers a tappable (or tap-blocking, if action is nil) rectangle.
// Later areas are on top, so they win.
func (u *ui) area(x, y, w, h float64, action func()) {
	if !u.tapping || u.measuring {
		return
	}
	if u.tapX >= x && u.tapX < x+w && u.tapY >= y && u.tapY < y+h {
		u.hit = true
		u.action = action
	}
}

func (u *ui) roundRectPath(x, y, w, h, r float64) *vector.Path {
	s := u.s
	x, y, w, h, r = x*s, y*s, w*s, h*s, math.Min(r*s, math.Min(w, h)/2)
	var p vector.Path
	p.MoveTo(f32(x+r), f32(y))
	p.LineTo(f32(x+w-r), f32(y))
	p.Arc(f32(x+w-r), f32(y+r), f32(r), -math.Pi/2, 0, vector.Clockwise)
	p.LineTo(f32(x+w), f32(y+h-r))
	p.Arc(f32(x+w-r), f32(y+h-r), f32(r), 0, math.Pi/2, vector.Clockwise)
	p.LineTo(f32(x+r), f32(y+h))
	p.Arc(f32(x+r), f32(y+h-r), f32(r), math.Pi/2, math.Pi, vector.Clockwise)
	p.LineTo(f32(x), f32(y+r))
	p.Arc(f32(x+r), f32(y+r), f32(r), math.Pi, 3*math.Pi/2, vector.Clockwise)
	p.Close()
	return &p
}

func (u *ui) fillRound(x, y, w, h, r float64, c color.Color) {
	if !u.drawing() {
		return
	}
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(c)
	vector.FillPath(u.dst, u.roundRectPath(x, y, w, h, r), nil, op)
}

func (u *ui) strokeRound(x, y, w, h, r, width float64, c color.Color) {
	if !u.drawing() {
		return
	}
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(c)
	vector.StrokePath(u.dst, u.roundRectPath(x, y, w, h, r), &vector.StrokeOptions{Width: f32(width * u.s)}, op)
}

// panel draws a rounded box with a border and blocks taps on it.
func (u *ui) panel(x, y, w, h, r float64, fill, border color.Color) {
	u.fillRound(x, y, w, h, r, fill)
	if border != nil {
		u.strokeRound(x+0.5, y+0.5, w-1, h-1, r, 1, border)
	}
	u.area(x, y, w, h, nil)
}

func (u *ui) circle(cx, cy, r float64, c color.Color) {
	if u.drawing() {
		vector.FillCircle(u.dst, f32(cx*u.s), f32(cy*u.s), f32(r*u.s), c, true)
	}
}

func (u *ui) face(kind fontKind, size float64, weight int) *text.GoTextFace {
	return face(kind, size*u.s, weight)
}

// textW is the width of str in dp.
func (u *ui) textW(str string, kind fontKind, size float64, weight int) float64 {
	return text.Advance(str, u.face(kind, size, weight)) / u.s
}

// lineH is the height of one line in dp.
func (u *ui) lineH(kind fontKind, size float64, weight int) float64 {
	return lineHeight(u.face(kind, size, weight)) / u.s
}

// text draws str with its top-left at (x, y) and returns its width.
func (u *ui) text(str string, x, y float64, kind fontKind, size float64, weight int, c color.Color) float64 {
	f := u.face(kind, size, weight)
	if u.drawing() {
		op := &text.DrawOptions{}
		op.GeoM.Translate(x*u.s, y*u.s)
		op.ColorScale.ScaleWithColor(c)
		text.Draw(u.dst, str, f, op)
	}
	return text.Advance(str, f) / u.s
}

// textRight draws str with its top-right corner at (x, y).
func (u *ui) textRight(str string, x, y float64, kind fontKind, size float64, weight int, c color.Color) {
	u.text(str, x-u.textW(str, kind, size, weight), y, kind, size, weight, c)
}

// paragraph draws wrapped text and returns its height.
func (u *ui) paragraph(str string, x, y, w float64, size float64, c color.Color) float64 {
	f := u.face(sans, size, regular)
	lh := size * 1.45
	lines := wrapWidth(str, f, w*u.s)
	for i, line := range lines {
		u.text(line, x, y+float64(i)*lh, sans, size, regular, c)
	}
	return float64(len(lines)) * lh
}

// paragraphMax is paragraph cut to at most maxLines, ending in "…" if
// anything was left out.
func (u *ui) paragraphMax(str string, x, y, w float64, size float64, c color.Color, maxLines int) float64 {
	f := u.face(sans, size, regular)
	lh := size * 1.45
	lines := wrapWidth(str, f, w*u.s)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := strings.Fields(lines[maxLines-1])
		for len(last) > 1 && u.textW(strings.Join(last, " ")+"…", sans, size, regular) > w {
			last = last[:len(last)-1]
		}
		lines[maxLines-1] = strings.Join(last, " ") + "…"
	}
	for i, line := range lines {
		u.text(line, x, y+float64(i)*lh, sans, size, regular, c)
	}
	return float64(len(lines)) * lh
}

func f32(v float64) float32 { return float32(v) }

// iconName picks one of the stroke icons from the mocks.
type iconName int

const (
	iconThermo iconName = iota
	iconDrop
	iconLeaf
	iconBolt
	iconMetal
	iconAir
	iconGlobe
	iconIndustry
	iconBuild
	iconLink
	iconInspect
	iconRemove
	iconPause
	iconCheck
	iconCross
	iconChevronUp
	iconChevronDown
	iconWarning
)

// icon draws a 24-unit icon scaled to size dp at (x, y).
func (u *ui) icon(name iconName, x, y, size float64, c color.Color) {
	if !u.drawing() {
		return
	}
	k := size / 24 * u.s
	px := func(v float64) float32 { return f32(x*u.s + v*k) }
	py := func(v float64) float32 { return f32(y*u.s + v*k) }
	var p vector.Path
	m := func(a, b float64) { p.MoveTo(px(a), py(b)) }
	l := func(a, b float64) { p.LineTo(px(a), py(b)) }
	arc := func(cx, cy, r, a0, a1 float64) {
		p.Arc(px(cx), py(cy), f32(r*k), f32(a0), f32(a1), vector.Clockwise)
	}
	circ := func(cx, cy, r float64) {
		m(cx+r, cy)
		arc(cx, cy, r, 0, 2*math.Pi)
		p.Close()
	}
	fill := false
	switch name {
	case iconThermo:
		m(10, 14)
		l(10, 4)
		arc(12, 4, 2, math.Pi, 2*math.Pi)
		l(14, 14)
		arc(12, 17, 3.6, -math.Pi/3.2, math.Pi+math.Pi/3.2)
		p.Close()
	case iconDrop:
		m(12, 3)
		p.CubicTo(px(14), py(6), px(18), py(10), px(18), py(14))
		arc(12, 14, 6, 0, math.Pi)
		p.CubicTo(px(6), py(10), px(10), py(6), px(12), py(3))
		p.Close()
	case iconLeaf:
		m(5, 19)
		p.CubicTo(px(5), py(11), px(11), py(6), px(19), py(5))
		p.CubicTo(px(18), py(13), px(13), py(19), px(5), py(19))
		p.Close()
		m(5, 19)
		l(12, 12)
	case iconBolt:
		m(13, 2)
		l(4, 14)
		l(11, 14)
		l(10, 22)
		l(19, 10)
		l(12, 10)
		p.Close()
		fill = true
	case iconMetal:
		m(4, 9)
		l(12, 4)
		l(20, 9)
		l(12, 14)
		p.Close()
		m(4, 14)
		l(12, 19)
		l(20, 14)
	case iconAir:
		m(3, 8)
		l(14, 8)
		arc(14, 5, 3, math.Pi/2, math.Pi/2+math.Pi*1.4)
		m(3, 12)
		l(17, 12)
		arc(17, 15, 3, -math.Pi/2, -math.Pi/2+math.Pi*1.4)
		m(3, 16)
		l(11, 16)
	case iconGlobe:
		circ(12, 12, 9)
		m(5, 9)
		p.CubicTo(px(8), py(10), px(9), py(7), px(12), py(8))
		p.CubicTo(px(15), py(9), px(15), py(12), px(19), py(10))
		m(6, 16)
		p.CubicTo(px(8), py(15), px(10), py(17), px(12), py(16))
	case iconIndustry:
		m(3, 20)
		l(3, 10)
		l(8, 13)
		l(8, 10)
		l(13, 13)
		l(13, 6)
		l(17, 6)
		l(18, 20)
		p.Close()
	case iconBuild:
		m(4, 20)
		l(20, 20)
		m(6, 20)
		l(6, 10)
		l(12, 5)
		l(18, 10)
		l(18, 20)
		m(10, 20)
		l(10, 15)
		l(14, 15)
		l(14, 20)
	case iconLink:
		circ(6, 18, 2.5)
		circ(18, 6, 2.5)
		m(8, 16)
		l(16, 8)
	case iconInspect:
		circ(11, 11, 6)
		m(20, 20)
		l(15.5, 15.5)
	case iconRemove:
		m(4, 7)
		l(20, 7)
		m(9, 7)
		l(9, 4)
		l(15, 4)
		l(15, 7)
		m(6, 7)
		l(7, 20)
		l(17, 20)
		l(18, 7)
	case iconPause:
		u.fillRound(x+size*6/24, y+size*5/24, size*4/24, size*14/24, 1, c)
		u.fillRound(x+size*14/24, y+size*5/24, size*4/24, size*14/24, 1, c)
		return
	case iconCheck:
		m(5, 12)
		l(10, 17)
		l(19, 7)
	case iconCross:
		m(6, 6)
		l(18, 18)
		m(18, 6)
		l(6, 18)
	case iconChevronUp:
		m(6, 15)
		l(12, 9)
		l(18, 15)
	case iconChevronDown:
		m(6, 9)
		l(12, 15)
		l(18, 9)
	case iconWarning:
		m(12, 3.5)
		l(22, 20.5)
		l(2, 20.5)
		p.Close()
		m(12, 10)
		l(12, 14)
		m(12, 17)
		l(12, 17.2)
	}
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(c)
	if fill {
		vector.FillPath(u.dst, &p, nil, op)
		return
	}
	vector.StrokePath(u.dst, &p, &vector.StrokeOptions{
		Width:    f32(math.Max(2*k, 1)),
		LineCap:  vector.LineCapRound,
		LineJoin: vector.LineJoinRound,
	}, op)
}
