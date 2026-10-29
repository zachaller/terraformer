package game

import (
	"bytes"
	_ "embed"
	"math"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Fonts: Sora for UI text, JetBrains Mono for numbers. Both are variable
// fonts, so one file serves every weight. See fonts/README.md.
var (
	//go:embed fonts/Sora.ttf
	soraTTF []byte
	//go:embed fonts/JetBrainsMono.ttf
	monoTTF []byte
)

type fontKind int

const (
	sans fontKind = iota
	mono
)

// Font weights.
const (
	regular  = 400
	semibold = 600
	bold     = 700
)

var fonts struct {
	once       sync.Once
	sans, mono *text.GoTextFaceSource
	mu         sync.Mutex
	faces      map[faceKey]*text.GoTextFace
}

type faceKey struct {
	kind   fontKind
	px     float64
	weight int
}

func loadFonts() {
	var err error
	if fonts.sans, err = text.NewGoTextFaceSource(bytes.NewReader(soraTTF)); err != nil {
		panic(err)
	}
	if fonts.mono, err = text.NewGoTextFaceSource(bytes.NewReader(monoTTF)); err != nil {
		panic(err)
	}
	fonts.faces = map[faceKey]*text.GoTextFace{}
}

// face returns a cached face of the given kind, size in pixels and weight.
func face(kind fontKind, px float64, weight int) *text.GoTextFace {
	fonts.once.Do(loadFonts)
	key := faceKey{kind, math.Round(px*2) / 2, weight}
	fonts.mu.Lock()
	defer fonts.mu.Unlock()
	if f, ok := fonts.faces[key]; ok {
		return f
	}
	src := fonts.sans
	if kind == mono {
		src = fonts.mono
	}
	f := &text.GoTextFace{Source: src, Size: key.px}
	f.SetVariation(text.MustParseTag("wght"), float32(weight))
	fonts.faces[key] = f
	return f
}

// lineHeight is the height of one line of text in a face.
func lineHeight(f *text.GoTextFace) float64 {
	m := f.Metrics()
	return m.HAscent + m.HDescent
}

// wrapWidth breaks s into lines no wider than maxW pixels in face f.
func wrapWidth(s string, f *text.GoTextFace, maxW float64) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		try := word
		if line != "" {
			try = line + " " + word
		}
		if line != "" && text.Advance(try, f) > maxW {
			lines = append(lines, line)
			line = word
			continue
		}
		line = try
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
