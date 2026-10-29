package sim

// Resource is a material that moves through networks and is kept in the
// colony's stores.
type Resource int

const (
	Ore Resource = iota
	Metal
	NumResources
)

func (r Resource) String() string {
	switch r {
	case Ore:
		return "Ore"
	case Metal:
		return "Metal"
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
	NumBuildingTypes
)

// BuildingSpec defines what a building type costs and does. Rates are per
// simulation step (10 steps make one in-game day) at full power.
type BuildingSpec struct {
	Name string
	Cost float64 // metal, paid from the colony stores

	PowerOut float64 // produced every step, regardless of anything else
	PowerIn  float64 // needed to run at full rate

	Inputs  Amounts // consumed from the input buffer
	Outputs Amounts // added to the output buffer
	// Extracts means the ore output is dug out of the tile's deposit.
	Extracts bool

	Heat      float64 // °C added to its own tile (half to neighbours)
	Pollution float64 // added to its own tile

	// Depot buildings connect a network to the colony stores: surplus
	// output goes into the stores, and consumers can draw from them.
	Depot bool
}

// Specs is the data for every building type, indexed by BuildingType.
var Specs = [NumBuildingTypes]BuildingSpec{
	Lander: {
		Name:     "Lander",
		PowerOut: 3,
		Depot:    true,
	},
	SolarPanel: {
		Name:     "Solar",
		Cost:     4,
		PowerOut: 2,
	},
	Mine: {
		Name:      "Mine",
		Cost:      6,
		PowerIn:   1,
		Outputs:   Amounts{Ore: 0.1},
		Extracts:  true,
		Pollution: 0.02,
	},
	Smelter: {
		Name:      "Smelter",
		Cost:      8,
		PowerIn:   2,
		Inputs:    Amounts{Ore: 0.1},
		Outputs:   Amounts{Metal: 0.05},
		Pollution: 0.05,
	},
	HeaterMachine: {
		Name:    "Heater",
		Cost:    6,
		PowerIn: 3,
		Heat:    40,
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
	// MaxLinkLength is the longest link, in tile widths.
	MaxLinkLength = 8
	// linkCostPerTile is the metal a link costs per tile width of length.
	linkCostPerTile = 0.5
	// demolishRefund is the share of a building's cost returned on demolition.
	demolishRefund = 0.5
)
