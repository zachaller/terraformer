package game

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// settings are the player's preferences, kept beside the save file.
type settings struct {
	Sound bool
	// TutorialDone is set once the first-time walkthrough is finished or
	// skipped.
	TutorialDone bool
}

func settingsPath() string {
	saveDir.Lock()
	defer saveDir.Unlock()
	if saveDir.path == "" {
		return ""
	}
	return filepath.Join(saveDir.path, "settings.json")
}

// loadSettings reads the settings, or returns the defaults.
func loadSettings() settings {
	s := settings{Sound: true}
	if path := settingsPath(); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, &s)
		}
	}
	return s
}

// saveSettings writes the settings, if there's somewhere to keep them.
func (g *Game) saveSettings() {
	path := settingsPath()
	if path == "" || !g.savingEnabled() {
		return
	}
	data, err := json.Marshal(g.settings)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, data, 0o644)
}
