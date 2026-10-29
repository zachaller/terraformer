// Package game is the Ebitengine front end: rendering, input and UI.
package game

import (
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/zachaller/terraformer/sim"
)

//go:embed planet.kage
var planetShaderSrc []byte

// lightDir is the sun direction in view space, so the side facing the player
// is always lit.
var lightDir = sim.Vec3{X: -0.5, Y: 0.5, Z: 0.75}.Normalize()

// heaterPower is how much the heater tool warms a tile per tap, in °C.
const heaterPower = 30

// Options configure a new Game.
type Options struct {
	Seed     uint64
	GridSize int // cells per cube face edge

	// DemoSteps, if positive, places a ring of heaters around the equator and
	// fast-forwards this many simulation steps before the game starts.
	DemoSteps int
	// View is the starting view: surface, temperature, water or life.
	View string

	// ScreenshotPath, if set, selects the tile at the centre of the screen,
	// saves a PNG of the first frames to this path and exits. Used to check
	// rendering without a display.
	ScreenshotPath string
}

// Game implements ebiten.Game.
type Game struct {
	opts   Options
	planet *sim.Planet
	shader *ebiten.Shader

	atlas      *ebiten.Image
	atlasPix   []byte
	atlasDirty bool

	hud, label *ebiten.Image

	cam      camera
	input    pointerInput
	selected *sim.Cell

	tool     tool
	view     viewMode
	speed    int     // index into speeds
	stepDebt float64 // fractional simulation steps owed

	w, h  int
	ticks int
	done  bool
}

// New creates a game with a freshly generated planet.
func New(opts Options) (*Game, error) {
	shader, err := ebiten.NewShader(planetShaderSrc)
	if err != nil {
		return nil, fmt.Errorf("compiling planet shader: %w", err)
	}
	p := sim.NewPlanet(opts.GridSize, opts.Seed)
	n := p.Grid.N
	g := &Game{
		opts:       opts,
		planet:     p,
		shader:     shader,
		atlas:      ebiten.NewImage(sim.Faces*n, n),
		atlasPix:   make([]byte, sim.Faces*n*n*4),
		atlasDirty: true,
		cam:        newCamera(),
		speed:      1,
	}
	for v := range numViews {
		if strings.EqualFold(opts.View, v.String()) || (opts.View == "temp" && v == viewTemp) {
			g.view = v
		}
	}
	if opts.DemoSteps > 0 {
		placeDemoHeaters(p)
		for range opts.DemoSteps {
			p.Step()
		}
	}
	return g, nil
}

// placeDemoHeaters puts a ring of heaters around the equator: enough to tip
// the planet into runaway warming.
func placeDemoHeaters(p *sim.Planet) {
	for idx := range p.Tiles {
		c := p.Grid.CellAt(idx)
		if d := p.Grid.DirFromCell(c); math.Abs(d.Y) < 0.05 && idx%7 == 0 {
			p.AddHeat(c, heaterPower)
		}
	}
}

func (g *Game) Update() error {
	if g.done {
		return ebiten.Termination
	}
	g.ticks++
	g.layoutCamera()

	g.handleKeys()
	gs := g.input.update()
	g.cam.zoomBy(gs.zoom)
	switch {
	case gs.dragging:
		g.cam.drag(gs.dragX, gs.dragY)
	case gs.tap:
		g.cam.stop()
	default:
		g.cam.coast()
	}
	if gs.tap && !g.tapButton(gs.tapX, gs.tapY) {
		g.tapPlanet(gs.tapX, gs.tapY)
	}

	if g.opts.ScreenshotPath != "" && g.ticks == 2 {
		g.selectAt(g.cam.cx, g.cam.cy)
	}

	g.stepDebt += speeds[g.speed].steps / float64(ebiten.TPS())
	for g.stepDebt >= 1 {
		g.planet.Step()
		g.stepDebt--
		g.atlasDirty = true
	}
	return nil
}

// handleKeys provides desktop shortcuts for the on-screen buttons.
func (g *Game) handleKeys() {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyH):
		g.tool = 1 - g.tool
	case inpututil.IsKeyJustPressed(ebiten.KeyV):
		g.view = (g.view + 1) % numViews
		g.atlasDirty = true
	case inpututil.IsKeyJustPressed(ebiten.KeySpace):
		if g.speed == 0 {
			g.speed = 1
		} else {
			g.speed = 0
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyF):
		g.speed = len(speeds) - 1
	}
}

// layoutCamera centres the planet in the part of the screen not covered by
// system bars or cutouts.
func (g *Game) layoutCamera() {
	l, t, r, b := currentSafeArea()
	g.cam.layoutIn(l, t, float64(g.w)-l-r, float64(g.h)-t-b)
}

func (g *Game) tapPlanet(x, y float64) {
	g.selectAt(x, y)
	if g.selected != nil && g.tool == toolHeat {
		g.planet.AddHeat(*g.selected, heaterPower)
		g.atlasDirty = true
	}
}

func (g *Game) selectAt(x, y float64) {
	if c, ok := g.cam.pick(g.planet.Grid, x, y); ok {
		g.selected = &c
	} else {
		g.selected = nil
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.layoutCamera()
	if g.atlasDirty {
		fillAtlas(g.atlasPix, g.planet, g.view)
		g.atlas.WritePixels(g.atlasPix)
		g.atlasDirty = false
	}
	g.drawPlanet(screen)
	g.drawHUD(screen)
	g.drawButtons(screen)

	if g.opts.ScreenshotPath != "" && g.ticks >= 10 && !g.done {
		if err := savePNG(screen, g.opts.ScreenshotPath); err != nil {
			fmt.Fprintln(os.Stderr, "screenshot:", err)
		}
		g.done = true
	}
}

func (g *Game) drawPlanet(screen *ebiten.Image) {
	w, h := float32(g.w), float32(g.h)
	aw, ah := float32(g.atlas.Bounds().Dx()), float32(g.atlas.Bounds().Dy())
	vs := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0},
		{DstX: w, DstY: 0, SrcX: aw, SrcY: 0},
		{DstX: 0, DstY: h, SrcX: 0, SrcY: ah},
		{DstX: w, DstY: h, SrcX: aw, SrcY: ah},
	}
	for i := range vs {
		vs[i].ColorR, vs[i].ColorG, vs[i].ColorB, vs[i].ColorA = 1, 1, 1, 1
	}

	sel := []float32{-1, 0, 0}
	if g.selected != nil {
		sel = []float32{float32(g.selected.Face), float32(g.selected.I), float32(g.selected.J)}
	}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = g.atlas
	op.Uniforms = map[string]any{
		"Center":   []float32{float32(g.cam.cx), float32(g.cam.cy)},
		"Radius":   float32(g.cam.radius),
		"Row0":     vec3f(g.cam.rot[0]),
		"Row1":     vec3f(g.cam.rot[1]),
		"Row2":     vec3f(g.cam.rot[2]),
		"N":        float32(g.planet.Grid.N),
		"Selected": sel,
		"Time":     float32(g.ticks) / float32(ebiten.TPS()),
		"Light":    vec3f(lightDir),
	}
	screen.DrawTrianglesShader(vs, []uint16{0, 1, 2, 1, 2, 3}, g.shader, op)
}

func (g *Game) drawHUD(screen *ebiten.Image) {
	p := g.planet
	st := p.Stats()
	msg := fmt.Sprintf("Day %d   Atmosphere %.2f kPa   Avg temp %.1fC\nIce %.0f   Water %.1f   Life on %.1f%% of tiles   FPS %.0f",
		p.Ticks/10, p.Atmosphere, st.AvgTemp, st.Ice, st.Water, st.LifeCover*100, ebiten.ActualFPS())
	if c := g.selected; c != nil {
		t := p.Tile(*c)
		msg += fmt.Sprintf("\n\nTile %d (%d, %d)  %v  height %+.2f\nTemp %.1fC  Ice %.2f  Water %.3f  Life %.0f%%",
			c.Face, c.I, c.J, t.Kind(), t.Height, t.Temp, t.Ice, t.Water, t.Life*100)
		if t.Heat > 0 {
			msg += fmt.Sprintf("  Heater +%.0fC", t.Heat)
		}
		if sim.Habitable(t) {
			msg += "\nConditions suit microbes"
		}
	}
	// The debug font is fixed-size, so draw it offscreen and scale it up to
	// stay readable on high-DPI phone screens.
	if g.hud == nil {
		g.hud = ebiten.NewImage(480, 100)
	}
	g.hud.Clear()
	ebitenutil.DebugPrint(g.hud, msg)
	s := ebiten.Monitor().DeviceScaleFactor()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(s, s)
	l, t, _, _ := currentSafeArea()
	op.GeoM.Translate(l+8*s, t+8*s)
	screen.DrawImage(g.hud, op)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := ebiten.Monitor().DeviceScaleFactor()
	g.w = int(math.Ceil(float64(outsideWidth) * s))
	g.h = int(math.Ceil(float64(outsideHeight) * s))
	return g.w, g.h
}

func vec3f(v sim.Vec3) []float32 { return []float32{float32(v.X), float32(v.Y), float32(v.Z)} }

func savePNG(screen *ebiten.Image, path string) error {
	b := screen.Bounds()
	img := image.NewRGBA(b)
	screen.ReadPixels(img.Pix)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
