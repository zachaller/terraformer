// Package game is the Ebitengine front end: rendering, input and UI.
package game

import (
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/zachaller/terraformer/sim"
)

//go:embed planet.kage
var planetShaderSrc []byte

// lightDir is the sun direction in view space, so the side facing the player
// is always lit.
var lightDir = sim.Vec3{X: -0.5, Y: 0.5, Z: 0.75}.Normalize()

// Options configure a new Game.
type Options struct {
	Seed     uint64
	GridSize int // cells per cube face edge

	// ScreenshotPath, if set, selects the tile at the centre of the screen,
	// saves a PNG of the first frames to this path and exits. Used to check
	// rendering without a display.
	ScreenshotPath string
}

// Game implements ebiten.Game.
type Game struct {
	opts   Options
	planet *sim.Planet
	atlas  *ebiten.Image
	shader *ebiten.Shader

	cam      camera
	input    pointerInput
	selected *sim.Cell

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
	return &Game{
		opts:   opts,
		planet: p,
		atlas:  newAtlasImage(p),
		shader: shader,
		cam:    newCamera(),
	}, nil
}

func (g *Game) Update() error {
	if g.done {
		return ebiten.Termination
	}
	g.ticks++
	g.cam.layout(g.w, g.h)

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
	if gs.tap {
		g.selectAt(gs.tapX, gs.tapY)
	}

	if g.opts.ScreenshotPath != "" && g.ticks == 2 {
		g.selectAt(g.cam.cx, g.cam.cy)
	}
	return nil
}

func (g *Game) selectAt(x, y float64) {
	if c, ok := g.cam.pick(g.planet.Grid, x, y); ok {
		g.selected = &c
	} else {
		g.selected = nil
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.cam.layout(g.w, g.h)
	g.drawPlanet(screen)
	g.drawHUD(screen)

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
	msg := fmt.Sprintf("Drag: rotate   Wheel/pinch: zoom   Click/tap: select\nFPS %.0f   Tiles %d",
		ebiten.ActualFPS(), g.planet.Grid.NumCells())
	if c := g.selected; c != nil {
		t := g.planet.Tile(*c)
		msg += fmt.Sprintf("\nSelected: face %d (%d, %d)  %v  height %+.2f", c.Face, c.I, c.J, t.Kind, t.Height)
	}
	ebitenutil.DebugPrint(screen, msg)
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
