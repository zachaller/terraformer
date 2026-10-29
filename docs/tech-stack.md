# Terraformer: Tech Stack & Architecture

Status: decided (early planning). No code written yet.

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
| Planet grid | Cube-sphere (6 square grids projected onto a sphere) | Trivial tile lookup in a shader; simple neighbour rules |
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

## Simulation model (first pass)

- **Local layer:** per-tile cellular-automaton rules on a fixed tick, e.g.
  toxic → barren next to a filter; barren → grass next to water; grass spreads.
- **Global layer:** a few planet stats (temperature, atmosphere, water level),
  fed by tiles and buildings. Thresholds unlock tile types (e.g. oceans need
  temperature above 0 °C).
- **Deterministic:** seeded RNG and a fixed tick, so planets are reproducible and
  rules are easy to test.
- **Scale:** a few thousand tiles (e.g. 6 × 32 × 32 ≈ 6,000), which is cheap in
  plain Go.

## Game design decisions

- **Open sandbox.** No levels or fail states; one planet you keep shaping.
- **Emergent life.** The player never places life directly. Machines change
  conditions (temperature, atmosphere, water, soil); life appears and spreads
  on its own wherever conditions suit it, and dies back if they stop suiting it.
- **Ecological succession.** Life arrives in stages, each needing the one before:
  microbes → lichen/moss → grass → shrubs → forest on land; plankton → fish in
  water; animals once there's enough plant life to feed them. Each stage has a
  habitable range of conditions defined in `content/`.

## Play Store checklist (for later)

- Google Play developer account (one-time $25 fee).
- New personal accounts must run a closed test with a minimum number of testers
  over a set period before going public (12 testers / 14 days at last check;
  verify current rules). Recruit testers early.
- AAB format, 64-bit (arm64), current target API level (raised every year).
- Privacy policy URL, content rating questionnaire, store listing art.
- Monetization: paid app or free with no ads to start; Play Billing can wait.

## Roadmap

1. **Globe prototype (desktop):** rotating shader planet, drag to rotate, pinch or
   scroll to zoom, tap to highlight a tile.
2. **Globe prototype on a phone:** same thing via `ebitenmobile`; check feel and
   frame rate.
3. **Sim core:** grid, tile types, a couple of spread rules, global stats, tests.
4. **First playable:** place one or two buildings, watch the planet change.
5. **Content and polish:** more buildings, biomes, art, audio, UI.
6. **Release pipeline:** CI AAB build, closed test, Play Store listing.

## Open design questions

- Machines as scaffolding: once an ecosystem can sustain itself, can or should
  the player remove the machines? (A lighter, optional take on Terra Nil's
  "recycle everything and leave".)
- Development setup: is an Android device and Android Studio available?
