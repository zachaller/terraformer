# Terraformer: Tech Stack & Architecture

Status: decided. Roadmap steps 1–5 are built; see README.md.

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
| Planet grid | Cube-sphere (6 square grids projected onto a sphere), conformal mapping | Trivial tile lookup in a shader; simple neighbour rules; square tiles whose rows run straight across face seams |
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
   the current rotation, and converts the hit point to a (cube face, u, v)
   coordinate.
3. It samples a **tile data texture** (tile type per cell, updated by the
   simulation) and colours the pixel: biome blending, lighting, an atmosphere
   rim whose strength follows atmosphere level, and water that follows sea level.
4. Buildings and plants are sprites drawn at projected positions and hidden
   when they rotate to the far side.

Tapping does the same maths in Go: screen point → sphere hit → cube face cell.

**Cube to sphere.** A square grid can't cover a sphere evenly: somewhere
the grid needs points where three rows meet instead of four. A cube-sphere
puts those at the cube's eight corners. The face grid uses the conformal
mapping of Rančić, Purser and Mesinger (1996), with the Taylor-series
coefficients from their Table B1 (as reproduced in
[CubedSphere.jl](https://github.com/CliMA/CubedSphere.jl)). Conformal means
tiles are square rather than sheared, and grid lines cross the seams between
faces at right angles, so rows carry on straight from one face to the next.
The trade-off is tile size: tiles shrink towards the corners (the corner
tile has about a ninth of the area of a face-centre tile, the next one a
fifth). The earlier equal-angle mapping kept sizes within about 25% but bent
every row at the seams.

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

All in `sim/step.go`, deterministic (seeded RNG, fixed tick of 10 steps per
in-game day at 1x speed), about 0.5 ms per step for 6,144 tiles on desktop.

- **Temperature:** each tile relaxes towards an equilibrium set by latitude,
  the greenhouse effect (rises with atmospheric pressure), elevation and ice
  cover (ice reflects sunlight), plus any heater warmth; heat also diffuses to
  the four neighbours.
- **Ice and water:** ice melts above 0 °C and water freezes below. The ice
  holds frozen CO₂, so melting releases atmosphere: the global feedback loop.
- **Water flow:** water above the soil's retained moisture (0.05) flows to
  lower neighbours. Pairwise exchange keeps total water exactly conserved.
- **Life:** microbes appear by chance on tiles that are between -5 and 60 °C
  with some moisture, grow logistically, spread to neighbours, and die back
  when conditions turn bad.
- **Tuning:** all rates are constants at the top of `sim/step.go`;
  `maxReleasedCO2` in `sim/planet.go` controls how strong the CO₂ feedback is,
  which decides how easily the planet tips into runaway warming. Currently a
  ring of ~37 equatorial heaters tips it in about 150 days.

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
6. **Content and polish:** more buildings, biomes, art, audio, UI.
7. **Release pipeline:** CI AAB build, closed test, Play Store listing.

## Open design questions

- Machines as scaffolding: once an ecosystem can sustain itself, can or should
  the player remove the machines? (A lighter, optional take on Terra Nil's
  "recycle everything and leave".)
- Development setup: is an Android device and Android Studio available?
