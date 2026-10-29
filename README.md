# Terraformer

A mobile terraforming game in Go with [Ebitengine](https://ebitengine.org):
shape a barren planet until life takes hold. See
[docs/tech-stack.md](docs/tech-stack.md) for the design and technical plan.

![Globe prototype](docs/images/globe-prototype.png)

## Status: globe prototype

- A procedurally generated cube-sphere planet (6 × 32 × 32 tiles by default).
- Rendered as a 2D "shader globe": one screen quad and a Kage shader that
  ray-traces the sphere and looks each pixel up in a tile atlas.
- Drag to rotate (with momentum), mouse wheel or pinch to zoom, click or tap to
  select a tile.

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
| `-screenshot` | | Save a PNG of the first frames to this path and exit |

Headless screenshot (e.g. on a server):
`xvfb-run -a go run ./cmd/desktop -screenshot shot.png`

## Tests

```sh
go test ./...
```

## Layout

```
sim/          pure Go simulation: cube-sphere grid, planet generation (no engine imports)
game/         Ebitengine front end: camera, input, planet shader, HUD
cmd/desktop/  desktop entry point
docs/         design and technical docs
```
