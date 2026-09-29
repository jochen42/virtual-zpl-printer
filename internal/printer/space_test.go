package printer

import (
	"bytes"
	"testing"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
)

func TestMissingSpaceGlyphRendersAsSpace(t *testing.T) {
	render := func(fd string) []byte {
		out, err := Render([]byte("^XA^PW300^LL80^CI28^FO10,10^AEN,30,30^FH_^FD"+fd+"^FS^XZ"), zpl.DPI203)
		if err != nil || len(out.Images) != 1 {
			t.Fatalf("render %q: %v", fd, err)
		}
		return out.Images[0]
	}
	// U+202F narrow no-break space, which font E (OCR-B) has no glyph for.
	if !bytes.Equal(render("100_E2_80_AFkg"), render("100 kg")) {
		t.Fatal("narrow no-break space should render like a plain space")
	}
}
