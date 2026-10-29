# Terraformer

A mobile terraforming game in Go with [Ebitengine](https://ebitengine.org):
shape a barren planet until life takes hold. See
[docs/tech-stack.md](docs/tech-stack.md) for the design and technical plan.

![Starting planet](docs/images/globe-prototype.png)
![After warming: seas and microbes](docs/images/terraformed.png)
![A small colony: lander, solar, mine, smelter and heater](docs/images/colony.png)

## Status: economy core (roadmap step 4)

- A procedurally generated cube-sphere planet (6 × 32 × 32 tiles by default),
  rendered as a 2D "shader globe": one screen quad and a Kage shader that
  ray-traces the sphere and looks each pixel up in a tile atlas.
- A running climate simulation: temperature by latitude, greenhouse effect,
  heat spreading between tiles, ice melting and freezing, water flowing
  downhill, and microbes appearing and spreading where it's warm, wet and
  clean. Melting ice releases CO₂, which thickens the atmosphere and warms the
  whole planet: heat enough of it and the planet tips into runaway warming.
- A colony economy: choose a landing site, then build and link machines.

| Building | Cost (metal) | Does |
|---|---|---|
| Lander | free, placed first | Holds the colony stores; makes 3 power |
| Solar | 4 | Makes 2 power |
| Mine | 6 | Digs ore from a deposit; uses 1 power; pollutes |
| Smelter | 8 | Turns 2 ore into 1 metal; uses 2 power; pollutes heavily |
| Heater | 6 | Warms its tile by up to +40 °C (neighbours +20 °C); uses 3 power |

Links connect buildings into networks. A network shares its power (short of
power, everything in it slows down and its links turn orange) and moves ore
and metal between buildings. Networks linked to the lander send surplus into
the colony stores, which pay for construction, and feed machines from them.
Links reach 8 tiles and cost 0.5 metal per tile. Pollution from mines and
smelters stops life growing nearby, so where you put industry matters.
Buildings that are idle (unpowered, out of input, or with a full output
buffer) pulse red.

### Controls

| Action | Touch | Desktop |
|---|---|---|
| Rotate | Drag | Drag with left mouse button |
| Zoom | Pinch | Mouse wheel |
| Use the current tool on a tile | Tap | Click |
| Open the build menu (Inspect, Link, Demolish, buildings) | **Tool** button | `B`, or `1`–`7` to pick directly |
| Link buildings | Link tool: tap one building, then another (chains continue from the last) | same |
| Cycle view (Surface / Temperature / Water / Life / Industry) | **View** button | `V` |
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
| `-view` | surface | Starting view: `surface`, `temp`, `water`, `life` or `industry` |
| `-zoom` | 1 | Starting zoom |
| `-demo-colony` | false | Land and build a small working colony facing the camera |
| `-demo-heaters` | false | Put fixed warmth on a ring of tiles around the equator (enough to tip the planet into runaway warming) |
| `-demo-steps` | 0 | Fast-forward this many steps before starting (10 steps = 1 day) |
| `-width`, `-height` | 1024, 720 | Window size |
| `-safe-area` | | Simulate phone system bars: `left,top,right,bottom` insets in pixels |
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
sim/          pure Go simulation: grid, planet generation, climate/water/life rules, buildings and networks (no engine imports)
game/         Ebitengine front end: camera, input, planet shader, views, build menu, HUD
cmd/desktop/  desktop entry point
mobile/       Android entry point, bound by ebitenmobile
android/      thin Android project (one Activity hosting the game view)
scripts/      build scripts
docs/         design and technical docs
```
