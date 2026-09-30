package render

import (
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font/opentype"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
)

// Custom fonts get IDs from the Unicode private use plane so they never clash
// with the letter/digit IDs of built-in fonts.
const firstCustomFontID zpl.Font = 0xF0000

var (
	customFontMu  sync.RWMutex
	customFontIDs = map[string]zpl.Font{}
	nextFontID    = firstCustomFontID
)

// RegisterFont makes a TrueType/OpenType font available to ^A@ and ^CW under
// its printer path, e.g. "E:ARIAL.TTF". Registering a name again replaces it.
func RegisterFont(name string, data []byte) error {
	parsed, err := opentype.Parse(data)
	if err != nil {
		return err
	}
	fm, err := getSharedFontManager()
	if err != nil {
		return err
	}

	customFontMu.Lock()
	id := nextFontID
	nextFontID++
	customFontIDs[zpl.NormalizeFontName(name)] = id
	customFontMu.Unlock()

	fm.mu.Lock()
	fm.custom[id] = parsed
	fm.mu.Unlock()
	return nil
}

// lookupFont resolves a printer font path to a registered font. A font stored
// on another drive matches too, as long as the file name is the same.
func lookupFont(name string) (zpl.Font, bool) {
	name = zpl.NormalizeFontName(name)
	customFontMu.RLock()
	defer customFontMu.RUnlock()
	if id, ok := customFontIDs[name]; ok {
		return id, true
	}
	file := name[strings.Index(name, ":")+1:]
	for n, id := range customFontIDs {
		if n[strings.Index(n, ":")+1:] == file {
			return id, true
		}
	}
	return 0, false
}

// fieldAscent returns how far below a ^FO field's top the baseline of a
// downloaded font sits. Printers fit the font's line box (ascent + descent)
// into the field height, so the baseline is at height*ascent/(ascent+descent),
// not at the ascent of the face scaled to the field height.
func (fm *fontManager) fieldAscent(f zpl.Font, height int) (int, bool) {
	if f < firstCustomFontID {
		return 0, false
	}
	face, err := fm.getFace(f, height)
	if err != nil {
		return 0, false
	}
	m := face.Metrics()
	if m.Ascent+m.Descent <= 0 {
		return 0, false
	}
	return int(math.Round(float64(height) * float64(m.Ascent) / float64(m.Ascent+m.Descent))), true
}
