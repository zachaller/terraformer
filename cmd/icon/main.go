// Command icon draws the app icon and writes every size the platforms need:
// Android launcher icons (adaptive layers and legacy), the Play Store icon,
// the Mac .icns and the desktop window icon. Run it from the repo root after
// changing the design:
//
//	go run ./cmd/icon
//
// The icon is a planet half way through terraforming: barren rust and ice on
// one side, seas and green land on the other, under a blue glow of air.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

func main() {
	res := "android/app/src/main/res"
	for _, d := range []struct {
		name  string
		scale float64 // relative to mdpi
	}{{"mdpi", 1}, {"hdpi", 1.5}, {"xhdpi", 2}, {"xxhdpi", 3}, {"xxxhdpi", 4}} {
		dir := filepath.Join(res, "mipmap-"+d.name)
		legacy := int(48 * d.scale)
		layer := int(108 * d.scale)
		write(filepath.Join(dir, "ic_launcher.png"), render(legacy, legacyScene, roundedSquare(0.9, 0.18)))
		write(filepath.Join(dir, "ic_launcher_round.png"), render(legacy, legacyScene, circle(0.92)))
		write(filepath.Join(dir, "ic_launcher_background.png"), render(layer, adaptiveBackground, nil))
		write(filepath.Join(dir, "ic_launcher_foreground.png"), render(layer, adaptiveForeground, nil))
		write(filepath.Join(dir, "ic_launcher_monochrome.png"), render(layer, adaptiveMonochrome, nil))
	}

	// The Play Store wants a full square; it rounds the corners itself.
	write("docs/images/icon.png", render(512, legacyScene, nil))
	write("cmd/desktop/icon.png", render(256, legacyScene, roundedSquare(0.9, 0.2)))

	// macOS app icons sit in an 824-pixel rounded square on a 1024 canvas.
	icns := map[string]int{"icp4": 16, "icp5": 32, "icp6": 64, "ic07": 128, "ic08": 256, "ic09": 512, "ic10": 1024}
	mac := map[int]*image.NRGBA{}
	for _, size := range icns {
		mac[size] = render(size, legacyScene, roundedSquare(824.0/1024, 0.225))
	}
	writeICNS("scripts/Terraformer.icns", icns, mac)
}

// A scene gives the colour (premultiplied by alpha) at a point, with x and y
// from -1 to 1 across the image and y pointing up.
type scene func(x, y float64) (r, g, b, a float64)

// A mask gives how much of the point is inside the icon's shape (0 to 1).
type mask func(x, y float64) bool

// render draws a scene at size × size pixels, averaging several samples per
// pixel so edges are smooth.
func render(size int, s scene, m mask) *image.NRGBA {
	ss := 4
	if size >= 512 {
		ss = 3
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := range size {
		for px := range size {
			var r, g, b, a float64
			for sy := range ss {
				for sx := range ss {
					x := (float64(px)+(float64(sx)+0.5)/float64(ss))/float64(size)*2 - 1
					y := 1 - (float64(py)+(float64(sy)+0.5)/float64(ss))/float64(size)*2
					if m != nil && !m(x, y) {
						continue
					}
					cr, cg, cb, ca := s(x, y)
					r, g, b, a = r+cr, g+cg, b+cb, a+ca
				}
			}
			n := float64(ss * ss)
			r, g, b, a = r/n, g/n, b/n, a/n
			if a > 0 {
				r, g, b = r/a, g/a, b/a // un-premultiply for NRGBA
			}
			img.SetNRGBA(px, py, color.NRGBA{to8(r), to8(g), to8(b), to8(a)})
		}
	}
	return img
}

func to8(v float64) uint8 {
	// Gamma-encode for the screen.
	return uint8(math.Round(math.Pow(clamp(v, 0, 1), 1/2.2) * 255))
}

func roundedSquare(side, radius float64) mask {
	h := side
	return func(x, y float64) bool {
		dx := max(math.Abs(x)-(h-radius), 0)
		dy := max(math.Abs(y)-(h-radius), 0)
		return math.Abs(x) <= h && math.Abs(y) <= h && dx*dx+dy*dy <= radius*radius
	}
}

func circle(r float64) mask {
	return func(x, y float64) bool { return x*x+y*y <= r*r }
}

// The scenes. Adaptive icon layers are 108 dp, of which only the middle 72 dp
// is sure to show (66 dp in a circle mask), so the planet sits well inside
// that; the legacy and store icons zoom in so it fills more of the square.

const planetR = 0.5 // in adaptive-layer units

func adaptiveBackground(x, y float64) (r, g, b, a float64) {
	c := space(x, y)
	return c[0], c[1], c[2], 1
}

func adaptiveForeground(x, y float64) (r, g, b, a float64) {
	c, alpha := planet(x/planetR, y/planetR)
	return c[0], c[1], c[2], alpha
}

// adaptiveMonochrome is the shape Android 13+ tints for themed icons: the
// planet, with its barren side fainter than its living side.
func adaptiveMonochrome(x, y float64) (r, g, b, a float64) {
	px, py := x/planetR, y/planetR
	d := px*px + py*py
	if d > 1 {
		return 0, 0, 0, 0
	}
	n := vec{px, py, math.Sqrt(1 - d)}
	a = 0.45 + 0.55*living(n, rotate(n))
	return a, a, a, a
}

func legacyScene(x, y float64) (r, g, b, a float64) {
	const zoom = 0.68 // how much of the adaptive layer the square shows
	x, y = x*zoom, y*zoom
	bg := space(x, y)
	fg, alpha := planet(x/planetR, y/planetR)
	return fg[0] + bg[0]*(1-alpha), fg[1] + bg[1]*(1-alpha), fg[2] + bg[2]*(1-alpha), 1
}

// space is the background: a deep blue gradient with a faint teal haze
// behind the planet, and stars.
func space(x, y float64) rgb {
	t := (y + 1) / 2
	c := mixc(rgb{0.001, 0.002, 0.008}, rgb{0.005, 0.01, 0.03}, t)
	d := math.Sqrt(x*x + y*y)
	c = c.add(rgb{0.0, 0.025, 0.035}.scale(math.Exp(-d * d * 2.5)))
	// Stars: at most one in each cell of a grid, at a random spot.
	const cells = 14
	cx, cy := math.Floor((x+1)*cells/2), math.Floor((y+1)*cells/2)
	if h := hash(int64(cx), int64(cy), 1); h > 0.35 {
		sx := (cx+0.15+0.7*(hash(int64(cx), int64(cy), 2)+1)/2)/cells*2 - 1
		sy := (cy+0.15+0.7*(hash(int64(cx), int64(cy), 3)+1)/2)/cells*2 - 1
		dist := math.Hypot(x-sx, y-sy)
		bright := 0.25 + 0.5*(hash(int64(cx), int64(cy), 4)+1)/2
		c = c.add(rgb{0.8, 0.85, 1}.scale(bright * math.Exp(-dist*dist/(2*0.006*0.006))))
	}
	return c
}

var (
	light    = vec{-0.3, 0.45, 0.84}.norm()
	airColor = rgb{0.25, 0.5, 1.0}
)

// planet is the planet and its glow of air, centred, radius 1; it returns
// the colour premultiplied by the coverage.
func planet(x, y float64) (rgb, float64) {
	d2 := x*x + y*y
	if d2 > 1 {
		// The air's glow just outside the edge, brighter on the lit side.
		d := math.Sqrt(d2)
		t := (d - 1) / 0.16
		if t >= 1 {
			return rgb{}, 0
		}
		lit := 0.35 + 0.65*clamp((x*light.x+y*light.y)/d*0.8+0.5, 0, 1)
		a := (1 - t) * (1 - t) * 0.75 * lit
		return airColor.scale(a), a
	}
	n := vec{x, y, math.Sqrt(1 - d2)}
	p := rotate(n)
	h := fbm(p.scale(2.1), 5) + 0.06
	life := living(n, p)

	// The barren side: rust highlands, darker basins, polar ice.
	barren := mixc(rgb{0.32, 0.17, 0.12}, rgb{0.72, 0.36, 0.18}, smoothstep(-0.25, 0.3, h))
	barren = mixc(barren, rgb{0.55, 0.5, 0.48}, smoothstep(0.35, 0.6, fbm(p.scale(5), 3)+h*0.3)*0.5)
	// The living side: deep and shallow sea, grassland and forest.
	var alive rgb
	if h < 0 {
		alive = mixc(rgb{0.02, 0.09, 0.3}, rgb{0.05, 0.3, 0.5}, smoothstep(-0.35, 0, h))
	} else {
		alive = mixc(rgb{0.22, 0.5, 0.14}, rgb{0.06, 0.26, 0.09}, smoothstep(0.05, 0.4, h))
		alive = mixc(rgb{0.55, 0.62, 0.3}, alive, smoothstep(0, 0.05, h)) // beaches
	}
	c := mixc(barren, alive, life)
	// Ice caps, bigger on the cold barren side.
	capEdge := mix(0.62, 0.8, life) + 0.08*fbm(p.scale(6), 3)
	ice := smoothstep(capEdge, capEdge+0.04, math.Abs(n.y))
	c = mixc(c, rgb{0.85, 0.9, 0.97}, ice)

	// Sunlight, a glint off the sea, and the air thickening at the edge.
	diff := max(n.dot(light), 0)
	c = c.scale(0.06 + 1.05*math.Pow(diff, 0.85))
	if h < 0 && ice < 0.5 {
		half := light.add(vec{0, 0, 1}).norm()
		c = c.add(rgb{1, 1, 1}.scale(0.45 * life * math.Pow(max(n.dot(half), 0), 60)))
	}
	rim := math.Pow(1-n.z, 2.5)
	c = mixc(c, airColor.scale(0.3+0.9*diff), rim*0.85)
	return c, 1
}

// living is how terraformed the planet is at n (p is n rotated into the
// planet's own frame): a wavy front across it, lower left alive.
func living(n, p vec) float64 {
	front := n.dot(vec{-0.9, -0.3, 0.15}.norm()) + 0.08 + 0.2*fbm(p.scale(3.2), 4)
	return smoothstep(-0.06, 0.06, front)
}

// rotate turns a point on the sphere into the planet's frame, so its
// continents don't line up with the picture.
func rotate(n vec) vec {
	const a, b = 0.6, 0.35
	x, z := n.x*math.Cos(a)-n.z*math.Sin(a), n.x*math.Sin(a)+n.z*math.Cos(a)
	y, z2 := n.y*math.Cos(b)-z*math.Sin(b), n.y*math.Sin(b)+z*math.Cos(b)
	return vec{x, y, z2}
}

// writeICNS writes a Mac icon file: a header, then each image as PNG data
// tagged with its type code.
func writeICNS(path string, types map[string]int, imgs map[int]*image.NRGBA) {
	var body bytes.Buffer
	for _, code := range []string{"icp4", "icp5", "icp6", "ic07", "ic08", "ic09", "ic10"} {
		data := encode(imgs[types[code]])
		body.WriteString(code)
		binary.Write(&body, binary.BigEndian, uint32(len(data)+8))
		body.Write(data)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(body.Len()+8))
	out.Write(body.Bytes())
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(path)
}

func write(path string, img *image.NRGBA) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, encode(img), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(path)
}

func encode(img image.Image) []byte {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

// Small maths helpers.

type vec struct{ x, y, z float64 }

func (a vec) add(b vec) vec        { return vec{a.x + b.x, a.y + b.y, a.z + b.z} }
func (a vec) scale(s float64) vec  { return vec{a.x * s, a.y * s, a.z * s} }
func (a vec) dot(b vec) float64    { return a.x*b.x + a.y*b.y + a.z*b.z }
func (a vec) norm() vec            { return a.scale(1 / math.Sqrt(a.dot(a))) }
func (a rgb) add(b rgb) rgb        { return rgb{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a rgb) scale(s float64) rgb  { return rgb{a[0] * s, a[1] * s, a[2] * s} }
func mixc(a, b rgb, t float64) rgb { return a.scale(1 - t).add(b.scale(t)) }

type rgb [3]float64

func mix(a, b, t float64) float64 { return a + (b-a)*t }
func clamp(v, lo, hi float64) float64 {
	return min(max(v, lo), hi)
}

func smoothstep(e0, e1, x float64) float64 {
	t := clamp((x-e0)/(e1-e0), 0, 1)
	return t * t * (3 - 2*t)
}

// hash is a pseudo-random value in [-1, 1] for integer coordinates.
func hash(x, y, z int64) float64 {
	h := uint64(0x2545F4914F6CDD1D)
	for _, v := range [3]int64{x, y, z} {
		h ^= uint64(v)
		h *= 0xBF58476D1CE4E5B9
		h ^= h >> 31
	}
	h *= 0x94D049BB133111EB
	h ^= h >> 29
	return float64(h>>11)/float64(1<<53)*2 - 1
}

// noise is 3D value noise in roughly [-1, 1], and fbm sums octaves of it.
func noise(p vec) float64 {
	x0, y0, z0 := math.Floor(p.x), math.Floor(p.y), math.Floor(p.z)
	fx, fy, fz := smoothstep(0, 1, p.x-x0), smoothstep(0, 1, p.y-y0), smoothstep(0, 1, p.z-z0)
	ix, iy, iz := int64(x0), int64(y0), int64(z0)
	c := func(dx, dy, dz int64) float64 { return hash(ix+dx, iy+dy, iz+dz) }
	return mix(
		mix(mix(c(0, 0, 0), c(1, 0, 0), fx), mix(c(0, 1, 0), c(1, 1, 0), fx), fy),
		mix(mix(c(0, 0, 1), c(1, 0, 1), fx), mix(c(0, 1, 1), c(1, 1, 1), fx), fy),
		fz)
}

func fbm(p vec, octaves int) float64 {
	sum, amp, norm := 0.0, 1.0, 0.0
	for range octaves {
		sum += amp * noise(p)
		norm += amp
		amp *= 0.5
		p = p.scale(2.03)
	}
	return sum / norm
}
