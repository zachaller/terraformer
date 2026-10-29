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

	network int // index into Planet.networks
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
	return math.Min(n.PowerSupply/n.PowerDemand, 1)
}

// economy is the colony's buildings, links and stores.
type economy struct {
	Stores    Amounts
	buildings []*Building         // in construction order
	at        map[int]*Building   // by tile index
	links     map[[2]int]struct{} // tile index pairs, lower index first
	networks  []*Network
	dirty     bool // networks need recomputing
	// machineHeat is this step's heating from machines, per tile.
	machineHeat []float64
}

func newEconomy(cells int) economy {
	return economy{
		at:          map[int]*Building{},
		links:       map[[2]int]struct{}{},
		machineHeat: make([]float64, cells),
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
	ErrNoOre         = errors.New("Mines must go on an ore deposit")
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
	case spec.Extracts && p.Tiles[idx].Ore <= 0:
		return ErrNoOre
	case p.eco.Stores[Metal] < spec.Cost:
		return fmt.Errorf("%s needs %.0f metal; the stores have %.1f", spec.Name, spec.Cost, p.eco.Stores[Metal])
	}
	return nil
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
	delete(p.eco.at, idx)
	p.eco.buildings = slices.DeleteFunc(p.eco.buildings, func(x *Building) bool { return x == b })
	for l := range p.eco.links {
		if l[0] == idx || l[1] == idx {
			delete(p.eco.links, l)
		}
	}
	p.eco.dirty = true
	return nil
}

// LinkLength is the distance between the centres of tiles a and b, in tile
// widths.
func (p *Planet) LinkLength(a, b Cell) float64 {
	da, db := p.Grid.DirFromCell(a), p.Grid.DirFromCell(b)
	angle := math.Acos(math.Min(da.Dot(db), 1))
	return angle / (math.Pi / 2 / float64(p.Grid.N))
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
	if l := p.LinkLength(a, b); l > MaxLinkLength {
		return fmt.Errorf("Too far: %.0f tiles (links reach %d)", l, MaxLinkLength)
	}
	if cost := p.LinkCost(a, b); p.eco.Stores[Metal] < cost {
		return fmt.Errorf("Link needs %.1f metal; the stores have %.1f", cost, p.eco.Stores[Metal])
	}
	return nil
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
	for _, l := range p.Links() {
		ra, rb := find(l[0]), find(l[1])
		if ra != rb {
			parent[max(ra, rb)] = min(ra, rb)
		}
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
	for _, n := range p.eco.networks {
		n.PowerSupply, n.PowerDemand = 0, 0
		for _, b := range n.Members {
			n.PowerSupply += b.Spec().PowerOut
			n.PowerDemand += b.Spec().PowerIn
		}
		power := n.PowerSatisfaction()
		for _, b := range n.Members {
			p.runBuilding(b, power)
		}
		for r := range NumResources {
			p.distribute(n, Resource(r))
		}
	}
}

// runBuilding operates b for one step, as fast as power, inputs, output
// space and (for mines) the ore deposit allow.
func (p *Planet) runBuilding(b *Building, power float64) {
	s := b.Spec()
	if s.PowerIn == 0 && s.Inputs == (Amounts{}) && s.Outputs == (Amounts{}) && s.Heat == 0 {
		b.Activity = 1 // passive buildings: lander, solar
		return
	}
	t := &p.Tiles[b.Tile]
	f := power
	for r := range NumResources {
		if in := s.Inputs[r]; in > 0 {
			f = math.Min(f, b.In[r]/in)
		}
		if out := s.Outputs[r]; out > 0 {
			f = math.Min(f, (bufferCap-b.Out[r])/out)
		}
	}
	if s.Extracts {
		f = math.Min(f, t.Ore/s.Outputs[Ore])
	}
	f = math.Max(f, 0)
	b.Activity = f

	for r := range NumResources {
		b.In[r] -= s.Inputs[r] * f
		b.Out[r] += s.Outputs[r] * f
	}
	if s.Extracts {
		t.Ore = math.Max(t.Ore-s.Outputs[Ore]*f, 0)
	}
	t.Pollution += s.Pollution * f
	if s.Heat > 0 {
		p.eco.machineHeat[b.Tile] += s.Heat * f
		for _, nb := range p.neighbors[b.Tile] {
			p.eco.machineHeat[nb] += s.Heat * f / 2
		}
	}
}

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
			give := math.Min(need(b), p.eco.Stores[r])
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
			give := math.Min(want, producer.Out[r])
			producer.Out[r] -= give
			consumer.In[r] += give
		}
	}
}
