package render

import (
	"image"
	"image/color"
	"os"
	"testing"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
)

func renderOne(t *testing.T, src string) image.Image {
	t.Helper()
	labels, err := zpl.ParseAll(src)
	if err != nil || len(labels) != 1 {
		t.Fatalf("parse %q: %d labels, %v", src, len(labels), err)
	}
	img, err := New(zpl.DPI203).Render(labels[0])
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func isInk(img image.Image, x, y int) bool {
	return color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y < 128
}

// ^PW, ^LL and ^LH set in a setup format apply to the formats after it.
func TestLayoutCarriesOverFormats(t *testing.T) {
	img := renderOne(t, "^XA^PW400^LL200^LH20,20^XZ^XA^FO0,0^GB10,10,10^FS^XZ")
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 200 {
		t.Fatalf("size %v, want 400x200", b.Size())
	}
	if !isInk(img, 20, 20) || isInk(img, 19, 19) {
		t.Fatal("box should start at the label home 20,20")
	}
}

// The Code 128 interpretation line is font A magnified by the module width,
// centered on the bars, one magnified dot below them.
func TestCode128InterpretationLine(t *testing.T) {
	// Subset B "1": 46 modules, 92 dots at ^BY2. The glyph is 10 dots wide
	// (5 columns x 2), so it starts at 10 + (92-10)/2 = 51; its stem is column 2.
	img := renderOne(t, "^XA^PW200^LL100^BY2^FO10,10^BCN,50,Y,N,N^FD1^FS^XZ")
	top := 10 + 50 + 2
	if !isInk(img, 55, top) || !isInk(img, 56, top+1) {
		t.Error("missing top of the 1's stem")
	}
	if isInk(img, 55, top-1) || isInk(img, 51, top) {
		t.Error("ink outside the glyph")
	}
	if !isInk(img, 53, top+12) || !isInk(img, 57, top+12) {
		t.Error("missing the 1's base (row 6)")
	}
}

// A downloaded font's ^FO baseline sits at height*ascent/(ascent+descent)
// below the field top: 32 dots for DejaVu Sans Mono (1901/2384) at 40 dots.
func TestDownloadedFontFieldOrigin(t *testing.T) {
	data, err := os.ReadFile("dejavu_sans_mono.ttf")
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterFont("E:LAYOUTTEST.TTF", data); err != nil {
		t.Fatal(err)
	}
	fo := renderOne(t, "^XA^PW200^LL100^FO10,10^A@N,40,40,E:LAYOUTTEST.TTF^FDHg^FS^XZ")
	ft := renderOne(t, "^XA^PW200^LL100^FT10,42^A@N,40,40,E:LAYOUTTEST.TTF^FDHg^FS^XZ")
	b := fo.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isInk(fo, x, y) != isInk(ft, x, y) {
				t.Fatalf("^FO10,10 and ^FT10,42 differ at %d,%d", x, y)
			}
		}
	}
}

// A QR code starts at its field origin: printers draw no quiet zone.
func TestQRCodeStartsAtFieldOrigin(t *testing.T) {
	img := renderOne(t, "^XA^PW200^LL200^FO10,10^BQN,2,4^FDQA,test^FS^XZ")
	// The top-left finder pattern's outer ring starts at the origin.
	if !isInk(img, 10, 10) || isInk(img, 9, 9) || isInk(img, 9, 10) {
		t.Fatal("QR code should start at 10,10 without a quiet zone")
	}
}
