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
- A climate and atmosphere simulation built on real terraforming ideas
  (mostly the ones proposed for Mars), with timescales compressed so a planet
  takes hours rather than centuries:
  - **Air made of gases**: CO₂, nitrogen, oxygen and factory-made
    super-greenhouse gases (PFCs), each tracked as a partial pressure. Warming
    comes from CO₂, PFCs, thicker air and water vapour (tap the pressure chip
    for the atmosphere sheet).
  - **Water that obeys its phase rules**: below 0.62 kPa (water's triple
    point) warmed ice turns straight to vapour, so liquid water first needs
    thicker air; the boiling point rises with pressure. Water evaporates,
    falls as rain or snow (mostly near where it rose), and flows downhill.
  - **CO₂ reservoirs**: polar caps of dry ice hold the air's CO₂ at the
    pressure where it would freeze at their temperature, and the ground holds
    more, released as it warms. Darken the caps or warm the planet enough and
    it tips into runaway warming.
  - **Albedo**: ice reflects sunlight, dark basins, water and plants absorb
    it, and dust darkens ice.
  - **Losing the air**: without a magnetic shield the solar wind slowly
    strips the atmosphere away.
  - **Life tied to the air**: microbes, then moss, grass, shrubs and forest on
    land (needing soil that earlier life builds, nitrogen, and an ozone layer
    that oxygen forms); plankton and fish in standing water; grazing animals
    once there's enough oxygen. Plants turn CO₂ into oxygen, and seas (and,
    late on, carbon scrubbers) lock CO₂ away, so late in the game the planet
    needs greenhouse gases to stay warm as CO₂ falls.
- A colony economy: choose a landing site, then build and link machines.
  Deposits of iron, fluorite and nitrate, and volcanic tiles, are scattered
  over the planet; buildings unlock as goals are completed.
- 24 goals that guide you from landing to air people could breathe, shown at
  the top of the screen with a hint and progress (tap the card for the full
  list): Land → Dig for ore → Make metal → Warm the ground → Above the triple
  point → Liquid water → First life → Spread life → Super greenhouse → Thicken
  the air → First moss → Release the caps → Thaw → A breath of oxygen → Seas →
  Nitrogen → Grasslands → Shield the air → Life in the seas → Forests →
  Oxygen-rich → Grazers → Living world → Breathable air. Completing one offers
  a Show me button. Expect 5–10 hours.
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

| Building | Cost (metal) | Unlocks after | Does |
|---|---|---|---|
| Lander | free, placed first | | Holds the colony stores; makes 3 power |
| Solar | 4 | | Makes 2 power |
| Pylon | 3 | | Connects every building within 4 tiles into its network without links; its links reach 12 tiles |
| Wind turbine | 10 | Thicken the air | Up to 6 power, from 2 kPa, full at 40 kPa |
| Geothermal | 18 | First life | 10 power on a volcanic tile; warms it |
| Mine | 6 | | Digs a deposit: iron ore, fluorite or nitrate; 1 power; pollutes |
| Smelter | 8 | | Turns 2 ore into 1 metal; 2 power; pollutes heavily |
| Ice drill | 10 | Warm the ground | Mines ice into water; 2 power |
| Heater | 6 | | About +30 °C on its tile, +19 °C on the ring around it, fading out; 3 power |
| Dust spreader | 12 | Warm the ground | Darkens ice up to 3 tiles around so it absorbs sunlight; 2 power |
| Greenhouse factory | 25 | Liquid water | Turns fluorite into PFCs, which warm the whole planet; 4 power |
| Orbital mirror | 150 | Thicken the air | +45 °C of focused sunlight, fading over 4 tiles; no power |
| Electrolyser | 20 | Liquid water | Splits water into oxygen; 4 power |
| Nitrogen plant | 20 | First moss | Turns nitrate into nitrogen; 3 power |
| Magnetic shield | 400 | Thaw | Stops the air escaping to space; 20 power; one only |
| Carbon scrubber | 35 | Oxygen-rich | Captures CO₂ from the air and locks it away; 6 power |
| Biolab | 30 | First life | Life takes hold far faster up to 3 tiles around; 3 power |
| Comet | 120 | Thaw | Strikes 30 days after it's aimed: water, nitrogen and CO₂, a crater, and destruction 2 tiles around; one at a time |

Links connect buildings into networks. A new building links itself to the
nearest building in reach (preferring the lander's network); the placement
preview shows the link and its cost, or that a pylon connects it for free. A network shares its power (short of
power, everything in it slows down and its links turn orange) and moves ore
and metal between buildings. Networks linked to the lander send surplus into
the colony stores, which pay for construction, and feed machines from them.
Links reach 8 tiles and cost 0.5 metal per tile. Buildings that don't need
the stores can run on their own small network (a dust spreader on a polar cap
with a solar panel beside it, say). Pollution from mines, smelters and
factories stops life growing nearby, so where you put industry matters.
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
| Build menu (by category: Power, Industry, Climate, Air, Projects; and new planet) | **Build** | `B`, or `1`–`4` for Solar, Mine, Smelter, Heater |
| Place a building: tap a tile to preview it (heat or pollution area, effects, warnings), tap another to move it, then **Build here** | Tap, then **Build here** | Click, then `Enter` |
| Link buildings by hand (new buildings link themselves): drag from one to another (shows reach and cost); or tap one, then the other | **Link**, then drag | `L`, then drag |
| Inspect a tile or building | **Inspect** | `I` |
| Remove a building (half its metal back) | **Remove** | `X` |
| Map view (Surface / Temperature / Water / Life / Industry) | Right-hand buttons | `V` |
| Speed (Paused / 1× / 4× / 16×) | Speed pill | `Space` pause, `F` fastest |
| Atmosphere details | Tap the kPa chip | |
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
| `-demo-showcase` | false | With `-demo-colony`, also build one of every other building |
| `-demo-heaters` | false | Put fixed warmth on a ring of tiles around the equator and a little greenhouse gas in the air: enough to tip the planet into runaway warming |
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
go test ./...          # includes a bot playthrough of every goal (several minutes)
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
