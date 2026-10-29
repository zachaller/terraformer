package sim

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Building is one placed building.
type Building struct {
	Type BuildingType
	Tile int // tile index; at most one building per tile

	In, Out Amounts // input and output buffers
	// Activity is how hard the building ran last step, from 0 (idle or
	// starved) to 1 (full rate).
	Activity float64

	// Timer counts steps for buildings that wait: a comet's approach.
	Timer int

	network int // index into Planet.networks
	// rings caches the tiles around it, nearest first, for its area
	// effects.
	rings [][]int
}

func (b *Building) Spec() *BuildingSpec { return &Specs[b.Type] }

// Network is a set of buildings joined by links. Buildings in a network share
// power, and resources flow freely between them.
type Network struct {
	Members     []*Building // in tile order, for determinism
	PowerSupply float64
	PowerDemand float64
	HasDepot    bool
}

// PowerSatisfaction is the share of power demand met, from 0 to 1.
func (n *Network) PowerSatisfaction() float64 {
	if n.PowerDemand == 0 {
		return 1
	}
	return min(n.PowerSupply/n.PowerDemand, 1)
}

// economy is the colony's buildings, links and stores.
type economy struct {
	Stores    Amounts
	Produced  Amounts             // total ever produced, for goals
	Emitted   Air                 // total gas ever released by machines
	buildings []*Building         // in construction order
	at        map[int]*Building   // by tile index
	links     map[[2]int]struct{} // tile index pairs, lower index first
	networks  []*Network
	dirty     bool // networks need recomputing
	// machineHeat is this step's heating from machines, per tile, and
	// seeded marks tiles near a running biolab.
	machineHeat []float64
	seeded      []bool
}

func newEconomy(cells int) economy {
	return economy{
		at:          map[int]*Building{},
		links:       map[[2]int]struct{}{},
		machineHeat: make([]float64, cells),
		seeded:      make([]bool, cells),
	}
}

// Buildings returns every building, in construction order.
func (p *Planet) Buildings() []*Building { return p.eco.buildings }

// BuildingAt returns the building on c, or nil.
func (p *Planet) BuildingAt(c Cell) *Building { return p.eco.at[p.Grid.Index(c)] }

// Links returns every link as a pair of tile indices, in a stable order.
func (p *Planet) Links() [][2]int {
	out := make([][2]int, 0, len(p.eco.links))
	for l := range p.eco.links {
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	return out
}

// NetworkOf returns the network b belongs to.
func (p *Planet) NetworkOf(b *Building) *Network {
	p.updateNetworks()
	return p.eco.networks[b.network]
}

// Produced returns the total of each resource produced so far.
func (p *Planet) Produced() Amounts { return p.eco.Produced }

// Stores returns the colony's stockpile.
func (p *Planet) Stores() Amounts { return p.eco.Stores }

// Landed reports whether the lander has touched down.
func (p *Planet) Landed() bool { return len(p.eco.buildings) > 0 }

// Errors returned when building or linking isn't possible. They're shown to
// the player, so they read as plain sentences.
var (
	ErrNotLanded     = errors.New("Land first: choose a site for the lander")
	ErrAlreadyLanded = errors.New("The lander has already touched down")
	ErrOccupied      = errors.New("There's already a building here")
	ErrNoOre         = errors.New("Mines must go on a mineral deposit")
	ErrNoIce         = errors.New("Ice drills must go on ice")
	ErrNotVolcanic   = errors.New("Geothermal plants must go on a volcanic tile")
	ErrUnique        = errors.New("You can only have one of those")
	ErrNoBuilding    = errors.New("Links connect two buildings")
	ErrSameBuilding  = errors.New("Pick a different building to link to")
	ErrAlreadyLinked = errors.New("Those buildings are already linked")
	ErrLander        = errors.New("The lander can't be demolished")
)

// Land places the lander on c and fills the stores with starting metal.
func (p *Planet) Land(c Cell) error {
	if p.Landed() {
		return ErrAlreadyLanded
	}
	p.place(Lander, p.Grid.Index(c))
	p.eco.Stores[Metal] = startingMetal
	p.checkGoals()
	return nil
}

// CanBuild reports why a building of type t can't go on c, or nil if it can.
func (p *Planet) CanBuild(t BuildingType, c Cell) error {
	idx := p.Grid.Index(c)
	spec := &Specs[t]
	switch {
	case t == Lander:
		return ErrAlreadyLanded
	case !p.Landed():
		return ErrNotLanded
	case p.eco.at[idx] != nil:
		return ErrOccupied
	case !p.Unlocked(t):
		return fmt.Errorf("%s unlocks after the goal %q", spec.Name, spec.UnlockedBy)
	case spec.Site == DepositSite && p.Tiles[idx].Ore <= 0:
		return ErrNoOre
	case spec.Site == IceSite && p.Tiles[idx].Ice < 0.05:
		return ErrNoIce
	case spec.Site == VolcanicSite && !p.Tiles[idx].Volcanic:
		return ErrNotVolcanic
	case spec.Unique && p.count(t) > 0:
		return ErrUnique
	case spec.Site == RemoteSite && p.nearestBuilding(idx) < cometClearance:
		return fmt.Errorf("Aim the comet at least %d tiles from your buildings", cometClearance)
	case p.eco.Stores[Metal] < spec.Cost:
		return fmt.Errorf("%s needs %.0f metal; the stores have %.1f", spec.Name, spec.Cost, p.eco.Stores[Metal])
	}
	return nil
}

// count is how many buildings of type t there are.
func (p *Planet) count(t BuildingType) int {
	n := 0
	for _, b := range p.eco.buildings {
		if b.Type == t {
			n++
		}
	}
	return n
}

// nearestBuilding is the distance in tiles from tile idx to the nearest
// building.
func (p *Planet) nearestBuilding(idx int) float64 {
	d := math.Inf(1)
	c := p.Grid.CellAt(idx)
	for _, b := range p.eco.buildings {
		d = min(d, p.LinkLength(c, p.Grid.CellAt(b.Tile)))
	}
	return d
}

// Build places a building of type t on c, paying its cost from the stores.
func (p *Planet) Build(t BuildingType, c Cell) (*Building, error) {
	if err := p.CanBuild(t, c); err != nil {
		return nil, err
	}
	p.eco.Stores[Metal] -= Specs[t].Cost
	return p.place(t, p.Grid.Index(c)), nil
}

func (p *Planet) place(t BuildingType, idx int) *Building {
	b := &Building{Type: t, Tile: idx}
	p.eco.buildings = append(p.eco.buildings, b)
	p.eco.at[idx] = b
	p.eco.dirty = true
	return b
}

// Demolish removes the building on c and its links, refunding part of its cost.
func (p *Planet) Demolish(c Cell) error {
	idx := p.Grid.Index(c)
	b := p.eco.at[idx]
	switch {
	case b == nil:
		return errors.New("There's nothing here to demolish")
	case b.Type == Lander:
		return ErrLander
	}
	p.eco.Stores[Metal] += b.Spec().Cost * demolishRefund
	p.remove(b)
	return nil
}

// remove takes building b and its links off the planet.
func (p *Planet) remove(b *Building) {
	idx := b.Tile
	delete(p.eco.at, idx)
	p.eco.buildings = slices.DeleteFunc(p.eco.buildings, func(x *Building) bool { return x == b })
	for l := range p.eco.links {
		if l[0] == idx || l[1] == idx {
			delete(p.eco.links, l)
		}
	}
	p.eco.dirty = true
}

// LinkLength is the distance between the centres of tiles a and b, in tile
// widths.
func (p *Planet) LinkLength(a, b Cell) float64 {
	da, db := p.Dir(a), p.Dir(b)
	angle := math.Acos(min(da.Dot(db), 1))
	return angle / p.Grid.TileAngle()
}

// LinkCost is the metal a link between a and b costs.
func (p *Planet) LinkCost(a, b Cell) float64 {
	return math.Ceil(p.LinkLength(a, b)) * linkCostPerTile
}

// CanLink reports why buildings on a and b can't be linked, or nil if they can.
func (p *Planet) CanLink(a, b Cell) error {
	ia, ib := p.Grid.Index(a), p.Grid.Index(b)
	switch {
	case p.eco.at[ia] == nil || p.eco.at[ib] == nil:
		return ErrNoBuilding
	case ia == ib:
		return ErrSameBuilding
	}
	if _, ok := p.eco.links[linkKey(ia, ib)]; ok {
		return ErrAlreadyLinked
	}
	if l, reach := p.LinkLength(a, b), max(linkReach(p.eco.at[ia]), linkReach(p.eco.at[ib])); l > reach {
		return fmt.Errorf("Too far: %.0f tiles (links reach %.0f)", l, reach)
	}
	if cost := p.LinkCost(a, b); p.eco.Stores[Metal] < cost {
		return fmt.Errorf("Link needs %.1f metal; the stores have %.1f", cost, p.eco.Stores[Metal])
	}
	return nil
}

// linkReach is how far links from b can reach, in tiles.
func linkReach(b *Building) float64 { return LinkReach(b.Type) }

// LinkReach is how far links from a building of type t can reach, in tiles.
func LinkReach(t BuildingType) float64 { return max(Specs[t].LinkReach, MaxLinkLength) }

// Covers reports whether building b connects a building on tile idx
// without a link, being a pylon close enough.
func (p *Planet) covers(b *Building, idx int) bool {
	r := b.Spec().CoverRadius
	return r > 0 && b.Tile != idx && p.LinkLength(p.Grid.CellAt(b.Tile), p.Grid.CellAt(idx)) <= r
}

// Spokes returns the connections pylons make without links, as pairs of
// tile indices (pylon first).
func (p *Planet) Spokes() [][2]int {
	var out [][2]int
	for _, py := range p.eco.buildings {
		if py.Spec().CoverRadius == 0 {
			continue
		}
		for _, b := range p.eco.buildings {
			if p.covers(py, b.Tile) {
				out = append(out, [2]int{py.Tile, b.Tile})
			}
		}
	}
	return out
}

// AutoLink is how a building of type t placed on c would join a network:
// covered means a pylon (or, for a new pylon, a building nearby) connects
// it without a link; otherwise target is the building it should link to,
// if any is in reach: the nearest in the lander's network, or failing
// that the nearest at all.
func (p *Planet) AutoLink(t BuildingType, c Cell) (target Cell, ok, covered bool) {
	p.updateNetworks()
	idx := p.Grid.Index(c)
	r := Specs[t].CoverRadius
	best, bestScore := -1, math.Inf(1)
	for _, b := range p.eco.buildings {
		if b.Tile == idx {
			continue
		}
		d := p.LinkLength(c, p.Grid.CellAt(b.Tile))
		if p.covers(b, idx) || r > 0 && d <= r {
			return Cell{}, false, true
		}
		if d > max(LinkReach(t), linkReach(b)) {
			continue
		}
		score := d
		if !p.eco.networks[b.network].HasDepot {
			score += 1000 // only if nothing in the lander's network is in reach
		}
		if score < bestScore {
			best, bestScore = b.Tile, score
		}
	}
	if best < 0 {
		return Cell{}, false, false
	}
	return p.Grid.CellAt(best), true, false
}

// Link joins the buildings on a and b, paying for it from the stores.
func (p *Planet) Link(a, b Cell) error {
	if err := p.CanLink(a, b); err != nil {
		return err
	}
	p.eco.Stores[Metal] -= p.LinkCost(a, b)
	p.eco.links[linkKey(p.Grid.Index(a), p.Grid.Index(b))] = struct{}{}
	p.eco.dirty = true
	return nil
}

func linkKey(a, b int) [2]int {
	if a > b {
		a, b = b, a
	}
	return [2]int{a, b}
}

// updateNetworks regroups buildings into networks after buildings or links
// change, using union-find over the links.
func (p *Planet) updateNetworks() {
	e := &p.eco
	if !e.dirty {
		return
	}
	e.dirty = false

	parent := map[int]int{}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	for _, b := range e.buildings {
		parent[b.Tile] = b.Tile
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[max(ra, rb)] = min(ra, rb)
		}
	}
	for _, l := range p.Links() {
		union(l[0], l[1])
	}
	for _, s := range p.Spokes() {
		union(s[0], s[1])
	}

	sorted := slices.Clone(e.buildings)
	slices.SortFunc(sorted, func(a, b *Building) int { return a.Tile - b.Tile })
	byRoot := map[int]int{}
	e.networks = e.networks[:0]
	for _, b := range sorted {
		r := find(b.Tile)
		n, ok := byRoot[r]
		if !ok {
			n = len(e.networks)
			byRoot[r] = n
			e.networks = append(e.networks, &Network{})
		}
		b.network = n
		net := e.networks[n]
		net.Members = append(net.Members, b)
		net.HasDepot = net.HasDepot || b.Spec().Depot
	}
}

// stepEconomy runs one step of every network: share power, run machines,
// then move resources between buildings and the stores.
func (p *Planet) stepEconomy() {
	p.updateNetworks()
	clear(p.eco.machineHeat)
	clear(p.eco.seeded)
	var arrived []*Building
	for _, n := range p.eco.networks {
		n.PowerSupply, n.PowerDemand = 0, 0
		for _, b := range n.Members {
			n.PowerSupply += p.powerOut(b)
			n.PowerDemand += b.Spec().PowerIn
		}
		power := n.PowerSatisfaction()
		for _, b := range n.Members {
			p.runBuilding(b, power)
			if s := b.Spec(); s.Countdown > 0 && b.Timer >= s.Countdown {
				arrived = append(arrived, b)
			}
		}
		for r := range NumResources {
			p.distribute(n, Resource(r))
		}
	}
	for _, b := range arrived {
		p.impact(b)
	}
}

// powerOut is the power b makes this step.
func (p *Planet) powerOut(b *Building) float64 {
	s := b.Spec()
	if s.Wind {
		return s.PowerOut * p.WindFactor()
	}
	return s.PowerOut
}

// Output is what b produces per step at full rate: its recipe's outputs, or
// for a mine or drill, what it digs.
func (p *Planet) Output(b *Building) Amounts {
	s := b.Spec()
	out := s.Outputs
	switch s.Site {
	case DepositSite:
		out[p.Tiles[b.Tile].Mineral.Resource()] += s.Extracts
	case IceSite:
		out[Water] += s.Extracts * iceToWater
	}
	return out
}

// Rings returns the tiles around b, ring by ring out to n (ring 0 is its
// own tile).
func (p *Planet) Rings(b *Building, n int) [][]int {
	if len(b.rings) <= n {
		b.rings = p.ringsAround(b.Tile, n)
	}
	return b.rings[:n+1]
}

// ringsAround returns the tiles around idx, ring by ring out to n.
func (p *Planet) ringsAround(idx, n int) [][]int {
	seen := map[int]bool{idx: true}
	rings := [][]int{{idx}}
	for r := 1; r <= n; r++ {
		var ring []int
		for _, f := range rings[r-1] {
			for _, nb := range p.neighbors[f] {
				if !seen[int(nb)] {
					seen[int(nb)] = true
					ring = append(ring, int(nb))
				}
			}
		}
		rings = append(rings, ring)
	}
	return rings
}

// runBuilding operates b for one step, as fast as power, inputs, output
// space and (for mines and drills) what's left to dig allow.
func (p *Planet) runBuilding(b *Building, power float64) {
	s := b.Spec()
	t := &p.Tiles[b.Tile]
	out := p.Output(b)
	f := 1.0
	if s.PowerIn > 0 {
		f = power
	}
	if s.Wind {
		f = p.WindFactor()
	}
	for r := range NumResources {
		if in := s.Inputs[r]; in > 0 {
			f = min(f, b.In[r]/in)
		}
		if o := out[r]; o > 0 {
			f = min(f, (bufferCap-b.Out[r])/o)
		}
	}
	switch s.Site {
	case DepositSite:
		f = min(f, t.Ore/s.Extracts)
	case IceSite:
		f = min(f, t.Ice/s.Extracts)
	}
	if s.Emits.CO2 < 0 {
		// A scrubber can't take more CO₂ than there is.
		f = min(f, max(p.Air.CO2-scrubberFloor, 0)/-s.Emits.CO2)
	}
	f = max(f, 0)
	b.Activity = f
	if s.Countdown > 0 {
		b.Timer++
		b.Activity = float64(b.Timer) / float64(s.Countdown)
		return
	}

	for r := range NumResources {
		b.In[r] -= s.Inputs[r] * f
		b.Out[r] += out[r] * f
		p.eco.Produced[r] += out[r] * f
	}
	switch s.Site {
	case DepositSite:
		t.Ore = max(t.Ore-s.Extracts*f, 0)
	case IceSite:
		t.Ice = max(t.Ice-s.Extracts*f, 0)
	}
	p.Air.CO2 += s.Emits.CO2 * f
	p.Air.N2 += s.Emits.N2 * f
	p.Air.O2 += s.Emits.O2 * f
	p.Air.PFC += s.Emits.PFC * f
	p.eco.Emitted.CO2 += s.Emits.CO2 * f
	p.eco.Emitted.N2 += s.Emits.N2 * f
	p.eco.Emitted.O2 += s.Emits.O2 * f
	p.eco.Emitted.PFC += s.Emits.PFC * f
	t.Pollution += s.Pollution * f
	if s.Heat > 0 {
		for r, ring := range p.Rings(b, s.HeatRings) {
			h := s.Heat * f * (1 - float64(r)/float64(s.HeatRings+1))
			for _, idx := range ring {
				p.eco.machineHeat[idx] += h
			}
		}
	}
	if s.Dust > 0 {
		for r, ring := range p.Rings(b, s.DustRings) {
			d := s.Dust * f * (1 - float64(r)/float64(s.DustRings+1))
			for _, idx := range ring {
				if tt := &p.Tiles[idx]; tt.Ice > 0.01 || tt.DryIce > 0.01 {
					tt.Dust = min(tt.Dust+d, 1)
				}
			}
		}
	}
	if s.SeedRings > 0 && f > 0.5 {
		for _, ring := range p.Rings(b, s.SeedRings) {
			for _, idx := range ring {
				p.eco.seeded[idx] = true
			}
		}
	}
}

// impact strikes the planet with the comet b: it brings water, nitrogen
// and some CO₂, blasts a crater and heats the area, and destroys any
// buildings within two rings.
func (p *Planet) impact(b *Building) {
	rings := p.ringsAround(b.Tile, 3)
	for r, ring := range rings {
		k := 1 - float64(r)/4
		for _, idx := range ring {
			t := &p.Tiles[idx]
			t.Ice += cometIce * k
			t.Temp += 80 * k
			t.Height -= 0.12 * k
			t.Life, t.Bio, t.Soil = 0, [NumSpecies]float64{}, t.Soil*(1-k)
			if r <= 2 {
				if other := p.eco.at[idx]; other != nil && other != b && other.Type != Lander {
					p.remove(other)
				}
			}
		}
	}
	p.Air.N2 += cometN2
	p.Air.CO2 += cometCO2
	p.Vapour += cometVapour
	p.impacts++
	p.remove(b)
}

// What a comet brings.
const (
	cometIce    = 2.0 // per tile at the centre, less further out
	cometN2     = 1.5 // kPa
	cometCO2    = 1.0 // kPa
	cometVapour = 40  // water depth, into the air
)

// distribute moves resource r from producers' output buffers to consumers'
// input buffers, in tile order. With a depot in the network, all output goes
// to the stores and consumers are supplied from them.
func (p *Planet) distribute(n *Network, r Resource) {
	need := func(b *Building) float64 {
		if b.Spec().Inputs[r] == 0 {
			return 0
		}
		return bufferCap - b.In[r]
	}
	if n.HasDepot {
		for _, b := range n.Members {
			p.eco.Stores[r] += b.Out[r]
			b.Out[r] = 0
		}
		for _, b := range n.Members {
			give := min(need(b), p.eco.Stores[r])
			b.In[r] += give
			p.eco.Stores[r] -= give
		}
		return
	}
	for _, consumer := range n.Members {
		for _, producer := range n.Members {
			want := need(consumer)
			if want <= 0 {
				break
			}
			give := min(want, producer.Out[r])
			producer.Out[r] -= give
			consumer.In[r] += give
		}
	}
}

// AddMetal puts metal straight into the colony's stores. It's for demos and
// screenshots.
func (p *Planet) AddMetal(m float64) { p.eco.Stores[Metal] += m }
