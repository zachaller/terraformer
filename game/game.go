// Package game is the Ebitengine front end: rendering, input and UI.
package game

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/zachaller/terraformer/sim"
)

//go:embed planet.kage
var planetShaderSrc []byte

// lightDir is the sun direction in view space, so the side facing the player
// is always lit.
var lightDir = sim.Vec3{X: -0.5, Y: 0.5, Z: 0.75}.Normalize()

// demoHeat is how much each demo heater warms its tile, in °C.
const demoHeat = 30

// messageTime is how long a message stays on screen, in seconds.
const messageTime = 4

// Options configure a new Game.
type Options struct {
	Seed     uint64
	GridSize int // cells per cube face edge

	// DemoHeaters puts fixed warmth on a ring of tiles around the equator.
	DemoHeaters bool
	// DemoColony lands and builds a small working colony facing the camera.
	DemoColony bool
	// DemoSteps fast-forwards this many simulation steps before the game
	// starts.
	DemoSteps int
	// Zoom is the starting zoom (1 fits the planet on screen).
	Zoom float64
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
	menuOpen bool
	linkFrom *sim.Cell // first end of a link being made
	message  string
	msgUntil int // tick the message disappears
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
		atlas:      ebiten.NewImage(sim.Faces*n, 2*n),
		atlasPix:   make([]byte, sim.Faces*n*2*n*4),
		atlasDirty: true,
		cam:        newCamera(),
		speed:      1,
		tool:       toolLand,
	}
	for v := range numViews {
		if strings.EqualFold(opts.View, v.String()) || (opts.View == "temp" && v == viewTemp) {
			g.view = v
		}
	}
	if opts.Zoom > 0 {
		g.cam.zoom = opts.Zoom
	}
	if opts.DemoHeaters {
		placeDemoHeaters(p)
	}
	if opts.DemoColony {
		placeDemoColony(p, g.cam.rot[2])
		g.tool = toolInspect
	}
	for range opts.DemoSteps {
		p.Step()
	}
	return g, nil
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
		if bs := g.planet.Buildings(); len(bs) > 0 {
			c := g.planet.Grid.CellAt(bs[len(bs)-1].Tile)
			g.selected = &c
		}
	}
	if g.ticks == 1 && !g.planet.Landed() {
		g.showMessage("Tap a tile to choose your landing site")
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
	if g.planet.Landed() {
		for i, t := range menuTools {
			if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
				g.setTool(t)
			}
		}
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyB) && g.planet.Landed():
		g.menuOpen = !g.menuOpen
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.menuOpen = false
		g.linkFrom = nil
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

func (g *Game) showMessage(m string) {
	g.message = m
	g.msgUntil = g.ticks + messageTime*ebiten.TPS()
}

// tapPlanet selects the tapped tile and applies the current tool to it.
func (g *Game) tapPlanet(x, y float64) {
	g.menuOpen = false
	g.selectAt(x, y)
	if g.selected == nil {
		g.linkFrom = nil
		return
	}
	c := *g.selected
	p := g.planet
	g.atlasDirty = true

	if bt, ok := g.tool.building(); ok {
		if _, err := p.Build(bt, c); err != nil {
			g.showMessage(err.Error())
		} else {
			g.showMessage(fmt.Sprintf("Built %v. Use Link to connect it", bt))
		}
		return
	}
	switch g.tool {
	case toolLand:
		if err := p.Land(c); err != nil {
			g.showMessage(err.Error())
			return
		}
		g.tool = toolInspect
		g.showMessage("Landed! Open the Tool menu to build")
	case toolDemolish:
		if err := p.Demolish(c); err != nil {
			g.showMessage(err.Error())
		} else {
			g.showMessage("Demolished (half the metal refunded)")
		}
	case toolLink:
		g.tapLink(c)
	}
}

// tapLink makes links one tap at a time: tap a building to start, then tap
// another to link them. The second building becomes the start of the next
// link, so chains are quick to make. Tapping empty ground cancels.
func (g *Game) tapLink(c sim.Cell) {
	p := g.planet
	switch {
	case p.BuildingAt(c) == nil:
		g.linkFrom = nil
		g.showMessage(sim.ErrNoBuilding.Error())
	case g.linkFrom == nil || *g.linkFrom == c:
		g.linkFrom = &c
		g.showMessage("Now tap a building to link to")
	default:
		from := *g.linkFrom
		cost := p.LinkCost(from, c)
		if err := p.Link(from, c); err != nil {
			g.showMessage(err.Error())
			return
		}
		g.linkFrom = &c
		g.showMessage(fmt.Sprintf("Linked (%.1f metal). Tap another to keep going", cost))
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
	g.drawLinks(screen)
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
	if c := g.highlighted(); c != nil {
		sel = []float32{float32(c.Face), float32(c.I), float32(c.J)}
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

// highlighted is the tile to highlight: the start of a link being made, or
// else the selected tile.
func (g *Game) highlighted() *sim.Cell {
	if g.tool == toolLink && g.linkFrom != nil {
		return g.linkFrom
	}
	return g.selected
}

// drawLinks draws each link as a line hugging the planet's surface, hiding
// the parts on the far side. Links in networks short of power are orange.
func (g *Game) drawLinks(screen *ebiten.Image) {
	const segments = 12
	p := g.planet
	s := float32(ebiten.Monitor().DeviceScaleFactor())
	for _, l := range p.Links() {
		a := p.Grid.DirFromCell(p.Grid.CellAt(l[0]))
		b := p.Grid.DirFromCell(p.Grid.CellAt(l[1]))
		clr := color.RGBA{90, 220, 255, 255}
		if p.NetworkOf(p.BuildingAt(p.Grid.CellAt(l[0]))).PowerSatisfaction() < 1 {
			clr = color.RGBA{255, 150, 40, 255}
		}
		px, py, pvis := g.cam.project(a)
		for i := 1; i <= segments; i++ {
			t := float64(i) / segments
			d := a.Scale(1 - t).Add(b.Scale(t)).Normalize()
			x, y, vis := g.cam.project(d)
			if vis && pvis {
				vector.StrokeLine(screen, float32(px), float32(py), float32(x), float32(y), 2*s, clr, true)
			}
			px, py, pvis = x, y, vis
		}
	}
}

func (g *Game) drawHUD(screen *ebiten.Image) {
	p := g.planet
	st := p.Stats()
	stores := p.Stores()
	msg := fmt.Sprintf("Day %d   Atmosphere %.2f kPa   Avg %.1fC   Life %.1f%%\nMetal %.1f   Ore %.1f   Ice %.0f   Water %.1f   FPS %.0f",
		p.Ticks/10, p.Atmosphere, st.AvgTemp, st.LifeCover*100, stores[sim.Metal], stores[sim.Ore], st.Ice, st.Water, ebiten.ActualFPS())
	if c := g.selected; c != nil {
		msg += "\n\n" + g.tileInfo(*c)
	}
	if g.ticks < g.msgUntil {
		msg += "\n\n> " + g.message
	}
	// The debug font is fixed-size, so draw it offscreen and scale it up to
	// stay readable on high-DPI phone screens.
	if g.hud == nil {
		g.hud = ebiten.NewImage(480, 160)
	}
	g.hud.Clear()
	ebitenutil.DebugPrint(g.hud, msg)
	s := ebiten.Monitor().DeviceScaleFactor()
	l, t, _, _ := currentSafeArea()

	// A translucent panel behind the text keeps it readable over bright
	// terrain like ice caps. The debug font is 6×16 pixels per character.
	lines := strings.Split(msg, "\n")
	widest := 0
	for _, line := range lines {
		widest = max(widest, len(line))
	}
	pw, ph := float64(6*widest+12)*s, float64(16*len(lines)+8)*s
	vector.FillRect(screen, float32(l+2*s), float32(t+4*s), float32(pw), float32(ph), color.RGBA{0, 0, 0, 150}, false)

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(l+8*s, t+8*s)
	screen.DrawImage(g.hud, op)
}

// tileInfo describes a tile and any building on it for the HUD.
func (g *Game) tileInfo(c sim.Cell) string {
	p := g.planet
	t := p.Tile(c)
	idx := p.Grid.Index(c)
	s := fmt.Sprintf("Tile %d (%d, %d)  %v  height %+.2f\nTemp %.1fC  Ice %.2f  Water %.3f  Life %.0f%%",
		c.Face, c.I, c.J, t.Kind(), t.Height, t.Temp, t.Ice, t.Water, t.Life*100)
	var extras []string
	if t.Ore > 0 {
		extras = append(extras, fmt.Sprintf("Ore deposit %.0f", t.Ore))
	}
	if t.Pollution > 0.01 {
		extras = append(extras, fmt.Sprintf("Pollution %.2f", t.Pollution))
	}
	if h := p.TotalHeat(idx); h > 0.5 {
		extras = append(extras, fmt.Sprintf("Heated +%.0fC", h))
	}
	if sim.Habitable(t) {
		extras = append(extras, "Suits microbes")
	}
	if len(extras) > 0 {
		s += "\n" + strings.Join(extras, "   ")
	}

	b := p.BuildingAt(c)
	if b == nil {
		return s
	}
	spec := b.Spec()
	s += fmt.Sprintf("\n%s: running at %.0f%%", spec.Name, b.Activity*100)
	for r := range sim.NumResources {
		if b.In[r] > 0.005 || b.Out[r] > 0.005 {
			s += fmt.Sprintf("   %v in %.1f out %.1f", sim.Resource(r), b.In[r], b.Out[r])
		}
	}
	n := p.NetworkOf(b)
	s += fmt.Sprintf("\nNetwork: %d buildings, power %.0f/%.0f", len(n.Members), n.PowerSupply, n.PowerDemand)
	if !n.HasDepot {
		s += ", not linked to the lander"
	}
	return s
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
