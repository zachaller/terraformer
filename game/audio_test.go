package game

import (
	"math"
	"slices"
	"testing"
)

func checkSamples(t *testing.T, name string, ch ...[]float64) {
	t.Helper()
	peak := 0.0
	for _, s := range ch {
		if len(s) == 0 {
			t.Fatalf("%s: no samples", name)
		}
		for _, v := range s {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("%s: not a number", name)
			}
			peak = max(peak, math.Abs(v))
		}
	}
	if peak > 1 {
		t.Errorf("%s: peak %.2f clips", name, peak)
	}
	if peak < 0.05 {
		t.Errorf("%s: peak %.3f is nearly silent", name, peak)
	}
}

func TestEffects(t *testing.T) {
	for id := range numSounds {
		l, r := synthEffect(id)
		checkSamples(t, "effect", l, r)
	}
}

// TestLayersLoop checks every ambient layer is sound and joins up without a
// click: the step from its last sample back to its first is no bigger than
// its ordinary steps from one sample to the next.
func TestLayersLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("synthesising every layer takes a few seconds")
	}
	for l := range numLayers {
		left, right := synthLayer(l)
		checkSamples(t, "layer", left, right)
		for _, s := range [][]float64{left, right} {
			steps := make([]float64, len(s)-1)
			for i := range steps {
				steps[i] = math.Abs(s[i+1] - s[i])
			}
			slices.Sort(steps)
			typical := steps[len(steps)*999/1000]
			if seam := math.Abs(s[0] - s[len(s)-1]); seam > typical*1.5+1e-3 {
				t.Errorf("layer %d: seam step %.4f, typical steps up to %.4f", l, seam, typical)
			}
		}
	}
}
