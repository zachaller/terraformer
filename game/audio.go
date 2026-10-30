package game

import (
	"bytes"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// All the game's sound is synthesised here when it starts, so there are no
// audio files: an ambient soundscape (wind and a low drone on a barren
// planet, a warm chord fading in as life spreads) and short effects.

const sampleRate = 44100

type soundID int

const (
	sndClick soundID = iota
	sndBuild
	sndRemove
	sndGoal
	sndAlert
	sndImpact
	numSounds
)

// audioSys plays the game's sound. Its methods do nothing on a nil
// audioSys, so the game runs silently where sound isn't set up.
type audioSys struct {
	ctx            *audio.Context
	sounds         [numSounds][]byte
	barren, living *audio.Player
	// ready is set once synthesis, which runs in the background, is done.
	ready atomic.Bool
}

var (
	audioOnce sync.Once
	audioCtx  *audio.Context
)

// newAudio starts synthesising every sound in the background (it takes a
// moment), then starts the ambient loops, silent until update sets their
// volume. Until then the game is silent.
func newAudio() *audioSys {
	audioOnce.Do(func() { audioCtx = audio.NewContext(sampleRate) })
	a := &audioSys{ctx: audioCtx}
	go func() {
		a.sounds[sndClick] = encode(synthClick())
		a.sounds[sndBuild] = encode(synthSweep(0.35, 300, 110, 0.5))
		a.sounds[sndRemove] = encode(synthSweep(0.3, 180, 70, 0.45))
		a.sounds[sndGoal] = encode(synthChime())
		a.sounds[sndAlert] = encode(synthAlert())
		a.sounds[sndImpact] = encode(synthImpact())
		a.barren = a.loop(synthBarren())
		a.living = a.loop(synthLiving())
		a.ready.Store(true)
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

// update sets the ambient loops' volumes for how alive the planet is
// (lifeCover, 0 to 1), or silences them.
func (a *audioSys) update(lifeCover float64, on bool) {
	if a == nil || !a.ready.Load() || a.barren == nil || a.living == nil {
		return
	}
	living := min(lifeCover*3, 1)
	if !on {
		a.barren.SetVolume(0)
		a.living.SetVolume(0)
		return
	}
	a.barren.SetVolume(0.45 * (1 - 0.6*living))
	a.living.SetVolume(0.35 * living)
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

func synthClick() []float64 {
	s := samples(0.04)
	r := rand.New(rand.NewPCG(1, 2))
	for i := range s {
		t := float64(i) / sampleRate
		s[i] = 0.22*math.Sin(2*math.Pi*1400*t)*math.Exp(-t/0.008) + 0.12*(r.Float64()*2-1)*math.Exp(-t/0.003)
	}
	return s
}

// synthSweep is a soft thud whose pitch falls from f0 to f1 Hz.
func synthSweep(seconds, f0, f1, amp float64) []float64 {
	s := samples(seconds)
	r := rand.New(rand.NewPCG(3, 4))
	phase, noise := 0.0, 0.0
	for i := range s {
		t := float64(i) / sampleRate
		f := f0 + (f1-f0)*t/seconds
		phase += 2 * math.Pi * f / sampleRate
		noise += 0.08 * ((r.Float64()*2 - 1) - noise)
		s[i] = amp * (0.8*math.Sin(phase) + 0.6*noise) * math.Exp(-t/(seconds*0.35))
	}
	return s
}

// synthChime is a rising arpeggio: C, E, G, high C.
func synthChime() []float64 {
	s := samples(1.8)
	for k, f := range []float64{523.25, 659.25, 783.99, 1046.5} {
		start := int(float64(k) * 0.12 * sampleRate)
		for i := start; i < len(s); i++ {
			t := float64(i-start) / sampleRate
			env := math.Exp(-t/0.5) * min(t/0.005, 1)
			s[i] += 0.12 * env * (math.Sin(2*math.Pi*f*t) + 0.3*math.Sin(4*math.Pi*f*t))
		}
	}
	return s
}

func synthAlert() []float64 {
	s := samples(0.3)
	for _, start := range []float64{0, 0.13} {
		for i := int(start * sampleRate); i < len(s); i++ {
			t := float64(i)/sampleRate - start
			if t > 0.08 {
				break
			}
			s[i] += 0.16 * math.Sin(2*math.Pi*880*t) * math.Sin(math.Pi*t/0.08)
		}
	}
	return s
}

// synthImpact is a long, low rumble.
func synthImpact() []float64 {
	s := samples(3.5)
	r := rand.New(rand.NewPCG(5, 6))
	brown := 0.0
	for i := range s {
		t := float64(i) / sampleRate
		brown = 0.995*brown + 0.05*(r.Float64()*2-1)
		s[i] = (0.9*brown + 0.5*math.Sin(2*math.Pi*42*t)) * math.Exp(-t/1.1) * min(t/0.02, 1)
	}
	return s
}

// The ambient loops are loopLen seconds long. Every tone and slow wobble in
// them repeats a whole number of times per loop, and noise is crossfaded
// across the seam, so they loop without a click.
const loopLen = 24.0

// cycles rounds frequency f (Hz) to repeat a whole number of times per loop.
func cycles(f float64) float64 { return math.Round(f*loopLen) / loopLen }

// synthBarren is wind (filtered noise rising and falling, different in each
// ear) over a quiet, low drone.
func synthBarren() (left, right []float64) {
	n := int(loopLen * sampleRate)
	fade := sampleRate // one second crossfade at the seam
	r := rand.New(rand.NewPCG(7, 8))
	wind := func(gustPhase float64) []float64 {
		raw := make([]float64, n+fade)
		lo, lo2 := 0.0, 0.0
		for i := range raw {
			lo += 0.02 * ((r.Float64()*2 - 1) - lo)
			lo2 += 0.05 * (lo - lo2)
			raw[i] = lo2 * 6
		}
		out := make([]float64, n)
		for i := range out {
			v := raw[i]
			if i < fade {
				f := float64(i) / float64(fade)
				v = raw[i]*f + raw[n+i]*(1-f)
			}
			t := float64(i) / sampleRate
			gust := 0.55 + 0.45*math.Sin(2*math.Pi*cycles(1/8.0)*t+gustPhase)
			out[i] = v * gust
		}
		return out
	}
	left, right = wind(0), wind(2)
	f1, f2 := cycles(55), cycles(82.5)
	for i := range left {
		t := float64(i) / sampleRate
		swell := 0.6 + 0.4*math.Sin(2*math.Pi*cycles(1/12.0)*t)
		drone := 0.07 * swell * (math.Sin(2*math.Pi*f1*t) + 0.6*math.Sin(2*math.Pi*f2*t))
		left[i] = 0.5*left[i] + drone
		right[i] = 0.5*right[i] + drone
	}
	return left, right
}

// synthLiving is a soft, slowly breathing chord (A major), with a gentle
// bell now and then.
func synthLiving() (left, right []float64) {
	n := int(loopLen * sampleRate)
	left, right = make([]float64, n), make([]float64, n)
	chord := []float64{cycles(220), cycles(277.18), cycles(329.63), cycles(440)}
	for i := range left {
		t := float64(i) / sampleRate
		for k, f := range chord {
			breathe := 0.55 + 0.45*math.Sin(2*math.Pi*cycles(1/6.0)*t+float64(k))
			v := 0.04 * breathe * (math.Sin(2*math.Pi*f*t) + 0.2*math.Sin(4*math.Pi*f*t))
			pan := float64(k) / float64(len(chord)-1)
			left[i] += v * (1 - 0.5*pan)
			right[i] += v * (0.5 + 0.5*pan)
		}
	}
	// Bells on a pentatonic scale, at fixed times, each ringing out before
	// the loop ends.
	bells := []struct{ at, f float64 }{{2, 880}, {6.5, 659.25}, {11, 739.99}, {15.5, 987.77}, {19, 880}}
	for b, bell := range bells {
		start := int(bell.at * sampleRate)
		for i := start; i < n && i < start+4*sampleRate; i++ {
			t := float64(i-start) / sampleRate
			v := 0.05 * math.Sin(2*math.Pi*bell.f*t) * math.Exp(-t/0.9) * min(t/0.01, 1)
			if b%2 == 0 {
				left[i] += v
				right[i] += v * 0.5
			} else {
				left[i] += v * 0.5
				right[i] += v
			}
		}
	}
	return left, right
}
