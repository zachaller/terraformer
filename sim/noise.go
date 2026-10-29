package sim

import "math"

// noise3 is seeded 3D value noise in roughly [-1, 1].
func noise3(seed uint64, p Vec3) float64 {
	x0, y0, z0 := math.Floor(p.X), math.Floor(p.Y), math.Floor(p.Z)
	fx, fy, fz := smooth(p.X-x0), smooth(p.Y-y0), smooth(p.Z-z0)
	ix, iy, iz := int64(x0), int64(y0), int64(z0)

	corner := func(dx, dy, dz int64) float64 {
		return hash3(seed, ix+dx, iy+dy, iz+dz)
	}
	x00 := lerp(corner(0, 0, 0), corner(1, 0, 0), fx)
	x10 := lerp(corner(0, 1, 0), corner(1, 1, 0), fx)
	x01 := lerp(corner(0, 0, 1), corner(1, 0, 1), fx)
	x11 := lerp(corner(0, 1, 1), corner(1, 1, 1), fx)
	return lerp(lerp(x00, x10, fy), lerp(x01, x11, fy), fz)
}

// fbm sums octaves of noise3 for natural-looking terrain, in roughly [-1, 1].
func fbm(seed uint64, p Vec3, octaves int) float64 {
	sum, amp, norm := 0.0, 1.0, 0.0
	for o := range octaves {
		sum += amp * noise3(seed+uint64(o)*0x9E3779B97F4A7C15, p)
		norm += amp
		amp *= 0.5
		p = p.Scale(2)
	}
	return sum / norm
}

// hash3 maps integer lattice coordinates to a pseudo-random value in [-1, 1].
func hash3(seed uint64, x, y, z int64) float64 {
	h := seed
	for _, v := range [3]int64{x, y, z} {
		h ^= uint64(v)
		h *= 0xBF58476D1CE4E5B9
		h ^= h >> 31
	}
	h *= 0x94D049BB133111EB
	h ^= h >> 29
	return float64(h>>11)/float64(1<<53)*2 - 1
}

func smooth(t float64) float64     { return t * t * (3 - 2*t) }
func lerp(a, b, t float64) float64 { return a + (b-a)*t }
