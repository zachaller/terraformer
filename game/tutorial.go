package game

import (
	"image/color"
	"math"

	"github.com/zachaller/terraformer/sim"
)

// The first-time walkthrough: a few cards over the game that show the
// basics. Some wait for the player to do something; the rest have a Next
// button. Skip ends it, and it's only shown once.

type tutorialStep struct {
	title, text string
	// done reports whether the player has done what the step asks; nil
	// means the step waits for Next.
	done func(p *sim.Planet) bool
	// highlight names a dock button to pulse: "build" or "colony".
	highlight string
}

func hasBuilding(t sim.BuildingType) func(p *sim.Planet) bool {
	return func(p *sim.Planet) bool {
		for _, b := range p.Buildings() {
			if b.Type == t {
				return true
			}
		}
		return false
	}
}

var tutorialSteps = []tutorialStep{
	{
		title: "Welcome to your planet",
		text:  "It's frozen, airless and dead. Your job is to make it live. Drag to turn it, and pinch to zoom.",
	},
	{
		title: "Land",
		text:  "Tap a tile to land. Pick somewhere near iron ore: the factory button on the right shows deposits, iron in gold.",
		done:  func(p *sim.Planet) bool { return p.Landed() },
	},
	{
		title:     "Dig for ore",
		text:      "Open Build, then Industry, and put a Mine on iron ore within 4 tiles of the lander. The lander powers and connects buildings near it.",
		done:      hasBuilding(sim.Mine),
		highlight: "build",
	},
	{
		title:     "Make metal",
		text:      "Build a Smelter near the lander. It turns ore into metal, which pays for everything else.",
		done:      hasBuilding(sim.Smelter),
		highlight: "build",
	},
	{
		title: "Spreading out",
		text:  "Buildings more than 4 tiles from the lander need a Pylon (Build, Power) nearby. Pylons join up to 12 tiles apart, and each can carry only so many buildings.",
	},
	{
		title:     "Finding your way",
		text:      "The goal at the top always says what to do next. Colony shows your resources and buildings, and tapping the kPa chip shows the air. Good luck!",
		highlight: "colony",
	},
}

// tutorialFor is the step to start the walkthrough at, or -1 if it's done.
// A player who's already well into a game starts at the later steps.
func tutorialFor(g *Game) int {
	if g.settings.TutorialDone {
		return -1
	}
	if g.planet.GoalsDone() > 3 {
		return 4
	}
	return 0
}

// advanceTutorial moves past steps the player has done.
func (g *Game) advanceTutorial() {
	for g.tutorial >= 0 && g.tutorial < len(tutorialSteps) {
		s := tutorialSteps[g.tutorial]
		if s.done == nil || !s.done(g.planet) {
			return
		}
		g.tutorial++
		g.sound(sndGoal)
	}
	if g.tutorial >= len(tutorialSteps) {
		g.endTutorial()
	}
}

func (g *Game) endTutorial() {
	g.tutorial = -1
	g.settings.TutorialDone = true
	g.saveSettings()
}

// tutorialHighlight is the dock button the walkthrough points at, if any.
func (g *Game) tutorialHighlight() string {
	if g.tutorial < 0 || g.tutorial >= len(tutorialSteps) {
		return ""
	}
	return tutorialSteps[g.tutorial].highlight
}

// tutorialCard draws the walkthrough's card ending at bottom, and returns
// its top (bottom if there's nothing to show).
func (g *Game) tutorialCard(u *ui, left, right, bottom float64) float64 {
	if g.tutorial < 0 || g.tutorial >= len(tutorialSteps) {
		return bottom
	}
	s := tutorialSteps[g.tutorial]
	w := right - left
	inner := w - 28
	was := u.measuring
	u.measuring = true
	textH := u.paragraph(s.text, 0, 0, inner, 13, colBody)
	u.measuring = was
	h := 12 + u.lineH(sans, 15, semibold) + 6 + textH + 10 + 32 + 12
	y := bottom - h
	u.panel(left, y, w, h, 16, withAlpha(colSheet, 245), colTeal)
	x := left + 14
	cy := y + 12
	step := tutorialStepLabel(g.tutorial)
	u.text(s.title, x, cy, sans, 15, semibold, colText)
	u.textRight(step, x+inner, cy+2, mono, 11, semibold, colMuted)
	cy += u.lineH(sans, 15, semibold) + 6
	cy += u.paragraph(s.text, x, cy, inner, 13, colBody) + 10

	// Skip, and Next (or a note that it's waiting for the player).
	u.text("Skip", x, cy+(32-u.lineH(sans, 13, semibold))/2, sans, 13, semibold, colMuted)
	u.area(x-8, cy-4, 60, 40, g.endTutorial)
	if s.done == nil {
		label := "Next"
		if g.tutorial == len(tutorialSteps)-1 {
			label = "Done"
		}
		bw := u.textW(label, sans, 13, bold) + 32
		bx := x + inner - bw
		u.fillRound(bx, cy, bw, 32, 10, colTeal)
		u.text(label, bx+16, cy+(32-u.lineH(sans, 13, bold))/2, sans, 13, bold, rgba(0x06231E))
		u.area(bx, cy-4, bw, 40, func() {
			g.tutorial++
			if g.tutorial >= len(tutorialSteps) {
				g.endTutorial()
			}
		})
	} else {
		u.textRight("Waiting for you…", x+inner, cy+(32-u.lineH(sans, 12, regular))/2, sans, 12, regular, colTealTx)
	}
	return y
}

func tutorialStepLabel(i int) string {
	return string(rune('1'+i)) + "/" + string(rune('0'+len(tutorialSteps)))
}

// pulse is a highlight colour that breathes, for pointing at things.
func (g *Game) pulse() color.Color {
	a := 0.5 + 0.5*math.Sin(float64(g.ticks)/8)
	return withAlpha(colTeal, uint8(120+135*a))
}
