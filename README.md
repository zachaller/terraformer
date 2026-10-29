# Terraformer

A mobile terraforming game in Go with [Ebitengine](https://ebitengine.org):
shape a barren planet until life takes hold. See
[docs/tech-stack.md](docs/tech-stack.md) for the design and technical plan.

![Starting planet](docs/images/globe-prototype.png)
![After warming: seas and microbes](docs/images/terraformed.png)

## Status: simulation core (roadmap step 3)

- A procedurally generated cube-sphere planet (6 × 32 × 32 tiles by default),
  rendered as a 2D "shader globe": one screen quad and a Kage shader that
  ray-traces the sphere and looks each pixel up in a tile atlas.
- A running climate simulation: temperature by latitude, greenhouse effect,
  heat spreading between tiles, ice melting and freezing, water flowing
  downhill, and microbes appearing and spreading where it's warm and wet.
  Melting ice releases CO₂, which thickens the atmosphere and warms the whole
  planet: heat enough of it and the planet tips into runaway warming.
- A temporary **Heater** tool (each tap adds +30 °C to a tile and +15 °C to its
  neighbours) to experiment with before real machines exist.

### Controls

| Action | Touch | Desktop |
|---|---|---|
| Rotate | Drag | Drag with left mouse button |
| Zoom | Pinch | Mouse wheel |
| Inspect a tile, or place a heater | Tap | Click |
| Switch tool (Inspect / Heater) | **Tool** button | `H` |
| Cycle view (Surface / Temperature / Water / Life) | **View** button | `V` |
| Cycle speed (Paused / 1x / 4x) | **Speed** button | `Space` pause, `F` fast |

## Running on desktop

Needs Go 1.25+. On Linux, also install Ebitengine's system dependencies:

```sh
sudo apt install libc6-dev libgl1-mesa-dev libxcursor-dev libxi-dev \
  libxinerama-dev libxrandr-dev libxxf86vm-dev libasound2-dev pkg-config
```

macOS and Windows need nothing extra. Then:

```sh
go run ./cmd/desktop                 # default planet
go run ./cmd/desktop -seed 7 -size 64
```

| Flag | Default | Meaning |
|---|---|---|
| `-seed` | 1 | Planet seed; the same seed always gives the same planet |
| `-size` | 32 | Tiles per cube-face edge (total tiles = 6 × size²) |
| `-view` | surface | Starting view: `surface`, `temp`, `water` or `life` |
| `-demo-steps` | 0 | Place a ring of heaters around the equator and fast-forward this many steps (10 steps = 1 day) |
| `-screenshot` | | Save a PNG of the first frames to this path and exit |

Headless screenshot (e.g. on a server):
`xvfb-run -a go run ./cmd/desktop -screenshot shot.png`

## Running on Android

Build a debug APK (signed with a debug key, for sideloading):

```sh
export ANDROID_HOME=/path/to/android-sdk            # platform 36, build-tools
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/<version>
go install github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@v2.10.5
scripts/build-android.sh
```

The script binds the Go game into `android/app/libs/terraformer.aar` with
`ebitenmobile`, then runs Gradle. The APK lands in
`android/app/build/outputs/apk/debug/app-debug.apk`. With a phone connected
over USB (developer options and USB debugging on):
`adb install -r android/app/build/outputs/apk/debug/app-debug.apk`

## Tests

```sh
go test ./...
```

## Layout

```
sim/          pure Go simulation: cube-sphere grid, planet generation, climate/water/life rules (no engine imports)
game/         Ebitengine front end: camera, input, planet shader, views, buttons, HUD
cmd/desktop/  desktop entry point
mobile/       Android entry point, bound by ebitenmobile
android/      thin Android project (one Activity hosting the game view)
scripts/      build scripts
docs/         design and technical docs
```
