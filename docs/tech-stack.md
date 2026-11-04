# Terraformer: Tech Stack & Architecture

Status: decided. Roadmap steps 1–5 are built, and 6 and 7 are under way; see README.md.

## The game in one paragraph

A mobile terraforming game in the spirit of *Terra Nil* and *Spore*'s space
stage: a rotatable planet made of tiles. Local buildings spread effects to
neighbouring tiles (Terra Nil), while planet-wide stats such as temperature,
atmosphere and sea level gate which transformations are possible (Spore).
Hobby project, targeting release on the Google Play Store.

## Decisions

| Area | Choice | Why |
|---|---|---|
| Language | Go | Developer's favourite language; hobby project, so enjoyment matters |
| Engine | [Ebitengine](https://ebitengine.org) (v2) | Native Go, mature Android support, games already shipped on Play |
| Shaders | Kage (Ebitengine's Go-like shader language) | Built in |
| Planet rendering | 2D "shader globe" | Ebitengine is 2D only; see below |
| Planet grid | Geodesic hex grid (hexagons plus 12 pentagons, built on an icosahedron) | Nearly even tile sizes and shapes, no visible seams or corners; still a cheap lookup in the shader |
| Simulation | Pure Go package, engine-independent | Testable with `go test`; portable if the engine ever changes |
| Content | Data files (YAML/JSON) for tiles, buildings, rules | Balance without code changes |
| Android build | `ebitenmobile bind` → `.aar` → thin Android Studio project → AAB | The standard Ebitengine mobile path |
| CI | GitHub Actions (Go + Android SDK + Gradle) | Reproducible release builds |

### Alternatives considered

- **Godot 4 + GDScript**: strong engine, but not Go.
- **Godot + Go via graphics.gd**: real 3D, but community bindings and a newer
  Android path. A fallback if the shader globe doesn't feel good.
- **Unity**: heavier; C#, not Go.
- **Pure-Go 3D engines (G3N, Kaiju)**: not viable on Android today.

## The shader globe

Ebitengine has no 3D, so the planet is faked:

1. Draw one screen-space quad where the planet should appear.
2. A Kage fragment shader treats each pixel as a ray hitting a sphere, applies
   the current rotation, and finds the tile under it (see below).
3. It samples a **tile data texture** (tile type per cell, updated by the
   simulation) and colours the pixel: biome blending, lighting, an atmosphere
   rim whose strength follows atmosphere level, and water that follows sea level.
4. Buildings and plants are sprites drawn at projected positions and hidden
   when they rotate to the far side.

Tapping does the same maths in Go: screen point → sphere hit → tile.

**The tile grid.** No grid of identical squares or hexagons can cover a
sphere; every approach puts the irregularity somewhere (games use latitude
bands, cube-spheres, HEALPix, hex spheres or relaxed irregular grids).
Terraformer uses a
geodesic hex grid, as *Before We Leave* and Oskar Stålberg's *Planet* do:
an icosahedron whose 20 triangular faces are each divided into a triangular
lattice with N steps per edge. Each lattice point is the centre of a tile,
so tiles are hexagons, except for the twelve at the icosahedron's corners,
which are pentagons. There are 10·N² + 2 tiles (6,252 at the default N = 25).

- **Addressing.** The faces pair up into ten diamonds, each an N×N patch of
  lattice points (I, J), plus the two poles. Points on a diamond's edges are
  shared; each is owned by exactly one diamond (`Grid.LatticeCell`).
  Neighbours are found once per grid size by linking adjacent lattice
  points within every diamond.
- **Even sizes.** Projecting a flat face onto the sphere shrinks tiles near
  its corners to under half the size of those at its centre. Within each
  face, positions are warped first (each barycentric weight w becomes
  w − w²/4), which brings the smallest tile to about 0.83 of the largest.
  The warp depends only on the weights, symmetrically, so neighbouring faces
  agree along their shared edge.
- **Lookup.** Go and the shader do the same thing: pick the face whose
  centre is nearest the direction, compute barycentric coordinates there,
  undo the warp (a few Newton steps), and take the nearest lattice point.
  The shader's atlas has a pixel for every lattice point in each diamond,
  including shared edges, so the three tiles around any point can be
  blended for smooth colours.

Earlier versions used a cube-sphere with equal-angle, then conformal,
mappings. Those gave square tiles, but visible corners and either bent rows
at the cube's seams or tiles that shrank badly towards its corners.

**This is the biggest technical risk, so it's the first prototype.**

## Project layout

```
terraformer/
  sim/       pure Go: planet grid, tiles, rules, global stats (no engine imports)
  content/   tile, building and rule definitions (data files)
  game/      Ebitengine: rendering, shaders, input, UI, audio
  cmd/desktop/  desktop entry point (main dev loop)
  mobile/    ebitenmobile entry point
  android/   thin Android Studio project that builds the AAB
  docs/      design and technical docs
```

Most development happens on desktop with an instant run loop; the phone is for
regular check-ins on feel and performance.

## Simulation model (as built in step 3)

Deterministic (seeded RNG, fixed tick of 10 steps per in-game day at 1x
speed), about 0.8 ms per step for about 6,000 tiles on desktop. Superseded in
its details by the terraforming model below.

- **Temperature:** each tile relaxes towards an equilibrium set by latitude,
  the greenhouse effect, elevation and albedo, plus any heater warmth; heat
  also diffuses to the six neighbours (five for the pentagons).
- **Ice and water:** ice melts above 0 °C (if the air is above water's
  triple point) and water freezes below.
- **Water flow:** water above the soil's retained moisture (0.05) flows to
  lower neighbours. Pairwise exchange keeps total water exactly conserved.
- **Life:** microbes appear by chance on tiles that are between -5 and 60 °C
  with some moisture, grow logistically, spread to neighbours, and die back
  when conditions turn bad.

## Game design decisions

- **Open sandbox.** No levels or fail states; one planet you keep shaping.
- **Emergent life.** The player never places life directly. Machines change
  conditions (temperature, atmosphere, water, soil); life appears and spreads
  on its own wherever conditions suit it, and dies back if they stop suiting it.
- **Ecological succession.** Life arrives in stages, each needing the one before:
  microbes → lichen/moss → grass → shrubs → forest on land; plankton → fish in
  water; animals once there's enough plant life to feed them. Each stage has a
  habitable range of conditions defined in `content/`.
- **Resource economy (Factorio-lite).** Machines cost materials to build and
  consume inputs to run. Loop: extract → process → build/run machines → planet
  changes → new resources unlock.
- **Industry vs. ecology.** Mines scar tiles and factories pollute nearby, which
  works against the life the player is growing. Placement is the puzzle. A
  self-sustaining ecosystem lets the player shut polluting machines down.

### Resources (first draft)

| Tier | Resources | Source |
|---|---|---|
| Raw | Ore, ice, carbon rock, silicates | Mining tiles; deposits are scattered, so location matters |
| Processed | Metal, water, oxygen, hydrogen, components | Factories (e.g. smelter: ore → metal; electrolyser: water → O₂ + H₂) |
| Energy | Power | Solar early; wind once there's atmosphere; geothermal near volcanic tiles; biofuel once there's life |
| Living | Biomass, wood | Harvested from grown life; over-harvesting makes it die back |

### Logistics: networks, not belts

- The player **drags a link** between buildings to connect them. Connected
  buildings form a **network**, and resources flow through it automatically.
- Each link has a **capacity** (units per tick) and a cost that grows with
  length, so distance and layout still matter.
- Buildings have small input/output buffers; production stalls when inputs run
  dry or outputs back up, which makes bottlenecks visible.
- Power travels on the same networks (or a parallel power layer; decide during
  prototyping).
- Conveyor belts are deliberately left out: fiddly with fingers on a rotating
  globe. They may be added later as an optional extra.

**Simulation:** each tick, `sim/` finds connected networks (a graph), then
distributes supply to demand within each network, respecting link capacities
and processing buildings in a fixed order so results stay deterministic. A simple
greedy pass is enough to start; a proper max-flow solver is only needed if
capacity-limited routing turns out to matter for gameplay.

### Economy as built (step 4)

- Buildings and their numbers live in one table, `sim.Specs` in
  `sim/buildings.go` (moving to data files later is still the plan).
- Each step, `sim/economy.go` regroups buildings into networks when links
  change (union-find), shares power within each network (machines run at the
  supplied/demanded ratio), runs each machine as fast as power, inputs,
  output space and ore allow, then moves resources: a network with the lander
  pools output into the colony stores and feeds consumers from them; one
  without passes output directly from producers to consumers in tile order.
- Links have a maximum length and a per-tile cost, but no throughput limit yet.
- Pollution is emitted by mines and smelters, drifts to neighbours and decays;
  above 0.25 a tile can't support life.

### First playable (step 5)

- **Goals** (`sim/goals.go`): an ordered list of milestones, each with a hint
  and a progress function; completed in order, checked every step, saved.
- **Saving** (`sim/save.go`, `game/save.go`): gzip-compressed gob of the
  dynamic state; terrain-derived data is regenerated from seed and size.
  Written via a temp file and rename. Saved every 30 s, on Android `onPause`,
  and on desktop window close.
- **Balance** is checked by `TestPlaythrough`, a bot that plays every goal
  without shortcuts. Current pacing for the bot at 1x: first life ~1 min,
  1% life ~8 min, 2 kPa ~13 min, thaw ~19 min, living world ~20 min. A human
  is slower; the test fails if the planet becomes trivially easy to thaw.

### Life stages (after step 5)

`sim/life.go`: each tile has a density per species after microbes. A
species can only appear on a tile where the one before it in its chain is
established (density 0.5), then grows logistically, spreads to neighbours,
and dies back when conditions fail. Land: moss (above 0°, 1 kPa), grass
(5°, 8 kPa), shrubs (8°, 20 kPa), forest (10°, 35 kPa), all needing damp
but not flooded ground. Water (standing water 0.2+): plankton (2°), fish
(8°, 20 kPa). Goals for moss, the seas, grasslands and forests extend the
playthrough; the bot reaches forests at about 26 minutes at 1x.

### Terraforming model (version 0.9)

A plausible model of the mechanisms proposed for terraforming Mars, with
timescales compressed so the whole arc fits a 5–10 hour game. Constants are
grouped at the top of `sim/atmosphere.go`, `sim/step.go`, `sim/life.go` and
`sim/buildings.go`.

- **Atmosphere** (`sim/atmosphere.go`): partial pressures of CO₂, N₂ and O₂
  in kPa, and PFCs in Pa. Greenhouse warming is 7 °C per doubling of
  (1 + CO₂ kPa), 12 °C per doubling of (1 + PFC Pa), 1.5 °C per doubling of
  (1 + pressure/10) for pressure broadening, and up to 10 °C from water
  vapour. Thicker air also carries heat from the equator towards the poles.
  PFCs break down slowly in sunlight, and without a running magnetic shield
  every gas escapes at 0.0025% a day.
- **Water**: liquid only above the triple point (0.62 kPa); below it, warmed
  ice sublimates into the air and falls again as frost on freezing ground.
  The boiling point follows Clausius–Clapeyron. Standing water evaporates
  (faster when warm), damp ground more slowly; 70% of rain falls back where it
  evaporated and the rest spreads over the planet, more on high ground, as
  snow where it's freezing.
- **CO₂ reservoirs**: dry ice in the polar caps is held at the CO₂ frost
  point: the caps keep the air at the pressure where CO₂ would freeze at
  their average temperature, releasing dry ice when warmed (warmest parts
  first) and slowly freezing extra CO₂ back. The frost point is shifted per
  planet so its caps hold exactly the starting 0.45 kPa, since the game's
  temperatures run warmer than real Mars. The regolith holds 45 kPa more,
  released from each tile once it's 8 °C warmer than at the start (all of it
  60 °C beyond that). Around 3 kPa the greenhouse feedback overtakes the caps'
  pull, and the planet tips into runaway warming to about 40 kPa.
- **Albedo**: per tile, from terrain (dark basalt basins 0.16 to bright peaks
  0.3), water (0.07), plants (0.14), and ice (0.5, down to 0.2 with dust). Each
  0.01 less reflected warms a tile by 0.45 °C.
- **Life** (`sim/life.go`): species need temperature, pressure, water, and now
  nitrogen, oxygen, an ozone layer (full at 2 kPa of O₂) and soil, which
  microbes and plants build. Photosynthesis turns CO₂ into O₂ in proportion to
  plant cover; standing water locks CO₂ into carbonate rock. Biolabs make new
  life much likelier to take hold nearby.
- **Economy**: deposits of iron (smelted into metal), fluorite (for
  greenhouse factories) and nitrate (for nitrogen plants), volcanic tiles
  for geothermal power, and ice and seas for water extractors. Building types unlock as goals
  are completed (`BuildingSpec.UnlockedBy`). Comets are aimed like buildings
  and strike after 30 days.
- **Pacing** is checked by `TestPlaythrough`, whose bot plays every goal; the
  test fails if it finishes in under two hours of game time at 1×, since
  people take several times longer than the bot.

## Play Store checklist (for later)

- Google Play developer account (one-time $25 fee).
- New personal accounts must run a closed test with a minimum number of testers
  over a set period before going public (12 testers / 14 days at last check;
  verify current rules). Recruit testers early.
- AAB format, 64-bit (arm64), current target API level (raised every year).
- Privacy policy URL, content rating questionnaire, store listing art.
- Monetization: paid app or free with no ads to start; Play Billing can wait.

## Roadmap

1. ✅ **Globe prototype (desktop):** rotating shader planet, drag to rotate, pinch or
   scroll to zoom, tap to highlight a tile.
2. ✅ **Globe prototype on a phone:** same thing via `ebitenmobile`; runs smoothly
   at 120 FPS on the developer's phone.
3. ✅ **Sim core:** grid, tile types, a couple of spread rules, global stats, tests.
4. ✅ **Economy core:** a mine, a smelter, a power source and links between them;
   network flow in `sim/` with tests.
5. ✅ **First playable:** build a small production chain that powers one
   terraforming machine, and watch the planet change and the first life appear.
6. **Content and polish:** more buildings, biomes, art, audio, UI. Mostly
   done: 20 buildings, 25 goals and a finale (a self-sustaining planet,
   lift-off and harder planets after), synthesised audio, a first-time
   walkthrough, an app icon (drawn by `cmd/icon`, like the rest of the art,
   in code); still to do: more art.
7. **Release pipeline:** CI and GitHub Releases are done (`.github/workflows/`:
   tests and every platform's build on each push, and a release from the
   Actions tab or a version tag). Still to do: the Play upload key, then a
   closed test and the Play Store listing.

## Open design questions

- ~~Machines as scaffolding~~: answered by the finale. The last goal asks
  the player to remove every machine once the Soletta and L1 shield can do
  their work from orbit, and the planet must hold for 1000 days. A carbon
  cycle (respiration, decay and volcanoes against photosynthesis and
  carbonate rock) keeps a living planet's air from running down.
- Development setup: is an Android device and Android Studio available?
