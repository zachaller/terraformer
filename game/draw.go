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

// shape draws a rounded rectangle at (x, y), w×h screen pixels with corner
// radius r, filled, or outlined width wide if width > 0, with the shape
// shader: one quad, so consecutive shapes draw as one batch.
func (u *ui) shape(x, y, w, h, r, width float64, c color.Color) {
	cr, cg, cb, ca := c.RGBA()
	if ca == 0 || w <= 0 || h <= 0 {
		return
	}
	hw, hh := w/2, h/2
	r = math.Min(r, math.Min(hw, hh))
	cx, cy := x+hw, y+hh
	pad := 1 + width/2 // room for the anti-aliased edge and outline
	cust := [4]float32{f32(hw), f32(hh), f32(r), f32(width)}
	v := u.g.shapeVerts[:0]
	for _, k := range [4][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		lx, ly := k[0]*(hw+pad), k[1]*(hh+pad)
		v = append(v, ebiten.Vertex{
			DstX: f32(cx + lx), DstY: f32(cy + ly),
			SrcX: f32(lx), SrcY: f32(ly),
			ColorR: float32(cr) / 0xffff, ColorG: float32(cg) / 0xffff,
			ColorB: float32(cb) / 0xffff, ColorA: float32(ca) / 0xffff,
			Custom0: cust[0], Custom1: cust[1], Custom2: cust[2], Custom3: cust[3],
		})
	}
	u.g.shapeVerts = v
	u.dst.DrawTrianglesShader(v, shapeIndices, u.g.shapeShader, nil)
}

var shapeIndices = []uint16{0, 1, 2, 1, 3, 2}

func (u *ui) fillRound(x, y, w, h, r float64, c color.Color) {
	if !u.drawing() {
		return
	}
	s := u.s
	u.shape(x*s, y*s, w*s, h*s, r*s, 0, c)
}

func (u *ui) strokeRound(x, y, w, h, r, width float64, c color.Color) {
	if !u.drawing() {
		return
	}
	s := u.s
	u.shape(x*s, y*s, w*s, h*s, r*s, width*s, c)
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
		s := u.s
		u.shape((cx-r)*s, (cy-r)*s, 2*r*s, 2*r*s, r*s, 0, c)
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
	u.sprite(spriteIcon, int(name), x, y, size, c, func(v *ui, x, y float64) {
		v.drawIcon(name, x, y, size, color.White)
	})
}

// Kinds of sprite.
const (
	spriteIcon = iota
	spriteGlyph
)

type spriteKey struct {
	kind, id int
	size     float64 // screen pixels
	fx, fy   uint8   // position within the pixel, in quarters
}

// sprite draws a size×size dp picture at (x, y) from a cache of images,
// drawing it with draw (at the given position, in dp) the first time it's
// needed at this size and sub-pixel position. Pictures drawn in white can
// be tinted by c; nil leaves them as drawn. Vector paths are costly to fill
// every frame, and a cached image draws in the same batch as its neighbours.
func (u *ui) sprite(kind, id int, x, y, size float64, c color.Color, draw func(v *ui, x, y float64)) {
	X, Y := x*u.s, y*u.s
	ix, iy := math.Floor(X), math.Floor(Y)
	fx, fy := math.Round((X-ix)*4), math.Round((Y-iy)*4)
	key := spriteKey{kind, id, size * u.s, uint8(fx), uint8(fy)}
	pad := math.Ceil(size*u.s*0.25) + 2 // strokes and edges can stray outside the box
	g := u.g
	img := g.sprites[key]
	if img == nil {
		if len(g.sprites) >= 512 {
			for _, im := range g.sprites {
				im.Deallocate()
			}
			clear(g.sprites)
		}
		if g.sprites == nil {
			g.sprites = map[spriteKey]*ebiten.Image{}
		}
		n := int(math.Ceil(size*u.s + 2*pad + 1))
		img = ebiten.NewImage(n, n)
		v := *u
		v.dst = img
		v.tapping = false
		draw(&v, (pad+fx/4)/u.s, (pad+fy/4)/u.s)
		g.sprites[key] = img
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(ix-pad, iy-pad)
	if c != nil {
		op.ColorScale.ScaleWithColor(c)
	}
	u.dst.DrawImage(img, op)
}

// drawIcon draws icon name with vector paths; see icon.
func (u *ui) drawIcon(name iconName, x, y, size float64, c color.Color) {
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
