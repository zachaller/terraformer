package game

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/zachaller/terraformer/sim"
)

// autosaveEvery is how often the game saves while playing, in seconds.
const autosaveEvery = 30

// saveDir is where the game keeps its save file. The platform sets it (on
// Android from the UI thread, before the game starts), hence the lock.
var saveDir struct {
	sync.Mutex
	path string
}

// SetSaveDir sets the directory the game saves into. Empty disables saving.
func SetSaveDir(dir string) {
	saveDir.Lock()
	defer saveDir.Unlock()
	saveDir.path = dir
}

func savePath() string {
	saveDir.Lock()
	defer saveDir.Unlock()
	if saveDir.path == "" {
		return ""
	}
	return filepath.Join(saveDir.path, "planet.sav")
}

// savingEnabled reports whether this game should load and save. Demo and
// screenshot runs never touch the save file.
func (g *Game) savingEnabled() bool {
	o := g.opts
	return !o.NoSave && !o.DemoColony && !o.DemoHeaters && o.DemoSteps == 0 && o.ScreenshotPath == "" &&
		savePath() != ""
}

// loadSave replaces the planet with the saved one, if there is a save.
func (g *Game) loadSave() {
	path := savePath()
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		g.showMessage("Couldn't open the save: " + err.Error())
		return
	}
	defer f.Close()
	p, err := sim.Load(f)
	if err != nil {
		g.showMessage("Couldn't load the save, starting fresh: " + err.Error())
		return
	}
	g.setPlanet(p)
	g.showMessage(fmt.Sprintf("Welcome back. Day %d", p.Ticks/10))
}

// save writes the planet to the save file, via a temporary file so a crash
// mid-write can't corrupt the existing save. The caller must hold g.mu.
func (g *Game) save() error {
	if !g.savingEnabled() || !g.planet.Landed() {
		return nil
	}
	path := savePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := g.planet.Save(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	g.lastSave = g.ticks
	return os.Rename(tmp, path)
}

// SaveNow saves immediately. It's safe to call from any goroutine, e.g. when
// the app is about to go into the background.
func (g *Game) SaveNow() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.save()
}

// autosave saves every autosaveEvery seconds. The caller must hold g.mu.
func (g *Game) autosave() {
	if g.ticks-g.lastSave < autosaveEvery*ebiten.TPS() {
		return
	}
	if err := g.save(); err != nil {
		g.showMessage("Couldn't save: " + err.Error())
		g.lastSave = g.ticks // don't retry every frame
	}
}

// newPlanet abandons the current planet for a freshly generated one.
func (g *Game) newPlanet() {
	g.setPlanet(sim.NewPlanet(g.opts.GridSize, uint64(time.Now().UnixNano())))
	g.tool = toolLand
	if err := os.Remove(savePath()); err != nil && !errors.Is(err, fs.ErrNotExist) && g.savingEnabled() {
		g.showMessage("Couldn't remove the old save: " + err.Error())
		return
	}
	g.showMessage("A new planet. Tap a tile to choose your landing site")
}
