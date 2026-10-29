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
const demoHeat = 40

// messageTime is how long a message stays on screen, in seconds.
const messageTime = 4

// Options configure a new Game.
type Options struct {
	Seed     uint64
	GridSize int // hex grid size (sim.Grid.N)

	// DemoHeaters puts fixed warmth on a ring of tiles around the equator.
	DemoHeaters bool
	// DemoColony lands and builds a small working colony facing the camera.
	DemoColony bool
	// DemoShowcase, with DemoColony, also builds one of every building.
	DemoShowcase bool
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

	atlas    *ebiten.Image
	atlasPix []byte
	atlasSrc []int32 // tile shown by each atlas colour pixel
	// heatRings is how much a heater warms its tile and each ring around
	// it, for placement previews.
	heatRings  []float64
	atlasDirty bool

	cam      camera
	input    pointerInput
	selected *sim.Cell

	tool     tool
	menuOpen bool
	message  string
	msgKind  msgKind
	msgUntil int // tick the message disappears
	// msgAction, if set, is a button on the message, labelled msgActionLabel.
	msgAction      func()
	msgActionLabel string

	pending *sim.Cell // where the building being placed would go

	goalCollapsed bool
	goalsOpen     bool         // the goals sheet is showing
	airOpen       bool         // the atmosphere sheet is showing
	buildTab      sim.Category // the build menu's category
	metalHist     []float64    // metal produced so far, sampled each day

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
		placeDemoColony(p, g.cam.rot[2], opts.DemoShowcase)
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
	g.planet = p
	g.heatRings = p.Grid.HeatRings(sim.Specs[sim.HeaterMachine].Heat, 3)
	if w, h := atlasSize(p.Grid); g.atlas == nil || g.atlas.Bounds().Dx() != w {
		g.atlas = ebiten.NewImage(w, h)
		g.atlasPix = make([]byte, w*h*4)
		g.atlasSrc = atlasSources(p.Grid)
	}
	g.atlasDirty = true
	g.selected = nil
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

	if g.focus != nil && g.cam.turnTowards(g.planet.Dir(*g.focus), 0.15) {
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

// handleKeys provides desktop shortcuts for the on-screen buttons.
func (g *Game) handleKeys() {
	landed := g.planet.Landed()
	if landed {
		for i, bt := range []sim.BuildingType{sim.SolarPanel, sim.Mine, sim.Smelter, sim.HeaterMachine} {
			if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
				g.setTool(buildTool(bt))
			}
		}
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyB) && landed:
		g.menuOpen = !g.menuOpen
	case inpututil.IsKeyJustPressed(ebiten.KeyI) && landed:
		g.setTool(toolInspect)
	case inpututil.IsKeyJustPressed(ebiten.KeyX) && landed:
		g.setTool(toolDemolish)
	case inpututil.IsKeyJustPressed(ebiten.KeyG):
		g.goalCollapsed = !g.goalCollapsed
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter):
		g.confirmPlacement()
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.menuOpen = false
		g.selected = nil
		g.pending = nil
		g.goalsOpen = false
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
	g.msgAction, g.msgActionLabel = nil, ""
}

// showMessageAction shows a message with a button on it, for a little longer.
func (g *Game) showMessageAction(m string, kind msgKind, label string, action func()) {
	g.showMessage(m, kind)
	g.msgAction, g.msgActionLabel = action, label
	g.msgUntil = g.ticks + 2*messageTime*ebiten.TPS()
}

// uiHitAt reports whether screen point (x, y) is on the UI rather than the
// planet.
func (g *Game) uiHitAt(x, y float64) bool {
	u := g.newUI(nil)
	u.tapping, u.tapX, u.tapY = true, x/u.s, y/u.s
	g.runUI(u)
	return u.hit
}

// tapPlanet selects the tapped tile and applies the current tool to it.
func (g *Game) tapPlanet(x, y float64) {
	g.menuOpen = false
	g.selectAt(x, y)
	if g.selected == nil {
		return
	}
	c := *g.selected
	p := g.planet
	g.atlasDirty = true

	if g.tool == toolInspect {
		g.focus = &c
	}
	if _, ok := g.placing(); ok {
		g.pending = &c
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

	// Tiles are passed to the shader by index; -1 means none.
	grid := g.planet.Grid
	sel := float32(-1)
	if c := g.highlighted(); c != nil && g.pending == nil {
		sel = float32(grid.Index(*c))
	}
	ghost, ghostType, ghostOK, tint := float32(-1), float32(0), float32(1), []float32{0, 0, 0}
	area, areaAmt := make([]float32, maxAreaCells), make([]float32, maxAreaCells)
	for i := range area {
		area[i] = -1
	}
	if bt, ok := g.placing(); ok && g.pending != nil {
		c := *g.pending
		ghost = float32(grid.Index(c))
		ghostType = float32(bt) + 1
		if g.planet.CanBuild(bt, c) != nil {
			ghostOK = 0
		}
		tints, col := g.placementArea(bt, c)
		for i, a := range tints {
			area[i], areaAmt[i] = float32(grid.Index(a.cell)), float32(a.amount)
		}
		tint = []float32{float32(col.R) / 255, float32(col.G) / 255, float32(col.B) / 255}
	}
	gridAt := []float32{0, 0, 0}
	if c := g.highlighted(); c != nil {
		gridAt = vec3f(g.planet.Dir(*c))
	}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = g.atlas
	op.Uniforms = map[string]any{
		"Center":    []float32{float32(g.cam.cx), float32(g.cam.cy)},
		"Radius":    float32(g.cam.radius),
		"Row0":      vec3f(g.cam.rot[0]),
		"Row1":      vec3f(g.cam.rot[1]),
		"Row2":      vec3f(g.cam.rot[2]),
		"N":         float32(grid.N),
		"TileAngle": float32(grid.TileAngle()),
		"Selected":  sel,
		"Time":      float32(g.ticks) / float32(ebiten.TPS()),
		"Light":     vec3f(lightDir),
		"Grid":      float32(g.gridAmount()),
		"GridAt":    gridAt,
		"Ghost":     ghost,
		"GhostType": ghostType,
		"GhostOK":   ghostOK,
		"Area":      area,
		"AreaAmt":   areaAmt,
		"AreaTint":  tint,
	}
	screen.DrawTrianglesShader(vs, []uint16{0, 1, 2, 1, 2, 3}, g.shader, op)
}

// highlighted is the tile to highlight: the selected one.
func (g *Game) highlighted() *sim.Cell { return g.selected }

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
	p := g.planet
	s := float32(ebiten.Monitor().DeviceScaleFactor())
	// Connections stay in the background, thin and faint, except while
	// placing a building, in the Industry view, or for the network of the
	// building being inspected.
	_, placing := g.placing()
	busy := placing || g.view == viewIndustry
	var focus *sim.Network
	if g.inspectOpen() {
		if b := p.BuildingAt(*g.selected); b != nil {
			focus = p.NetworkOf(b)
		}
	}
	style := func(tile int) (color.RGBA, float32, bool) {
		n := p.NetworkOf(p.BuildingAt(p.Grid.CellAt(tile)))
		c := color.RGBA{90, 220, 255, 255}
		if n.PowerSatisfaction() < 1 {
			c = color.RGBA{255, 150, 40, 255}
		}
		if busy || n == focus {
			return c, 2 * s, true
		}
		return withAlpha(c, 90), 1.2 * s, false
	}
	// Between hubs: always shown, as the network's backbone.
	for _, l := range p.HubLinks() {
		c, w, _ := style(l[0])
		g.drawArc(screen, p.Dir(p.Grid.CellAt(l[0])), p.Dir(p.Grid.CellAt(l[1])), w, c, false)
	}
	// From hubs to the buildings they connect: only while they matter.
	for _, sp := range p.Spokes() {
		if c, _, strong := style(sp[0]); strong {
			g.drawArc(screen, p.Dir(p.Grid.CellAt(sp[0])), p.Dir(p.Grid.CellAt(sp[1])), 1.2*s, withAlpha(c, 110), false)
		}
	}
	// What a building being placed would connect through.
	if bt, ok := g.placing(); ok && g.pending != nil && p.CanBuild(bt, *g.pending) == nil {
		if via, ok, _ := p.Connection(bt, *g.pending); ok {
			g.drawArc(screen, p.Dir(*g.pending), p.Dir(via), 2*s, color.RGBA{90, 220, 255, 200}, true)
		}
	}
}

// drawArc draws a line along the planet's surface from a to b, dashed if
// asked.
func (g *Game) drawArc(screen *ebiten.Image, a, b sim.Vec3, width float32, clr color.Color, dashed bool) {
	const segments = 12
	px, py, pvis := g.cam.project(a)
	for i := 1; i <= segments; i++ {
		t := float64(i) / segments
		d := a.Scale(1 - t).Add(b.Scale(t)).Normalize()
		x, y, vis := g.cam.project(d)
		if vis && pvis && (!dashed || i%2 == 1) {
			vector.StrokeLine(screen, float32(px), float32(py), float32(x), float32(y), width, clr, true)
		}
		px, py, pvis = x, y, vis
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
