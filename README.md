# Terraformer

A mobile terraforming game in Go with [Ebitengine](https://ebitengine.org):
shape a barren planet until life takes hold. See
[docs/tech-stack.md](docs/tech-stack.md) for the design and technical plan.

![Starting planet](docs/images/globe-prototype.png)
![After warming: seas and microbes](docs/images/terraformed.png)
![A small colony: lander, solar, mine, smelter and heater](docs/images/colony.png)
![Main screen](docs/images/ui-main.png)
![Inspect panel](docs/images/ui-inspect.png)
![A living planet: seas, grassland and forest](docs/images/late-game.png)

## Status: first playable (roadmap step 5)

- A procedurally generated planet covered in a geodesic hex grid (6,252
  tiles by default: hexagons plus the twelve pentagons a sphere needs),
  rendered as a 2D "shader globe": one screen quad and a Kage shader that
  ray-traces the sphere and looks each pixel up in a tile atlas. Tile colours
  are blended smoothly and broken up with noise, so the planet looks natural
  rather than like a ball of hexagons; a tile grid appears only around the
  tapped tile while a build, link or demolish tool is active.
- A running climate simulation: temperature by latitude, greenhouse effect,
  heat spreading between tiles, ice melting and freezing, water flowing
  downhill, and microbes appearing and spreading where it's warm, wet and
  clean. Life then follows in stages, each needing the one before it on its
  tile plus warmer ground and thicker air: moss, grass, shrubs and forest on
  land; plankton and fish in standing water. Melting ice releases CO₂, which thickens the atmosphere and warms the
  whole planet: heat enough of it and the planet tips into runaway warming.
- A colony economy: choose a landing site, then build and link machines.
- Goals that guide you from landing to forests, shown at the top of the
  screen with a hint and progress (tap the card for the full list): Land →
  Dig for ore → Make metal → Warm the ground → Liquid water → First life →
  Spread life → Thicken the air → First moss → Thaw → Living world → Life in
  the seas → Grasslands → Forests. Completing one offers a Show me button.
- Live alerts under the goal card for colony problems (no power, short of
  power, empty mines, starved or blocked buildings), each with a one-tap fix.
- A designed interface (see the [UX mocks](https://claude.ai/artifact/LMMKh2HSCZaoKDpbrFCqT3)):
  Sora and JetBrains Mono fonts (embedded; see `game/fonts/`), status chips,
  a goal card and goals list, alerts, a tool dock, a map-view rail, messages,
  an inspect panel (including what lives on a tile and what the next stage
  needs), placement previews and drag-to-link. Buildings are drawn in the
  shader with status badges (bolt: short of power; broken link: no power;
  !: blocked; -: deposit empty).
- Automatic saving every 30 seconds, when the app goes into the background,
  and when the desktop window closes. **Tool → New planet** (tap twice)
  starts over on a fresh planet.

| Building | Cost (metal) | Does |
|---|---|---|
| Lander | free, placed first | Holds the colony stores; makes 3 power |
| Solar | 4 | Makes 2 power |
| Mine | 6 | Digs ore from a deposit; uses 1 power; pollutes |
| Smelter | 8 | Turns 2 ore into 1 metal; uses 2 power; pollutes heavily |
| Heater | 6 | Warms its tile by about +30 °C and the ring around it by about +19 °C, fading over the next few rings; uses 3 power |

Links connect buildings into networks. A network shares its power (short of
power, everything in it slows down and its links turn orange) and moves ore
and metal between buildings. Networks linked to the lander send surplus into
the colony stores, which pay for construction, and feed machines from them.
Links reach 8 tiles and cost 0.5 metal per tile. Pollution from mines and
smelters stops life growing nearby, so where you put industry matters.
Buildings that are idle (unpowered, out of input, or with a full output
buffer) pulse red.

### Controls

The bottom bar holds the tools; the column of buttons on the right switches
map view; the pill above the bar sets the speed. Tap the goal card to
collapse it. With **Inspect**, tapping a tile opens a panel that explains it:
why a building isn't at full speed (with a one-tap fix), whether microbes can
live there and what's stopping them, and the building's network.

| Action | Touch | Desktop |
|---|---|---|
| Rotate | Drag | Drag with left mouse button |
| Zoom | Pinch | Mouse wheel |
| Use the current tool on a tile | Tap | Click |
| Build menu (Solar, Mine, Smelter, Heater, new planet) | **Build** | `B`, or `1`–`4` to pick a building |
| Place a building: tap a tile to preview it (heat or pollution area, effects, warnings), tap another to move it, then **Build here** | Tap, then **Build here** | Click, then `Enter` |
| Link buildings: drag from one to another (shows reach and cost); or tap one, then the other | **Link**, then drag | `L`, then drag |
| Inspect a tile or building | **Inspect** | `I` |
| Remove a building (half its metal back) | **Remove** | `X` |
| Map view (Surface / Temperature / Water / Life / Industry) | Right-hand buttons | `V` |
| Speed (Paused / 1× / 4×) | Speed pill | `Space` pause, `F` fast |
| Collapse the goal card | Tap it | `G` |
| Close panels, cancel a link or placement | Close button | `Esc` |

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
| `-seed` | 1 | Seed for a new planet (ignored when continuing a save); the same seed always gives the same planet |
| `-no-save` | false | Don't load or save the game (saves live in your user config directory under `terraformer/`) |
| `-size` | 25 | Hex grid size: steps along each icosahedron edge (total tiles = 10 × size² + 2) |
| `-view` | surface | Starting view: `surface`, `temp`, `water`, `life` or `industry` |
| `-zoom` | 1 | Starting zoom |
| `-demo-colony` | false | Land and build a small working colony facing the camera |
| `-demo-heaters` | false | Put fixed warmth on a ring of tiles around the equator: enough to tip the planet into runaway warming, with forests by about day 1000 (`-demo-steps 10000`) |
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
go test ./...          # includes a bot playthrough of every goal (~10s)
go test -short ./...   # skips it
go test ./sim -run Playthrough -v   # shows the bot's pacing
```

## Layout

```
sim/          pure Go simulation: grid, planet generation, climate/water/life rules, buildings and networks (no engine imports)
game/         Ebitengine front end: camera, input, planet shader, views, UI (ui.go, inspect.go, draw.go, text.go), fonts
cmd/desktop/  desktop entry point
mobile/       Android entry point, bound by ebitenmobile
android/      thin Android project (one Activity hosting the game view)
scripts/      build scripts
docs/         design and technical docs
```
