package sim

// Resource is a material that moves through networks and is kept in the
// colony's stores.
type Resource int

const (
	Ore Resource = iota
	Metal
	Water
	FluoriteRes
	NitrateRes
	NumResources
)

func (r Resource) String() string {
	switch r {
	case Ore:
		return "Ore"
	case Metal:
		return "Metal"
	case Water:
		return "Water"
	case FluoriteRes:
		return "Fluorite"
	case NitrateRes:
		return "Nitrate"
	}
	return "?"
}

// Amounts holds a quantity of each resource.
type Amounts [NumResources]float64

// BuildingType identifies a kind of building.
type BuildingType int

const (
	Lander BuildingType = iota
	SolarPanel
	Mine
	Smelter
	HeaterMachine
	IceDrill
	Electrolyser
	GreenhouseFactory
	NitrogenPlant
	DustSpreader
	WindTurbine
	Geothermal
	OrbitalMirror
	Comet
	MagneticShield
	Biolab
	CarbonScrubber
	Pylon
	NumBuildingTypes
)

// Category groups buildings in the build menu.
type Category int

const (
	CatPower Category = iota
	CatIndustry
	CatClimate
	CatAtmosphere
	CatProjects
	NumCategories
)

func (c Category) String() string {
	switch c {
	case CatPower:
		return "Power"
	case CatIndustry:
		return "Industry"
	case CatClimate:
		return "Climate"
	case CatAtmosphere:
		return "Air"
	case CatProjects:
		return "Projects"
	}
	return "?"
}

// Site is what a building needs from the tile it's built on.
type Site int

const (
	AnySite      Site = iota
	DepositSite       // a mineral deposit (mines)
	IceSite           // ice to drill
	VolcanicSite      // a volcanic tile
	RemoteSite        // well away from other buildings (comets)
)

// BuildingSpec defines what a building type costs and does. Rates are per
// simulation step (10 steps make one in-game day) at full power.
type BuildingSpec struct {
	Name     string
	Category Category
	Cost     float64 // metal, paid from the colony stores
	// UnlockedBy is the goal that must be complete before it can be built
	// ("" means from the start).
	UnlockedBy string
	Site       Site
	// Unique buildings can only be built once.
	Unique bool

	PowerOut float64 // produced every step, regardless of anything else
	PowerIn  float64 // needed to run at full rate
	// Wind means PowerOut scales with how thick the air is.
	Wind bool

	Inputs  Amounts // consumed from the input buffer
	Outputs Amounts // added to the output buffer
	// Extracts is how much a mine digs from its deposit, or a drill melts
	// from the ice, per step; the output is the deposit's resource (or
	// water).
	Extracts float64
	// Emits is gas released into the air (negative: taken out of it).
	Emits Air

	Heat      float64 // °C added to its own tile; see HeatRings
	HeatRings int     // rings of tiles around it that also warm, fading out
	Pollution float64 // added to its own tile

	// Dust is how much dust it spreads per step on icy tiles within
	// DustRings rings.
	Dust      float64
	DustRings int
	// SeedRings is how far a biolab helps life take hold.
	SeedRings int
	// Shield stops the air escaping to space while it runs at full power.
	Shield bool
	// Countdown is how many steps a comet takes to arrive.
	Countdown int

	// Hubs (the lander and pylons) join networks: they connect every
	// building within CoverRadius tiles, and other hubs within Reach tiles.
	Reach, CoverRadius float64

	// Depot buildings connect a network to the colony stores: surplus
	// output goes into the stores, and consumers can draw from them.
	Depot bool
}

// Specs is the data for every building type, indexed by BuildingType.
var Specs = [NumBuildingTypes]BuildingSpec{
	Lander: {
		Name:        "Lander",
		PowerOut:    3,
		Reach:       hubReach,
		CoverRadius: hubCover,
		// A trickle of metal from salvage and recycling, so a colony that's
		// spent everything can always build another mine eventually.
		Outputs: Amounts{Metal: 0.004},
		Depot:   true,
	},
	SolarPanel: {
		Name:     "Solar",
		Category: CatPower,
		Cost:     4,
		PowerOut: 2,
	},
	Pylon: {
		Name:        "Pylon",
		Category:    CatPower,
		Cost:        3,
		Reach:       hubReach,
		CoverRadius: hubCover,
	},
	WindTurbine: {
		Name:       "Wind turbine",
		Category:   CatPower,
		Cost:       10,
		UnlockedBy: "Thicken the air",
		PowerOut:   6,
		Wind:       true,
	},
	Geothermal: {
		Name:       "Geothermal",
		Category:   CatPower,
		Cost:       18,
		UnlockedBy: "First life",
		Site:       VolcanicSite,
		PowerOut:   10,
		Heat:       12,
	},
	Mine: {
		Name:      "Mine",
		Category:  CatIndustry,
		Cost:      6,
		Site:      DepositSite,
		PowerIn:   1,
		Extracts:  0.1,
		Pollution: 0.02,
	},
	Smelter: {
		Name:      "Smelter",
		Category:  CatIndustry,
		Cost:      8,
		PowerIn:   2,
		Inputs:    Amounts{Ore: 0.1},
		Outputs:   Amounts{Metal: 0.05},
		Pollution: 0.05,
	},
	IceDrill: {
		Name:       "Ice drill",
		Category:   CatIndustry,
		Cost:       10,
		UnlockedBy: "Warm the ground",
		Site:       IceSite,
		PowerIn:    2,
		Extracts:   0.001,
	},
	HeaterMachine: {
		Name:      "Heater",
		Category:  CatClimate,
		Cost:      6,
		PowerIn:   3,
		Heat:      70,
		HeatRings: 1,
	},
	DustSpreader: {
		Name:       "Dust spreader",
		Category:   CatClimate,
		Cost:       12,
		UnlockedBy: "Warm the ground",
		PowerIn:    2,
		Dust:       0.0004,
		DustRings:  3,
	},
	GreenhouseFactory: {
		Name:       "Greenhouse factory",
		Category:   CatClimate,
		Cost:       25,
		UnlockedBy: "Liquid water",
		PowerIn:    4,
		Inputs:     Amounts{FluoriteRes: 0.05},
		Emits:      Air{PFC: 0.00015},
		Pollution:  0.01,
	},
	OrbitalMirror: {
		Name:       "Orbital mirror",
		Category:   CatClimate,
		Cost:       150,
		UnlockedBy: "Thicken the air",
		Heat:       45,
		HeatRings:  4,
	},
	Electrolyser: {
		Name:       "Electrolyser",
		Category:   CatAtmosphere,
		Cost:       20,
		UnlockedBy: "Liquid water",
		PowerIn:    4,
		Inputs:     Amounts{Water: 0.1},
		Emits:      Air{O2: 0.0001},
	},
	NitrogenPlant: {
		Name:       "Nitrogen plant",
		Category:   CatAtmosphere,
		Cost:       20,
		UnlockedBy: "First moss",
		PowerIn:    3,
		Inputs:     Amounts{NitrateRes: 0.1},
		Emits:      Air{N2: 0.00015},
	},
	MagneticShield: {
		Name:       "Magnetic shield",
		Category:   CatAtmosphere,
		Cost:       400,
		UnlockedBy: "Thaw",
		Unique:     true,
		PowerIn:    20,
		Shield:     true,
	},
	CarbonScrubber: {
		Name:       "Carbon scrubber",
		Category:   CatAtmosphere,
		Cost:       35,
		UnlockedBy: "Oxygen-rich",
		PowerIn:    6,
		Emits:      Air{CO2: -0.0004},
	},
	Biolab: {
		Name:       "Biolab",
		Category:   CatProjects,
		Cost:       30,
		UnlockedBy: "First life",
		PowerIn:    3,
		SeedRings:  3,
	},
	Comet: {
		Name:       "Comet",
		Category:   CatProjects,
		Cost:       120,
		UnlockedBy: "Thaw",
		Site:       RemoteSite,
		Unique:     true,
		Countdown:  300,
	},
}

func (t BuildingType) String() string { return Specs[t].Name }

// Economy tuning.
const (
	// bufferCap is how much of each resource a building's input and output
	// buffers hold. A full output buffer stalls production.
	bufferCap = 5
	// startingMetal is in the colony stores when the lander touches down.
	startingMetal = 30
	// hubCover is how far, in tiles, hubs connect buildings around them,
	// and hubReach how far apart hubs can be and still connect.
	hubCover = 4
	hubReach = 10
	// demolishRefund is the share of a building's cost returned on demolition.
	demolishRefund = 0.5
	// iceToWater is how much water an ice drill gets from one unit of ice.
	iceToWater = 100
	// scrubberFloor is the CO₂ (kPa) carbon scrubbers leave in the air, for
	// plants to keep making oxygen from.
	scrubberFloor = 0.5
	// cometClearance is how far, in tiles, a comet must be aimed from any
	// building.
	cometClearance = 4
)

// WindFactor is how much of its rated power a wind turbine makes at the
// current pressure: nothing below 2 kPa, full power from 40 kPa.
func (p *Planet) WindFactor() float64 {
	return min(max((p.Pressure()-2)/38, 0), 1)
}

// Unlocked reports whether building type t can be built yet.
func (p *Planet) Unlocked(t BuildingType) bool {
	g := Specs[t].UnlockedBy
	return g == "" || p.goalsDone > GoalIndex(g)
}
