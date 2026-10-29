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

// Goals is the milestone sequence: from landing, through thickening the air
// and first life, to a living world with air people could breathe.
var Goals = []Goal{
	{
		Name:     "Land",
		Hint:     "Tap a tile to choose your landing site",
		Progress: func(p *Planet) float64 { return boolProgress(p.Landed()) },
	},
	{
		Name: "Dig for ore",
		Hint: "Build > Industry > Mine: build it on iron ore (gold in the Industry view) within 4 tiles of the lander, which connects it (or put a Pylon near it)",
		Progress: func(p *Planet) float64 {
			return boolProgress(p.anyRunning(Mine, true, 0.01))
		},
	},
	{
		Name:     "Make metal",
		Hint:     "Build a Smelter near the lander. It turns ore into metal",
		Progress: func(p *Planet) float64 { return p.eco.Produced[Metal] / 10 },
	},
	{
		Name: "Warm the ground",
		Hint: "Build a Heater. It needs 3 power: add Solar panels to its network. Pylons connect everything within 4 tiles",
		Progress: func(p *Planet) float64 {
			return boolProgress(p.anyRunning(HeaterMachine, false, 0.99))
		},
	},
	{
		Name: "Above the triple point",
		Hint: "Air this thin can't hold liquid water: warmed ice just turns to vapour. Heat the ground to drive out the CO2 frozen in it, or dust the polar caps (Dust spreader) so their dry ice turns to gas. Reach 0.62 kPa",
		Progress: func(p *Planet) float64 {
			return (p.Pressure() - startAir.Pressure()) / (TriplePoint - startAir.Pressure())
		},
	},
	{
		Name: "Liquid water",
		Hint: "Now heaters melt ice into water. Basins hold the most ice",
		Progress: func(p *Planet) float64 {
			return p.Stats().Water / 0.5
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
		Hint: "Warm more ground: grow microbes on 1% of the planet. A Biolab helps life take hold nearby",
		Progress: func(p *Planet) float64 {
			return p.Stats().LifeCover / 0.01
		},
	},
	{
		Name: "Super greenhouse",
		Hint: "Mine fluorite (purple in the Industry view) and build a Greenhouse factory: its gases trap thousands of times more heat than CO2. Reach 0.3 Pa",
		Progress: func(p *Planet) float64 {
			return p.Air.PFC / 0.3
		},
	},
	{
		Name: "Thicken the air",
		Hint: "Warmth drives CO2 out of the ground and the caps, which warms the planet further. Reach 5 kPa",
		Progress: func(p *Planet) float64 {
			return (p.Pressure() - TriplePoint) / (5 - TriplePoint)
		},
	},
	{
		Name: "First moss",
		Hint: "Where microbes are established and the air passes 1 kPa, moss can take root. Inspect a tile to see what it needs",
		Progress: func(p *Planet) float64 {
			return boolProgress(p.SpeciesCover(Moss) > 0)
		},
	},
	{
		Name: "Release the caps",
		Hint: "Half the polar dry ice still holds its CO2. Dust spreaders and Orbital mirrors on the caps set it free",
		Progress: func(p *Planet) float64 {
			return (p.startDryIce - p.Stats().DryIce) / (0.5 * p.startDryIce)
		},
	},
	{
		Name: "Thaw",
		Hint: "Keep warming: CO2, greenhouse factories and mirrors. Get the average temperature above 0C",
		Progress: func(p *Planet) float64 {
			return (p.Stats().AvgTemp - p.startAvgTemp) / -p.startAvgTemp
		},
	},
	{
		Name: "A breath of oxygen",
		Hint: "Oxygen forms an ozone layer that shields land plants from ultraviolet light. Electrolysers split water (from Ice drills) into it; plankton and moss make it too. Reach 0.6 kPa",
		Progress: func(p *Planet) float64 {
			return p.Air.O2 / 0.6
		},
	},
	{
		Name: "Seas",
		Hint: "Melt enough ice that standing water covers 5% of the planet",
		Progress: func(p *Planet) float64 {
			return p.Stats().SeaCover / 0.05
		},
	},
	{
		Name: "Nitrogen",
		Hint: "Plants need nitrogen. Mine nitrates (pale green in the Industry view) for a Nitrogen plant, or aim a Comet. Reach 3 kPa",
		Progress: func(p *Planet) float64 {
			return p.Air.N2 / 3
		},
	},
	{
		Name: "Grasslands",
		Hint: "Grass grows from moss once there's soil, nitrogen and an ozone layer. Grow grass on 5% of the planet",
		Progress: func(p *Planet) float64 {
			return p.SpeciesCover(Grass) / 0.05
		},
	},
	{
		Name: "Shield the air",
		Hint: "Without a magnetic field the solar wind strips the air away. Build a Magnetic shield and give it full power",
		Progress: func(p *Planet) float64 {
			return boolProgress(p.MagneticShield())
		},
	},
	{
		Name: "Life in the seas",
		Hint: "Plankton fills warm standing water; fish follow once there's 4 kPa of oxygen. Get fish into 1% of the planet",
		Progress: func(p *Planet) float64 {
			return p.SpeciesCover(Fish) / 0.01
		},
	},
	{
		Name: "Forests",
		Hint: "Shrubs grow from grass, then forests from shrubs, as soil deepens and the air thickens. Grow forest on 5% of the planet",
		Progress: func(p *Planet) float64 {
			return p.SpeciesCover(Forest) / 0.05
		},
	},
	{
		Name: "Oxygen-rich",
		Hint: "Plants turn CO2 into oxygen. Spread them, and keep the planet warm with greenhouse gases as CO2 falls. Reach 10 kPa of oxygen",
		Progress: func(p *Planet) float64 {
			return p.Air.O2 / 10
		},
	},
	{
		Name: "Grazers",
		Hint: "Animals graze the grasslands once there's 10 kPa of oxygen. Get them onto 1% of the planet",
		Progress: func(p *Planet) float64 {
			return p.SpeciesCover(Grazers) / 0.01
		},
	},
	{
		Name: "Living world",
		Hint: "Let life flourish: average microbial life across the planet of 60%",
		Progress: func(p *Planet) float64 {
			return p.Stats().LifeMean / 0.6
		},
	},
	{
		Name: "Breathable air",
		Hint: "At least 16 kPa of oxygen, at most 1 kPa of CO2 and 25 kPa in all, with the average above 5C. Plants, seas and Carbon scrubbers draw down CO2; greenhouse factories keep it warm",
		Progress: func(p *Planet) float64 {
			return min(p.Air.O2/16, (12-p.Air.CO2)/11, p.Pressure()/25, (p.Stats().AvgTemp-p.startAvgTemp)/(5-p.startAvgTemp))
		},
	},
}

// GoalIndex is the position of the goal called name in Goals, or -1.
func GoalIndex(name string) int {
	for i, g := range Goals {
		if g.Name == name {
			return i
		}
	}
	return -1
}

func boolProgress(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// anyRunning reports whether a building of type t runs at activity of at
// least minActivity, optionally only counting networks that include the
// lander.
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

// UnlockAll marks every goal done, so every building can be built. It's for
// demos and screenshots.
func (p *Planet) UnlockAll() { p.goalsDone = len(Goals) }
