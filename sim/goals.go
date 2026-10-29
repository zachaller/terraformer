package sim

import "fmt"

// Goal is one milestone in the sequence that guides the player from landing
// to a living world. Goals are completed in order.
type Goal struct {
	Name string
	Hint string // how to achieve it
	// Progress returns how far along the goal is, from 0 to 1 (done).
	Progress func(p *Planet) float64
}

// Goals is the milestone sequence.
var Goals = []Goal{
	{
		Name:     "Land",
		Hint:     "Tap a tile to choose your landing site",
		Progress: func(p *Planet) float64 { return boolProgress(p.Landed()) },
	},
	{
		Name: "Dig for ore",
		Hint: "Tool > Mine: build it on an ore deposit (gold in the Industry view), then Tool > Link it to the lander",
		Progress: func(p *Planet) float64 {
			return boolProgress(p.anyRunning(Mine, true, 0.01))
		},
	},
	{
		Name:     "Make metal",
		Hint:     "Build a Smelter and link it to the lander. It turns ore into metal",
		Progress: func(p *Planet) float64 { return p.eco.Produced[Metal] / 10 },
	},
	{
		Name: "Warm the ground",
		Hint: "Build a Heater and link it. It needs 3 power: add Solar panels to the network",
		Progress: func(p *Planet) float64 {
			return boolProgress(p.anyRunning(HeaterMachine, false, 0.99))
		},
	},
	{
		Name: "Liquid water",
		Hint: "Heaters melt the frozen ground. Basins hold the most ice",
		Progress: func(p *Planet) float64 {
			return p.Stats().Water / 0.1
		},
	},
	{
		Name: "First life",
		Hint: "Microbes need warm, damp ground with little pollution: keep heaters away from mines and smelters",
		Progress: func(p *Planet) float64 {
			return boolProgress(p.Stats().LifeCover > 0)
		},
	},
	{
		Name: "Spread life",
		Hint: "Warm more ground: grow microbes on 1% of the planet",
		Progress: func(p *Planet) float64 {
			return p.Stats().LifeCover / 0.01
		},
	},
	{
		Name: "Thicken the air",
		Hint: "Melting ice releases CO2, which warms the whole planet. Reach 2 kPa",
		Progress: func(p *Planet) float64 {
			return (p.Atmosphere - startAtmosphere) / (2 - startAtmosphere)
		},
	},
	{
		Name: "Thaw",
		Hint: "Keep melting ice. With enough CO2 the planet warms itself. Get the average temperature above 0C",
		Progress: func(p *Planet) float64 {
			return (p.Stats().AvgTemp - p.startAvgTemp) / -p.startAvgTemp
		},
	},
	{
		Name: "Living world",
		Hint: "Let microbes flourish: average life across the planet of 50%",
		Progress: func(p *Planet) float64 {
			return p.Stats().LifeMean / 0.5
		},
	},
}

func boolProgress(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// anyRunning reports whether a building of type t runs at activity of at
// least minActivity, optionally only counting networks linked to the lander.
func (p *Planet) anyRunning(t BuildingType, needDepot bool, minActivity float64) bool {
	for _, b := range p.eco.buildings {
		if b.Type == t && b.Activity >= minActivity && (!needDepot || p.NetworkOf(b).HasDepot) {
			return true
		}
	}
	return false
}

// GoalsDone is how many goals have been completed.
func (p *Planet) GoalsDone() int { return p.goalsDone }

// CurrentGoal returns the goal being worked on and its progress, or false
// once every goal is complete.
func (p *Planet) CurrentGoal() (Goal, float64, bool) {
	if p.goalsDone >= len(Goals) {
		return Goal{}, 1, false
	}
	g := Goals[p.goalsDone]
	return g, min(max(g.Progress(p), 0), 1), true
}

// checkGoals marks off goals that are now complete, in order.
func (p *Planet) checkGoals() {
	for p.goalsDone < len(Goals) && Goals[p.goalsDone].Progress(p) >= 1 {
		p.goalsDone++
	}
}

// GoalSummary describes the current goal for display, e.g. "Make metal (40%)".
func (p *Planet) GoalSummary() string {
	g, prog, ok := p.CurrentGoal()
	if !ok {
		return "All goals complete. Keep terraforming!"
	}
	if prog > 0 && prog < 1 {
		return fmt.Sprintf("%s (%.0f%%)", g.Name, prog*100)
	}
	return g.Name
}
