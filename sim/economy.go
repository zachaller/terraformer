package sim

import (
	"cmp"
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

// Network is a set of buildings connected through hubs. Buildings in a network share
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

// economy is the colony's buildings and stores.
type economy struct {
	Stores    Amounts
	Produced  Amounts           // total ever produced, for goals
	Emitted   Air               // total gas ever released by machines
	buildings []*Building       // in construction order
	at        map[int]*Building // by tile index
	// hubLinks and spokes are the connections hubs make (see HubLinks).
	hubLinks, spokes [][2]int
	// hubLoad is the load each hub (by tile) carries, and full marks
	// buildings in reach of hubs that have no room for them.
	hubLoad  map[int]float64
	full     map[int]bool
	networks []*Network
	dirty    bool // networks need recomputing
	// machineHeat is this step's heating from machines, per tile, and
	// seeded marks tiles near a running biolab.
	machineHeat []float64
	seeded      []bool
}

func newEconomy(cells int) economy {
	return economy{
		at:          map[int]*Building{},
		machineHeat: make([]float64, cells),
		seeded:      make([]bool, cells),
	}
}

// Buildings returns every building, in construction order.
func (p *Planet) Buildings() []*Building { return p.eco.buildings }

// BuildingAt returns the building on c, or nil.
func (p *Planet) BuildingAt(c Cell) *Building { return p.eco.at[p.Grid.Index(c)] }

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
	ErrNoIce         = errors.New("Ice drills must go on ice, or on a sea or lake to pump its water")
	ErrNotVolcanic   = errors.New("Geothermal plants must go on a volcanic tile")
	ErrUnique        = errors.New("You can only have one of those")
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
	case spec.Site == IceSite && p.Tiles[idx].Ice < 0.05 && p.Tiles[idx].Water < standingWater:
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
		d = min(d, p.Distance(c, p.Grid.CellAt(b.Tile)))
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

// Demolish removes the building on c, refunding part of its cost.
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

// remove takes building b off the planet.
func (p *Planet) remove(b *Building) {
	delete(p.eco.at, b.Tile)
	p.eco.buildings = slices.DeleteFunc(p.eco.buildings, func(x *Building) bool { return x == b })
	p.eco.dirty = true
}

// Distance is the distance between the centres of tiles a and b, in tile
// widths.
func (p *Planet) Distance(a, b Cell) float64 {
	da, db := p.Dir(a), p.Dir(b)
	angle := math.Acos(min(da.Dot(db), 1))
	return angle / p.Grid.TileAngle()
}

// Buildings join networks without links: hubs (the lander and pylons)
// connect buildings within their CoverRadius, and each other within their
// Reach. Each building a hub connects adds to its load, by how far away it
// is (see connectionLoad), up to the hub's Capacity, so dense colonies
// need more pylons.

// isHub reports whether buildings of type t are network hubs.
func isHub(t BuildingType) bool { return Specs[t].Reach > 0 }

// connectionLoad is how much of a hub's capacity connecting a building d
// tiles away takes.
func connectionLoad(d float64) float64 { return max(d, 1) }

// joins reports whether hub h reaches a building of type t on tile idx (a
// hub, or any building within its cover, capacity aside).
func (p *Planet) joins(h *Building, t BuildingType, idx int) bool {
	if h.Tile == idx {
		return false
	}
	d := p.Distance(p.Grid.CellAt(h.Tile), p.Grid.CellAt(idx))
	if isHub(t) {
		return d <= max(h.Spec().Reach, Specs[t].Reach)
	}
	return d <= h.Spec().CoverRadius
}

// HubLinks returns the connections between hubs, and Spokes those from
// hubs to the other buildings they connect, as pairs of tile indices (hub
// first).
func (p *Planet) HubLinks() [][2]int { p.updateNetworks(); return p.eco.hubLinks }
func (p *Planet) Spokes() [][2]int   { p.updateNetworks(); return p.eco.spokes }

// HubLoad returns the load hub b carries and its capacity.
func (p *Planet) HubLoad(b *Building) (load, capacity float64) {
	p.updateNetworks()
	return p.eco.hubLoad[b.Tile], b.Spec().Capacity
}

// HubsFull reports whether b is in reach of hubs that are all too loaded
// to connect it.
func (p *Planet) HubsFull(b *Building) bool { p.updateNetworks(); return p.eco.full[b.Tile] }

// Connection is how a building of type t placed on c would join a network:
// via is the hub that would connect it, and colony whether that hub's
// network includes the lander. ok is false when nothing would; full then
// says whether that's because the hubs in reach have no room.
func (p *Planet) Connection(t BuildingType, c Cell) (via Cell, ok, colony, full bool) {
	p.updateNetworks()
	idx := p.Grid.Index(c)
	best, bestScore := -1, math.Inf(1)
	for _, h := range p.eco.buildings {
		if !isHub(h.Type) || !p.joins(h, t, idx) {
			continue
		}
		d := p.Distance(c, p.Grid.CellAt(h.Tile))
		if !isHub(t) && p.eco.hubLoad[h.Tile]+connectionLoad(d) > h.Spec().Capacity+1e-9 {
			full = true
			continue
		}
		score := d
		if !p.eco.networks[h.network].HasDepot {
			score += 1000 // prefer the colony
		}
		if score < bestScore {
			best, bestScore = h.Tile, score
		}
	}
	if best < 0 {
		return Cell{}, false, false, full
	}
	return p.Grid.CellAt(best), true, p.eco.networks[p.eco.at[best].network].HasDepot, false
}

// updateNetworks regroups buildings into networks after buildings change,
// using union-find over the hubs' connections.
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
	e.hubLinks, e.spokes = e.hubLinks[:0], e.spokes[:0]
	e.hubLoad, e.full = map[int]float64{}, map[int]bool{}
	// Hubs connect each other freely.
	type pair struct {
		d       float64
		hub, to *Building
	}
	var pairs []pair
	for _, h := range e.buildings {
		if !isHub(h.Type) {
			continue
		}
		for _, b := range e.buildings {
			if !p.joins(h, b.Type, b.Tile) {
				continue
			}
			if isHub(b.Type) {
				if h.Tile < b.Tile {
					e.hubLinks = append(e.hubLinks, [2]int{h.Tile, b.Tile})
					union(h.Tile, b.Tile)
				}
				continue
			}
			pairs = append(pairs, pair{p.Distance(p.Grid.CellAt(h.Tile), p.Grid.CellAt(b.Tile)), h, b})
		}
	}
	// Other buildings: the shortest connections first, each building to
	// one hub, while the hub has room.
	slices.SortFunc(pairs, func(a, b pair) int {
		switch {
		case a.d != b.d:
			return cmp.Compare(a.d, b.d)
		case a.to.Tile != b.to.Tile:
			return a.to.Tile - b.to.Tile
		}
		return a.hub.Tile - b.hub.Tile
	})
	connected := map[int]bool{}
	for _, pr := range pairs {
		if connected[pr.to.Tile] {
			continue
		}
		load := connectionLoad(pr.d)
		if e.hubLoad[pr.hub.Tile]+load > pr.hub.Spec().Capacity+1e-9 {
			e.full[pr.to.Tile] = true
			continue
		}
		e.hubLoad[pr.hub.Tile] += load
		connected[pr.to.Tile] = true
		delete(e.full, pr.to.Tile)
		e.spokes = append(e.spokes, [2]int{pr.hub.Tile, pr.to.Tile})
		union(pr.hub.Tile, pr.to.Tile)
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

// DrillWater is how much an ice drill on tile t can still take, as a depth:
// its ice, and once that's gone, any standing water above what the ground
// holds as moisture, so drills keep working after the ice melts into seas.
func DrillWater(t Tile) float64 {
	return t.Ice + max(t.Water-soilRetention, 0)
}

// mineSource is the tile mine b digs: its own while it has ore, then the
// deposit tiles next to it of the same mineral with no building on them, so
// one mine works a whole small patch. -1 means the patch is dug out.
func (p *Planet) mineSource(b *Building) int {
	if p.Tiles[b.Tile].Ore > 0 {
		return b.Tile
	}
	m := p.Tiles[b.Tile].Mineral
	for _, i := range p.Rings(b, 1)[1] {
		if t := &p.Tiles[i]; t.Ore > 0 && t.Mineral == m && p.eco.at[i] == nil {
			return i
		}
	}
	return -1
}

// PatchOre is how much ore a mine on c has (or would have) to dig: the
// deposit on c and the deposits of the same mineral next to it with no
// building on them. c should be a deposit or a mine's tile.
func (p *Planet) PatchOre(c Cell) float64 {
	idx := p.Grid.Index(c)
	t := p.Tiles[idx]
	total := t.Ore
	for _, i := range p.ringsAround(idx, 1)[1] {
		if n := p.Tiles[i]; n.Ore > 0 && n.Mineral == t.Mineral && p.eco.at[i] == nil {
			total += n.Ore
		}
	}
	return total
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
	src := -1
	switch s.Site {
	case DepositSite:
		ore := 0.0
		if src = p.mineSource(b); src >= 0 {
			ore = p.Tiles[src].Ore
		}
		f = min(f, ore/s.Extracts)
	case IceSite:
		f = min(f, DrillWater(*t)/s.Extracts)
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
		if src >= 0 {
			p.Tiles[src].Ore = max(p.Tiles[src].Ore-s.Extracts*f, 0)
		}
	case IceSite:
		// Ice first; once it's gone, pump the standing water.
		dug := s.Extracts * f
		fromIce := min(dug, t.Ice)
		t.Ice -= fromIce
		t.Water = max(t.Water-(dug-fromIce), 0)
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

// Impacts is how many comets have struck.
func (p *Planet) Impacts() int { return p.impacts }
