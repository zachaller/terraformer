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
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
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

	// NoSave stops the game loading or saving the save file. Demo and
	// screenshot runs never save either way.
	NoSave bool

	// ScreenshotPath, if set, selects the tile at the centre of the screen,
	// saves a PNG of the first frames to this path and exits. Used to check
	// rendering without a display.
	ScreenshotPath string
}

// Game implements ebiten.Game.
type Game struct {
	// mu guards the game state: Update and Draw hold it, and so does
	// SaveNow, which the platform may call from another thread.
	mu sync.Mutex

	opts   Options
	planet *sim.Planet
	shader *ebiten.Shader

	atlas      *ebiten.Image
	atlasPix   []byte
	atlasSrc   []int32 // tile shown by each atlas colour pixel
	atlasDirty bool

	cam      camera
	input    pointerInput
	selected *sim.Cell

	tool     tool
	menuOpen bool
	linkFrom *sim.Cell // first end of a link being made
	message  string
	msgKind  msgKind
	msgUntil int // tick the message disappears

	goalCollapsed bool
	metalHist     []float64 // metal produced so far, sampled each day

	// planetTarget is the screen area (x, y, w, h in pixels) the UI leaves
	// for the planet; planetArea eases towards it so the planet glides
	// rather than jumps when panels open and close.
	planetTarget, planetArea [4]float64
	// focus, if set, is a tile the camera is turning to centre.
	focus    *sim.Cell
	view     viewMode
	speed    int     // index into speeds
	stepDebt float64 // fractional simulation steps owed

	goalsSeen       int // goals completed, as last announced
	lastSave        int // tick of the last save
	confirmNewUntil int // a second "New planet" tap before this tick confirms

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
	g := &Game{
		opts:   opts,
		shader: shader,
		cam:    newCamera(),
		speed:  1,
	}
	g.setPlanet(p)
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
	g.goalsSeen = p.GoalsDone()
	return g, nil
}

// setPlanet switches the game to planet p, resetting selection and tools.
func (g *Game) setPlanet(p *sim.Planet) {
	n := p.Grid.N
	g.planet = p
	if g.atlas == nil || g.atlas.Bounds().Dx() != sim.Faces*(n+2) {
		g.atlas = ebiten.NewImage(sim.Faces*(n+2), 2*(n+2))
		g.atlasPix = make([]byte, sim.Faces*(n+2)*2*(n+2)*4)
		g.atlasSrc = atlasSources(p.Grid)
	}
	g.atlasDirty = true
	g.selected, g.linkFrom = nil, nil
	g.menuOpen = false
	g.tool = toolLand
	if p.Landed() {
		g.tool = toolInspect
	}
	g.goalsSeen = p.GoalsDone()
	g.metalHist = nil
}

func (g *Game) Update() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.done {
		return ebiten.Termination
	}
	g.ticks++
	g.runUI(g.newUI(nil))
	g.easePlanetArea()
	g.layoutCamera()
	if g.ticks == 1 && g.savingEnabled() {
		g.loadSave()
		g.lastSave = g.ticks
	}

	g.handleKeys()
	gs := g.input.update()
	g.cam.zoomBy(gs.zoom)
	switch {
	case gs.dragging && (gs.dragX != 0 || gs.dragY != 0):
		g.focus = nil
		g.cam.drag(gs.dragX, gs.dragY)
	case gs.dragging, gs.tap:
		g.cam.stop()
	default:
		g.cam.coast()
	}
	if gs.tap {
		u := g.newUI(nil)
		u.tapping, u.tapX, u.tapY = true, gs.tapX/u.s, gs.tapY/u.s
		g.runUI(u)
		switch {
		case !u.hit:
			g.tapPlanet(gs.tapX, gs.tapY)
		case u.action != nil:
			u.action()
		}
	}

	if g.focus != nil && g.cam.turnTowards(g.planet.Grid.DirFromCell(*g.focus), 0.15) {
		g.focus = nil
	}

	if g.opts.ScreenshotPath != "" && g.ticks == 2 {
		g.selectAt(g.cam.cx, g.cam.cy)
		if bs := g.planet.Buildings(); len(bs) > 0 {
			c := g.planet.Grid.CellAt(bs[len(bs)-1].Tile)
			g.selected, g.focus = &c, &c
		}
	}

	g.stepDebt += speeds[g.speed].steps / float64(ebiten.TPS())
	for g.stepDebt >= 1 {
		g.planet.Step()
		g.stepDebt--
		g.atlasDirty = true
		if g.planet.Ticks%10 == 0 {
			g.metalHist = append(g.metalHist, g.planet.Produced()[sim.Metal])
			if len(g.metalHist) > 6 {
				g.metalHist = g.metalHist[1:]
			}
		}
	}
	g.announceGoals()
	if g.savingEnabled() {
		g.autosave()
	}
	return nil
}

// announceGoals shows a message when goals are completed.
func (g *Game) announceGoals() {
	done := g.planet.GoalsDone()
	if done <= g.goalsSeen {
		return
	}
	g.goalsSeen = done
	if done == 0 {
		return
	}
	msg := "Goal complete: " + sim.Goals[done-1].Name + "!"
	if done < len(sim.Goals) {
		msg += " Next: " + sim.Goals[done].Name
	} else {
		msg += " Every goal done. The planet is alive"
	}
	g.showMessage(msg, msgGood)
}

// handleKeys provides desktop shortcuts for the on-screen buttons.
func (g *Game) handleKeys() {
	landed := g.planet.Landed()
	if landed {
		for i, t := range buildTools {
			if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
				g.setTool(t)
			}
		}
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyB) && landed:
		g.menuOpen = !g.menuOpen
	case inpututil.IsKeyJustPressed(ebiten.KeyI) && landed:
		g.setTool(toolInspect)
	case inpututil.IsKeyJustPressed(ebiten.KeyL) && landed:
		g.setTool(toolLink)
	case inpututil.IsKeyJustPressed(ebiten.KeyX) && landed:
		g.setTool(toolDemolish)
	case inpututil.IsKeyJustPressed(ebiten.KeyG):
		g.goalCollapsed = !g.goalCollapsed
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.menuOpen = false
		g.linkFrom = nil
		g.selected = nil
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

// newUI starts a UI pass that draws to dst, or only lays out if dst is nil.
func (g *Game) newUI(dst *ebiten.Image) *ui {
	return &ui{g: g, dst: dst, s: ebiten.Monitor().DeviceScaleFactor()}
}

// easePlanetArea moves the planet's screen area a step towards where the
// UI wants it.
func (g *Game) easePlanetArea() {
	if g.planetArea[2] == 0 {
		g.planetArea = g.planetTarget
		return
	}
	for i := range g.planetArea {
		g.planetArea[i] += (g.planetTarget[i] - g.planetArea[i]) * 0.2
	}
}

// layoutCamera centres the planet in the space the UI leaves for it.
func (g *Game) layoutCamera() {
	a := g.planetArea
	if a[2] == 0 {
		l, t, r, b := currentSafeArea()
		a = [4]float64{l, t, float64(g.w) - l - r, float64(g.h) - t - b}
	}
	g.cam.layoutIn(a[0], a[1], a[2], a[3])
}

func (g *Game) showMessage(m string, kind msgKind) {
	g.message = m
	g.msgKind = kind
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

	if g.tool == toolInspect {
		g.focus = &c
	}
	if bt, ok := g.tool.building(); ok {
		if _, err := p.Build(bt, c); err != nil {
			g.showMessage(err.Error(), msgWarn)
		} else {
			g.showMessage(fmt.Sprintf("Built a %v. Now Link it to your network", bt), msgInfo)
		}
		return
	}
	switch g.tool {
	case toolLand:
		if err := p.Land(c); err != nil {
			g.showMessage(err.Error(), msgWarn)
			return
		}
		g.tool = toolInspect
		g.selected = nil
		g.showMessage("Landed! Tap Build to start your colony", msgGood)
	case toolDemolish:
		if err := p.Demolish(c); err != nil {
			g.showMessage(err.Error(), msgWarn)
		} else {
			g.showMessage("Removed. Half its metal was refunded", msgInfo)
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
		g.showMessage(sim.ErrNoBuilding.Error(), msgWarn)
	case g.linkFrom == nil || *g.linkFrom == c:
		g.linkFrom = &c
		g.showMessage("Now tap a building to link it to", msgInfo)
	default:
		from := *g.linkFrom
		cost := p.LinkCost(from, c)
		if err := p.Link(from, c); err != nil {
			g.showMessage(err.Error(), msgWarn)
			return
		}
		g.linkFrom = &c
		g.showMessage(fmt.Sprintf("Linked for %.1f metal. Tap another building to keep going", cost), msgGood)
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
	g.mu.Lock()
	defer g.mu.Unlock()
	g.layoutCamera()
	if g.atlasDirty {
		fillAtlas(g.atlasPix, g.atlasSrc, g.planet, g.view)
		g.atlas.WritePixels(g.atlasPix)
		g.atlasDirty = false
	}
	g.drawPlanet(screen)
	g.drawLinks(screen)
	g.runUI(g.newUI(screen))

	if g.opts.ScreenshotPath != "" && g.ticks >= 60 && !g.done {
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
	gridAt := []float32{0, 0, 0}
	if c := g.highlighted(); c != nil {
		gridAt = vec3f(g.planet.Grid.DirFromCell(*c))
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
		"Grid":     float32(g.gridAmount()),
		"GridAt":   gridAt,
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

// gridAmount is how strongly to draw tile grid lines: only while a tool that
// places or targets individual tiles is in use, and there's a tapped tile to
// centre them on.
func (g *Game) gridAmount() float64 {
	if g.tool == toolInspect || g.tool == toolLand || g.highlighted() == nil {
		return 0
	}
	return 1
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
