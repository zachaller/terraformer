package game

import (
	"bytes"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/zachaller/terraformer/sim"
)

// All the game's sound is synthesised here when it starts, so there are no
// audio files. The ambience is a set of looping layers, each faded in and
// out by what the planet is like (see soundscape): wind that deepens as the
// air thickens, rain, surf, insects, birds, music that warms as life
// spreads, and the hum of the colony when you're zoomed in on it. Effects
// get a little reverb, for a sense of space.

const sampleRate = 44100

type soundID int

const (
	sndClick soundID = iota
	sndBuildPower
	sndBuildIndustry
	sndBuildClimate
	sndBuildAir
	sndBuildProject
	sndRemove
	sndGoal
	sndAlert
	sndImpact
	numSounds
)

// buildSound is the sound for building something in category c.
func buildSound(c sim.Category) soundID {
	switch c {
	case sim.CatPower:
		return sndBuildPower
	case sim.CatClimate:
		return sndBuildClimate
	case sim.CatAtmosphere:
		return sndBuildAir
	case sim.CatProjects:
		return sndBuildProject
	}
	return sndBuildIndustry
}

// The ambient layers.
type layer int

const (
	layWindThin layer = iota // a faint hiss: almost no air to carry sound
	layWind                  // full wind, gusting
	layDrone                 // the barren planet's low drone
	layMusic                 // the warm chords of a living world
	layRain                  // rain
	laySurf                  // waves on a shore
	layInsects               // crickets
	layBirds                 // birdsong
	layHum                   // the colony's machinery, close up
	numLayers
)

// soundscape is how loud each ambient layer should be, 0 to 1.
type soundscape [numLayers]float64

// audioSys plays the game's sound. Its methods do nothing on a nil
// audioSys, so the game runs silently where sound isn't set up.
type audioSys struct {
	ctx    *audio.Context
	sounds [numSounds][]byte
	layers [numLayers]*audio.Player
	vol    soundscape // current volumes, eased towards the targets
	// Synthesis runs in the background: ready is set once the effects are
	// made, and layersReady counts the ambient layers made so far.
	ready       atomic.Bool
	layersReady atomic.Int32
}

var (
	audioOnce sync.Once
	audioCtx  *audio.Context
)

// layerGain is each layer's volume at full strength, to balance the mix.
var layerGain = soundscape{
	layWindThin: 0.35, layWind: 0.45, layDrone: 0.25, layMusic: 0.28, layRain: 0.45,
	laySurf: 0.45, layInsects: 0.32, layBirds: 0.45, layHum: 0.2,
}

// newAudio starts synthesising every sound in the background (it takes a
// moment), then starts the ambient loops, silent until update sets their
// volume. Until then the game is silent.
func newAudio() *audioSys {
	audioOnce.Do(func() { audioCtx = audio.NewContext(sampleRate) })
	a := &audioSys{ctx: audioCtx}
	go func() {
		for id := range numSounds {
			a.sounds[id] = encode(synthEffect(id))
		}
		a.ready.Store(true)
		// Layers start as each is made, in order, so a barren planet's
		// sound comes first.
		for l := range numLayers {
			a.layers[l] = a.loop(synthLayer(l))
			a.layersReady.Add(1)
		}
	}()
	return a
}

func (a *audioSys) loop(left, right []float64) *audio.Player {
	data := encode(left, right)
	stream := audio.NewInfiniteLoop(bytes.NewReader(data), int64(len(data)))
	p, err := a.ctx.NewPlayer(stream)
	if err != nil {
		return nil
	}
	p.SetVolume(0)
	p.Play()
	return p
}

// play plays sound id once.
func (a *audioSys) play(id soundID) {
	if a == nil || !a.ready.Load() {
		return
	}
	a.ctx.NewPlayerFromBytes(a.sounds[id]).Play()
}

// update eases each ambient layer's volume towards the soundscape s (all
// silent when on is false). Called every frame, so layers fade over a few
// seconds rather than jumping.
func (a *audioSys) update(s soundscape, on bool) {
	if a == nil || !a.ready.Load() {
		return
	}
	for l, p := range a.layers[:a.layersReady.Load()] {
		target := 0.0
		if on {
			target = min(max(s[l], 0), 1) * layerGain[l]
		}
		a.vol[l] += (target - a.vol[l]) * 0.02
		if p != nil {
			p.SetVolume(a.vol[l])
		}
	}
}

// soundscape works out how loud each ambient layer should be from the
// planet and the camera.
func (g *Game) soundscape() soundscape {
	p := g.planet
	var s soundscape
	pr := p.Pressure()
	air := pr / (pr + 25) // how well the air carries sound
	life := min(p.Stats().LifeCover*3, 1)
	var plants, animals float64
	sea := 0.0
	for i := range p.Tiles {
		t := &p.Tiles[i]
		plants += t.Bio[sim.Grass] + t.Bio[sim.Shrubs] + t.Bio[sim.Forest]
		animals += t.Bio[sim.Grazers]
		if t.Water > 0.08 {
			sea++
		}
	}
	n := float64(len(p.Tiles))
	plants, animals, sea = plants/n, animals/n, sea/n

	s[layWindThin] = 1 - smoothstep(0.5, 8, pr)
	s[layWind] = smoothstep(1, 30, pr)
	s[layDrone] = 1 - life
	s[layMusic] = life
	// Rain needs liquid water and moist air.
	s[layRain] = min(p.Humidity()*2.5, 1) * min(sea*12, 1) * air * 1.6
	s[laySurf] = min(sea*5, 1) * air * 1.6
	s[layInsects] = min(plants*6, 1) * air * 1.6
	s[layBirds] = min(animals*12, 1)
	// The colony's hum: how many running buildings are near the middle of
	// the screen, louder the closer you're zoomed in.
	centre := g.cam.rot[2]
	near := 0.0
	reach := 0.35 / g.cam.zoom
	for _, b := range p.Buildings() {
		d := p.Dir(p.Grid.CellAt(b.Tile))
		if centre.Dot(d) > math.Cos(reach) {
			near += b.Activity
		}
	}
	s[layHum] = min(near/6, 1) * smoothstep(1.5, 4, g.cam.zoom)
	return s
}

func smoothstep(e0, e1, x float64) float64 {
	t := min(max((x-e0)/(e1-e0), 0), 1)
	return t * t * (3 - 2*t)
}

// encode turns samples in [-1, 1] (mono, or left and right) into 16-bit
// little-endian stereo.
func encode(ch ...[]float64) []byte {
	left, right := ch[0], ch[0]
	if len(ch) > 1 {
		right = ch[1]
	}
	out := make([]byte, len(left)*4)
	for i := range left {
		for c, v := range [2]float64{left[i], right[i]} {
			s := int16(max(min(v, 1), -1) * 32767)
			out[i*4+c*2] = byte(s)
			out[i*4+c*2+1] = byte(s >> 8)
		}
	}
	return out
}

func samples(seconds float64) []float64 { return make([]float64, int(seconds*sampleRate)) }

// ---- Building blocks ----

// biquad is a second-order filter (RBJ cookbook).
type biquad struct{ b0, b1, b2, a1, a2, x1, x2, y1, y2 float64 }

func newBiquad(kind string, freq, q float64) *biquad {
	w := 2 * math.Pi * freq / sampleRate
	cw, alpha := math.Cos(w), math.Sin(w)/(2*q)
	var b0, b1, b2 float64
	switch kind {
	case "low":
		b0, b1, b2 = (1-cw)/2, 1-cw, (1-cw)/2
	case "high":
		b0, b1, b2 = (1+cw)/2, -(1 + cw), (1+cw)/2
	default: // band-pass, peak gain 1
		b0, b1, b2 = alpha, 0, -alpha
	}
	a0 := 1 + alpha
	return &biquad{b0: b0 / a0, b1: b1 / a0, b2: b2 / a0, a1: -2 * cw / a0, a2: (1 - alpha) / a0}
}

func (f *biquad) next(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1, f.y2, f.y1 = f.x1, x, f.y1, y
	return y
}

func filter(s []float64, kind string, freq, q float64) []float64 {
	f := newBiquad(kind, freq, q)
	out := make([]float64, len(s))
	for i, v := range s {
		out[i] = f.next(v)
	}
	return out
}

// reverb returns s with a Schroeder reverb mixed in at wet, as left and
// right with slightly different rooms.
func reverb(s []float64, wet, size float64) (left, right []float64) {
	l, r := room(s, size, 0), room(s, size, 23)
	left, right = make([]float64, len(s)), make([]float64, len(s))
	for i, v := range s {
		left[i] = v*(1-wet) + l[i]*wet
		right[i] = v*(1-wet) + r[i]*wet
	}
	return left, right
}

// room is s reverberating in a room (parallel combs into all-passes) of the
// given size; spread varies it, to give each ear its own.
func room(s []float64, size float64, spread int) []float64 {
	out := make([]float64, len(s))
	for _, d := range []int{1557, 1617, 1491, 1422, 1277, 1356} {
		d = int(float64(d+spread) * size)
		buf := make([]float64, d)
		lp := 0.0
		for i, v := range s {
			y := buf[i%d]
			lp = y*0.7 + lp*0.3 // damping: highs die away first
			buf[i%d] = v + lp*0.82
			out[i] += y / 6
		}
	}
	for _, d := range []int{225, 556, 441} {
		d = int(float64(d+spread/3) * size)
		buf := make([]float64, d)
		for i, v := range out {
			b := buf[i%d]
			buf[i%d] = v + b*0.5
			out[i] = b - v*0.5
		}
	}
	return out
}

// withTail pads s with seconds of silence, so a reverb has room to ring out.
func withTail(s []float64, seconds float64) []float64 {
	return append(s, samples(seconds)...)
}

// loopLen is how long each ambient layer's loop is, in seconds. Loops of
// different lengths drift against each other, so the mix rarely repeats.
var loopLen = [numLayers]float64{
	layWindThin: 12, layWind: 16, layDrone: 16, layMusic: 24, layRain: 10,
	laySurf: 16, layInsects: 10, layBirds: 17, layHum: 8,
}

// cycles rounds frequency f (Hz) to repeat a whole number of times in a
// loop of the given length, so the loop joins without a click.
func cycles(f, length float64) float64 { return math.Round(f*length) / length }

// seamless makes a loop of n samples from gen, which fills a slice with
// sound that doesn't repeat (filtered noise), by crossfading its end into
// its start.
func seamless(n int, gen func([]float64)) []float64 {
	fade := sampleRate
	raw := make([]float64, n+fade)
	gen(raw)
	out := make([]float64, n)
	for i := range out {
		out[i] = raw[i]
		if i < fade {
			f := float64(i) / float64(fade)
			out[i] = raw[i]*f + raw[n+i]*(1-f)
		}
	}
	return out
}

// noise fills s with white noise.
func noise(r *rand.Rand) func([]float64) {
	return func(s []float64) {
		for i := range s {
			s[i] = r.Float64()*2 - 1
		}
	}
}

// filtered fills s with white noise through a filter.
func filtered(r *rand.Rand, kind string, freq, q float64) func([]float64) {
	return func(s []float64) {
		f := newBiquad(kind, freq, q)
		for i := range s {
			s[i] = f.next(r.Float64()*2 - 1)
		}
	}
}

func normalize(peak float64, ch ...[]float64) {
	m := 0.0
	for _, s := range ch {
		for _, v := range s {
			m = max(m, math.Abs(v))
		}
	}
	if m == 0 {
		return
	}
	for _, s := range ch {
		for i := range s {
			s[i] *= peak / m
		}
	}
}

// ---- Ambient layers ----

func synthLayer(l layer) (left, right []float64) {
	length := loopLen[l]
	n := int(length * sampleRate)
	r := rand.New(rand.NewPCG(uint64(l)+11, 7))
	t := func(i int) float64 { return float64(i) / sampleRate }
	switch l {
	case layWindThin:
		// Fine dust hissing past in air too thin to carry much else.
		left = seamless(n, filtered(r, "band", 4500, 0.8))
		right = seamless(n, filtered(r, "band", 5200, 0.8))
		for i := range left {
			flutter := 0.6 + 0.4*math.Sin(2*math.Pi*cycles(0.3, length)*t(i))*math.Sin(2*math.Pi*cycles(0.11, length)*t(i))
			left[i] *= flutter
			right[i] *= 1.2 - flutter
		}
	case layWind:
		// Gusting wind: low roar plus a whistle that rises with each gust.
		for c := range 2 {
			gustF := cycles(1/7.0, length)
			gust := func(i int) float64 { return 0.5 + 0.5*math.Sin(2*math.Pi*gustF*t(i)+float64(c)*1.7) }
			body := seamless(n, filtered(r, "low", 380, 0.7))
			whistle := seamless(n, func(s []float64) {
				bp := newBiquad("band", 600, 6)
				for i := range s {
					if i%64 == 0 { // retune the whistle as the gust rises
						bp = retune(bp, 450+500*gust(i), 8)
					}
					s[i] = bp.next(r.Float64()*2-1) * gust(i) * gust(i)
				}
			})
			out := make([]float64, n)
			for i := range out {
				out[i] = body[i]*3*(0.35+0.65*gust(i)) + whistle[i]*1.5
			}
			if c == 0 {
				left = out
			} else {
				right = out
			}
		}
	case layDrone:
		left, right = samples(length), samples(length)
		f1, f2, f3 := cycles(55, length), cycles(82.5, length), cycles(110.4, length)
		for i := range left {
			x := t(i)
			swell := 0.6 + 0.4*math.Sin(2*math.Pi*cycles(1/8.0, length)*x)
			v := swell * (math.Sin(2*math.Pi*f1*x) + 0.6*math.Sin(2*math.Pi*f2*x) + 0.25*math.Sin(2*math.Pi*f3*x))
			left[i] = v * (1 + 0.1*math.Sin(2*math.Pi*cycles(0.25, length)*x))
			right[i] = v * (1 - 0.1*math.Sin(2*math.Pi*cycles(0.25, length)*x))
		}
	case layMusic:
		left, right = synthMusic(length)
	case layRain:
		// A hiss of rain, and drops pattering on rock and leaves.
		left = seamless(n, filtered(r, "high", 1800, 0.7))
		right = seamless(n, filtered(r, "high", 1800, 0.7))
		for range int(length * 70) {
			at, f := r.IntN(n), 1500+r.Float64()*3500
			pan, amp := r.Float64(), 0.5+r.Float64()*2
			for k := range int(0.03 * sampleRate) {
				v := amp * math.Sin(2*math.Pi*f*t(k)*(1-t(k)*8)) * math.Exp(-t(k)/0.006)
				left[(at+k)%n] += v * (1 - pan)
				right[(at+k)%n] += v * pan
			}
		}
	case laySurf:
		// Two waves to a loop, breaking a little apart in each ear.
		for c := range 2 {
			roar := seamless(n, filtered(r, "low", 700, 0.6))
			foam := seamless(n, filtered(r, "high", 3000, 0.7))
			out := make([]float64, n)
			for i := range out {
				ph := math.Mod(t(i)/(length/2)+float64(c)*0.08, 1)
				swell := math.Pow(math.Sin(math.Pi*ph), 2)
				crash := math.Exp(-math.Pow((ph-0.55)/0.12, 2))
				out[i] = roar[i]*2.5*(0.2+0.8*swell) + foam[i]*0.6*crash
			}
			if c == 0 {
				left = out
			} else {
				right = out
			}
		}
	case layInsects:
		// Two crickets chirping at their own rates, one in each ear, over a
		// faint buzz.
		left, right = samples(length), samples(length)
		for c, cr := range []struct{ f, rate float64 }{{4600, 1.6}, {5100, 2.1}} {
			rate := cycles(cr.rate, length)
			f := cycles(cr.f, length)
			for i := range left {
				x := t(i)
				chirp := math.Mod(x*rate, 1)
				pulses := 0.0
				if chirp < 0.18 { // three quick pulses
					pulses = math.Pow(math.Sin(math.Pi*math.Mod(chirp/0.06, 1)), 2)
				}
				v := math.Sin(2*math.Pi*f*x) * pulses
				if c == 0 {
					left[i] += v
					right[i] += v * 0.3
				} else {
					right[i] += v
					left[i] += v * 0.3
				}
			}
		}
		buzz := seamless(n, filtered(r, "band", 2800, 3))
		for i := range left {
			left[i] += buzz[i] * 0.6
			right[i] += buzz[i] * 0.6
		}
	case layBirds:
		left, right = synthBirds(length, r)
	case layHum:
		// Machinery: a mains hum, a rumble, and a clank every two seconds.
		left, right = samples(length), samples(length)
		rumble := seamless(n, filtered(r, "low", 160, 0.7))
		h := [3]float64{cycles(50, length), cycles(100, length), cycles(150.5, length)}
		for i := range left {
			x := t(i)
			v := 0.5*math.Sin(2*math.Pi*h[0]*x) + 0.3*math.Sin(2*math.Pi*h[1]*x) + 0.15*math.Sin(2*math.Pi*h[2]*x) + rumble[i]*2
			left[i], right[i] = v, v
		}
		clank := withTail(synthMetal(0.6, 330), 0)
		cl, cr := reverb(clank, 0.35, 0.8)
		for _, at := range []float64{0.3, 2.3, 4.3, 6.3} {
			for k := range cl {
				j := (int(at*sampleRate) + k) % n
				left[j] += cl[k] * 0.5
				right[j] += cr[k] * 0.4
			}
		}
	}
	normalize(0.9, left, right)
	return left, right
}

// retune returns a band-pass filter at a new frequency, keeping f's state
// so the sound doesn't click.
func retune(f *biquad, freq, q float64) *biquad {
	g := newBiquad("band", freq, q)
	g.x1, g.x2, g.y1, g.y2 = f.x1, f.x2, f.y1, f.y2
	return g
}

// synthMusic is soft, slowly breathing chords that move through a gentle
// progression (A, F♯m, D, E), with a bell now and then.
func synthMusic(length float64) (left, right []float64) {
	n := int(length * sampleRate)
	left, right = make([]float64, n), make([]float64, n)
	chords := [][]float64{
		{220, 277.18, 329.63, 440},
		{185, 277.18, 369.99, 440},
		{146.83, 220, 293.66, 369.99},
		{164.81, 246.94, 329.63, 415.3},
	}
	seg := length / float64(len(chords))
	for ci, chord := range chords {
		start := float64(ci) * seg
		for k, f := range chord {
			pan := float64(k) / float64(len(chord)-1)
			for i := range int((seg + 3) * sampleRate) {
				x := float64(i) / sampleRate
				// Each chord swells in over two seconds and lingers into
				// the next.
				env := math.Min(x/2, 1) * math.Min(math.Max((seg+3-x)/3, 0), 1)
				v := 0.04 * env * (math.Sin(2*math.Pi*f*x) + 0.25*math.Sin(2*math.Pi*2*f*x) + 0.1*math.Sin(2*math.Pi*3.01*f*x))
				j := (int(start*sampleRate) + i) % n
				left[j] += v * (1 - 0.5*pan)
				right[j] += v * (0.5 + 0.5*pan)
			}
		}
	}
	bells := []struct{ at, f float64 }{{1.5, 880}, {6.8, 739.99}, {12.4, 587.33}, {18.1, 659.25}, {21.5, 830.61}}
	bell := samples(4)
	for b, x := range bells {
		for i := range bell {
			s := float64(i) / sampleRate
			bell[i] = 0.06 * (math.Sin(2*math.Pi*x.f*s) + 0.3*math.Sin(2*math.Pi*2.76*x.f*s)*math.Exp(-s/0.4)) * math.Exp(-s/1.1) * min(s/0.01, 1)
		}
		bl, br := reverb(bell, 0.4, 1.2)
		for i := range bl {
			j := (int(x.at*sampleRate) + i) % n
			if b%2 == 0 {
				left[j] += bl[i]
				right[j] += br[i] * 0.6
			} else {
				left[j] += bl[i] * 0.6
				right[j] += br[i]
			}
		}
	}
	return left, right
}

// synthBirds is a few kinds of bird calling now and then, near and far.
func synthBirds(length float64, r *rand.Rand) (left, right []float64) {
	n := int(length * sampleRate)
	dry := make([]float64, n)
	pans := make([]float64, n)
	type call struct{ f0, f1, dur, vib float64 }
	kinds := [][]call{
		{{2600, 3800, 0.08, 0}, {3800, 2900, 0.07, 0}},                        // a rising-falling chirp
		{{3200, 3200, 0.12, 30}},                                              // a trill
		{{1800, 2500, 0.18, 6}, {2500, 1700, 0.22, 6}},                        // a slow whistle
		{{4200, 5200, 0.04, 0}, {4200, 5200, 0.04, 0}, {4400, 5600, 0.04, 0}}, // tiny peeps
	}
	at := 0.4
	for at < length-1.5 {
		kind := kinds[r.IntN(len(kinds))]
		pan := r.Float64()
		amp := 0.4 + 0.6*r.Float64()
		reps := 1 + r.IntN(4)
		for range reps {
			for _, c := range kind {
				start := int(at * sampleRate)
				ph := 0.0
				for i := range int(c.dur * sampleRate) {
					x := float64(i) / sampleRate
					u := x / c.dur
					f := c.f0 + (c.f1-c.f0)*u + 120*math.Sin(2*math.Pi*c.vib*x)
					ph += 2 * math.Pi * f / sampleRate
					env := math.Sin(math.Pi * u)
					if j := start + i; j < n {
						dry[j] += amp * env * math.Sin(ph)
						pans[j] = pan
					}
				}
				at += c.dur + 0.03
			}
			at += 0.08 + 0.1*r.Float64()
		}
		at += 0.8 + 2.5*r.Float64()
	}
	// Pan each call, then put them all in the open air.
	l, rr := make([]float64, n), make([]float64, n)
	for i, v := range dry {
		l[i], rr[i] = v*(1-pans[i]), v*pans[i]
	}
	wl, wr := room(l, 1.6, 0), room(rr, 1.6, 23)
	for i := range l {
		l[i] = l[i]*0.7 + wl[i]*0.3
		rr[i] = rr[i]*0.7 + wr[i]*0.3
	}
	return l, rr
}

// ---- Effects ----

func synthEffect(id soundID) (left, right []float64) {
	var dry []float64
	wet, size := 0.25, 1.0
	switch id {
	case sndClick:
		// A soft, short tick.
		dry = samples(0.05)
		for i := range dry {
			x := float64(i) / sampleRate
			dry[i] = 0.18 * math.Sin(2*math.Pi*1700*x) * math.Exp(-x/0.006) * min(x/0.0008, 1)
		}
		wet = 0.1
	case sndBuildPower:
		// Power coming on: a rising electric hum, crackle, then a click.
		dry = samples(0.7)
		r := rand.New(rand.NewPCG(21, 1))
		ph := 0.0
		for i := range dry {
			x := float64(i) / sampleRate
			f := 90 + 140*min(x/0.45, 1)
			ph += 2 * math.Pi * f / sampleRate
			buzz := math.Sin(ph) + 0.5*math.Sin(2*ph) + 0.33*math.Sin(3*ph) + 0.25*math.Sin(4*ph)
			env := min(x/0.05, 1) * math.Exp(-math.Max(x-0.45, 0)/0.08)
			spark := 0.0
			if r.Float64() < 0.004 {
				spark = r.Float64()*2 - 1
			}
			dry[i] = 0.12*buzz*env + 0.5*spark*(1-x/0.7)
		}
		addAt(dry, synthClickHard(), 0.45, 0.5)
	case sndBuildIndustry:
		// A heavy thud, and metal ringing.
		dry = synthThud(0.45, 140, 45, 0.6)
		addAt(dry, synthMetal(0.9, 520), 0.02, 0.35)
	case sndBuildClimate:
		// A warm whoosh rising into a low swell.
		r := rand.New(rand.NewPCG(31, 1))
		dry = samples(1.0)
		bp := newBiquad("band", 400, 1.5)
		for i := range dry {
			x := float64(i) / sampleRate
			if i%64 == 0 {
				bp = retune(bp, 300+1500*x, 1.5)
			}
			env := math.Sin(math.Pi * min(x/0.9, 1))
			dry[i] = 0.5*bp.next(r.Float64()*2-1)*env + 0.15*math.Sin(2*math.Pi*(110+40*x)*x)*env
		}
	case sndBuildAir:
		// A hiss of gas, and bubbling.
		r := rand.New(rand.NewPCG(41, 1))
		dry = samples(0.9)
		hp := newBiquad("high", 2500, 0.7)
		for i := range dry {
			x := float64(i) / sampleRate
			env := min(x/0.04, 1) * math.Exp(-x/0.35)
			dry[i] = 0.35 * hp.next(r.Float64()*2-1) * env
		}
		for k := range 6 {
			addAt(dry, synthBubble(300+float64(k)*90), 0.08+float64(k)*0.09, 0.25)
		}
	case sndBuildProject:
		// Something big: a deep boom and a shimmer of rising notes.
		dry = synthThud(1.2, 90, 30, 0.7)
		for k, f := range []float64{392, 523.25, 659.25, 783.99, 1046.5} {
			tone := samples(1.4)
			for i := range tone {
				x := float64(i) / sampleRate
				tone[i] = math.Sin(2*math.Pi*f*x) * math.Exp(-x/0.5) * min(x/0.01, 1)
			}
			addAt(dry, tone, 0.1+float64(k)*0.07, 0.07)
		}
		wet, size = 0.4, 1.4
	case sndRemove:
		// Crunching apart: crackles, a falling tone and a thud.
		r := rand.New(rand.NewPCG(51, 1))
		dry = synthThud(0.4, 120, 50, 0.4)
		for range 40 {
			at := r.Float64() * 0.35
			grain := samples(0.012)
			for i := range grain {
				grain[i] = (r.Float64()*2 - 1) * math.Exp(-float64(i)/sampleRate/0.002)
			}
			addAt(dry, filter(grain, "band", 1500+r.Float64()*2500, 2), at, 0.3*(1-at/0.4))
		}
		addAt(dry, synthMetal(0.4, 260), 0.05, 0.15)
	case sndGoal:
		// A rising arpeggio over a soft chord.
		dry = samples(2.2)
		for k, f := range []float64{523.25, 659.25, 783.99, 1046.5} {
			start := int(float64(k) * 0.12 * sampleRate)
			for i := start; i < len(dry); i++ {
				x := float64(i-start) / sampleRate
				env := math.Exp(-x/0.6) * min(x/0.005, 1)
				dry[i] += 0.12 * env * (math.Sin(2*math.Pi*f*x) + 0.3*math.Sin(4*math.Pi*f*x))
			}
		}
		for _, f := range []float64{261.63, 329.63, 392} {
			for i := range dry {
				x := float64(i) / sampleRate
				dry[i] += 0.035 * math.Sin(2*math.Pi*f*x) * math.Sin(math.Pi*min(x/2.2, 1))
			}
		}
		wet, size = 0.35, 1.3
	case sndAlert:
		// Two soft falling tones.
		dry = samples(0.45)
		for k, f := range []float64{659.25, 523.25} {
			start := int(float64(k) * 0.16 * sampleRate)
			for i := range int(0.14 * sampleRate) {
				x := float64(i) / sampleRate
				env := math.Sin(math.Pi * x / 0.14)
				dry[start+i] += 0.15 * env * (math.Sin(2*math.Pi*f*x) + 0.2*math.Sin(4*math.Pi*f*x))
			}
		}
		wet = 0.3
	case sndImpact:
		// A long, low rumble with debris pattering down.
		r := rand.New(rand.NewPCG(61, 1))
		dry = synthThud(3.5, 70, 25, 0.9)
		lp := newBiquad("low", 300, 0.7)
		for i := range dry {
			x := float64(i) / sampleRate
			dry[i] += 0.9 * lp.next(r.Float64()*2-1) * math.Exp(-x/1.1) * min(x/0.02, 1) * 3
		}
		for range 60 {
			at := 0.3 + r.Float64()*2.5
			grain := samples(0.01)
			for i := range grain {
				grain[i] = (r.Float64()*2 - 1) * math.Exp(-float64(i)/sampleRate/0.002)
			}
			addAt(dry, grain, at, 0.15*(1-at/3))
		}
		wet, size = 0.35, 1.6
	}
	// Fade the very end, so a sound cut short doesn't click.
	fade := min(int(0.04*sampleRate), len(dry))
	for i := range fade {
		dry[len(dry)-1-i] *= float64(i) / float64(fade)
	}
	left, right = reverb(withTail(dry, 0.6*size), wet, size)
	normalize(effectPeak[id], left, right)
	return left, right
}

// effectPeak is each effect's loudness, to balance them against each other.
var effectPeak = [numSounds]float64{
	sndClick: 0.25, sndBuildPower: 0.55, sndBuildIndustry: 0.6, sndBuildClimate: 0.5,
	sndBuildAir: 0.5, sndBuildProject: 0.7, sndRemove: 0.55, sndGoal: 0.6, sndAlert: 0.45, sndImpact: 0.9,
}

// addAt mixes src into dst at the given time, scaled by gain.
func addAt(dst, src []float64, at, gain float64) {
	start := int(at * sampleRate)
	for i, v := range src {
		if j := start + i; j < len(dst) {
			dst[j] += v * gain
		}
	}
}

// synthThud is a low thump whose pitch falls from f0 to f1 Hz.
func synthThud(seconds, f0, f1, amp float64) []float64 {
	s := samples(seconds)
	r := rand.New(rand.NewPCG(3, 4))
	lp := newBiquad("low", 400, 0.7)
	ph := 0.0
	for i := range s {
		x := float64(i) / sampleRate
		f := f1 + (f0-f1)*math.Exp(-x/(seconds*0.25))
		ph += 2 * math.Pi * f / sampleRate
		env := math.Exp(-x/(seconds*0.3)) * min(x/0.004, 1)
		s[i] = amp * (math.Sin(ph) + 0.6*lp.next(r.Float64()*2-1)*3) * env
	}
	return s
}

// synthMetal is metal ringing: inharmonic partials of base frequency f.
func synthMetal(seconds, f float64) []float64 {
	s := samples(seconds)
	for i := range s {
		x := float64(i) / sampleRate
		for k, m := range []float64{1, 2.52, 4.21, 6.73} {
			s[i] += math.Sin(2*math.Pi*f*m*x) * math.Exp(-x/(0.25/float64(k+1))) / float64(k+1)
		}
		s[i] *= 0.4 * min(x/0.001, 1)
	}
	return s
}

// synthClickHard is a switch closing.
func synthClickHard() []float64 {
	s := samples(0.03)
	r := rand.New(rand.NewPCG(9, 9))
	for i := range s {
		x := float64(i) / sampleRate
		s[i] = (0.6*math.Sin(2*math.Pi*2200*x) + 0.4*(r.Float64()*2-1)) * math.Exp(-x/0.004)
	}
	return s
}

// synthBubble is a bubble rising and popping: a quick upward chirp.
func synthBubble(f float64) []float64 {
	s := samples(0.06)
	ph := 0.0
	for i := range s {
		x := float64(i) / sampleRate
		ph += 2 * math.Pi * f * (1 + x*25) / sampleRate
		s[i] = math.Sin(ph) * math.Exp(-x/0.02)
	}
	return s
}
