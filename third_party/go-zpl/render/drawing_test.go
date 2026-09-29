package render

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/vector"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
)

// TestDrawingMatchesBitmap rasterizes the Drawing and compares it with the
// bitmap from the same render. Edges differ a little: the raster anti-aliases
// text (with extra ink for font 0) and steps curves and diagonals by integer
// maths. Misplaced or missing shapes show up as large differences.
func TestDrawingMatchesBitmap(t *testing.T) {
	cases := []struct {
		name, zpl string
		maxDiff   float64 // mismatched pixels / ink pixels
	}{
		{"box", "^XA^PW300^LL200^FO10,10^GB200,100,5^FS^FO30,30^GB50,50,50^FS^FO120,30^GB0,60,4^FS^FO150,30^GB40,0,6^FS^XZ", 0},
		{"reverse", "^XA^PW300^LL200^FO10,10^GB200,100,100^FS^FO50,50^FR^GB200,100,100^FS^XZ", 0},
		{"circle", "^XA^PW300^LL300^FO10,10^GC120,6^FS^FO150,10^GC60,60^FS^FO10,150^GE200,80,5^FS^XZ", 0.1},
		{"diagonal", "^XA^PW300^LL300^FO10,10^GD100,150,5,B,L^FS^FO150,10^GD100,150,3,B,R^FS^XZ", 0.2},
		{"text N", "^XA^PW600^LL200^FO20,20^A0N,50,50^FDHello Wg^FS^FT20,150^A0N,40,25^FDNarrow text^FS^XZ", 0.08},
		{"text R", "^XA^PW300^LL600^FO20,20^A0R,50,50^FDRotated^FS^FT200,20^A0R,40,30^FDBaseline^FS^XZ", 0.08},
		{"text I", "^XA^PW600^LL300^FO20,20^A0I,50,50^FDInverted^FS^FT500,250^A0I,40,30^FDBaseline^FS^XZ", 0.08},
		{"text B", "^XA^PW300^LL600^FO20,20^A0B,50,50^FDBottom^FS^FT200,500^A0B,40,30^FDBaseline^FS^XZ", 0.08},
		{"fonts", "^XA^PW800^LL300^FO10,10^AAN,30,20^FDFont A^FS^FO10,60^ABN,30,20^FDFont B^FS^FO10,110^ACN,30,20^FDFont C^FS^FO10,160^ADN,30,20^FDFont D^FS^FO10,210^AEN,30,20^FDFont E^FS^XZ", 0.1},
		{"field block", "^XA^PW400^LL300^FO10,10^A0N,30,30^FB300,3,0,C^FDA longer text that wraps onto lines^FS^XZ", 0.1},
		{"reverse text", "^XA^PW400^LL200^FO10,10^GB300,80,80^FS^FO20,20^A0N,50,50^FR^FDWhite^FS^XZ", 0.08},
		{"code128", "^XA^PW600^LL300^BY3^FO20,20^BCN,100,Y,N,N^FD12345678^FS^XZ", 0.05},
		{"qr", "^XA^PW400^LL400^FO20,20^BQN,2,5^FDQA,https://example.com^FS^XZ", 0},
		{"datamatrix", "^XA^PW400^LL400^FO20,20^BXN,8,200^FDHello DataMatrix^FS^XZ", 0},
		{"pdf417", "^XA^PW600^LL400^BY2^FO20,20^B7N,6,5,,,N^FDPDF417 test data^FS^XZ", 0},
		{"graphic field", "^XA^PW200^LL200^FO10,10^GFA,32,32,4,FF00FF00F0F0F0F0FF00FF00F0F0F0F0FF00FF00F0F0F0F0FF00FF00F0F0F0F0^FS^XZ", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, err := zpl.Parse(tc.zpl)
			if err != nil {
				t.Fatal(err)
			}
			img, d, err := New(zpl.DPI203).RenderDrawing(label)
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Ops) == 0 {
				t.Fatal("no ops recorded")
			}
			got := rasterize(d)
			ink, diff := 0, 0
			b := img.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					want := color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y < 128
					if want {
						ink++
					}
					if want != (got.GrayAt(x, y).Y < 128) {
						diff++
					}
				}
			}
			ratio := float64(diff) / float64(max(ink, 1))
			t.Logf("ink %d, diff %d (%.2f%%)", ink, diff, 100*ratio)
			if ink == 0 || ratio > tc.maxDiff {
				t.Errorf("drawing differs from bitmap: %d of %d ink pixels (%.2f%%)", diff, ink, 100*ratio)
			}
		})
	}
}

// rasterize paints d 1-bit: a pixel is covered if at least half of it is.
func rasterize(d *Drawing) *image.Gray {
	out := image.NewGray(image.Rect(0, 0, d.Width, d.Height))
	for i := range out.Pix {
		out.Pix[i] = 0xff
	}
	for _, op := range d.Ops {
		covered := func(x, y int) bool { return false }
		switch {
		case op.Mask != nil:
			m := op.Mask
			covered = func(x, y int) bool {
				x, y = x-m.X, y-m.Y
				return x >= 0 && y >= 0 && x < m.Width && y < m.Height && m.Bits[y*m.Stride+x/8]&(0x80>>(x%8)) != 0
			}
		case op.Fill:
			fill := coverage(op.Path, d.Width, d.Height)
			clip := coverage(op.Clip, d.Width, d.Height)
			covered = func(x, y int) bool {
				a := float64(fill.AlphaAt(x, y).A) / 255
				if op.Clip != nil {
					a *= float64(clip.AlphaAt(x, y).A) / 255
				}
				return a >= 0.5
			}
		}
		for y := 0; y < d.Height; y++ {
			for x := 0; x < d.Width; x++ {
				if !covered(x, y) {
					continue
				}
				switch op.Paint {
				case PaintBlack:
					out.Pix[y*out.Stride+x] = 0
				case PaintWhite:
					out.Pix[y*out.Stride+x] = 0xff
				case PaintInvert:
					out.Pix[y*out.Stride+x] ^= 0xff
				}
			}
		}
	}
	return out
}

func coverage(p Path, w, h int) *image.Alpha {
	r := vector.NewRasterizer(w, h)
	for _, s := range p {
		switch s.Op {
		case MoveTo:
			r.MoveTo(float32(s.P[0].X), float32(s.P[0].Y))
		case LineTo:
			r.LineTo(float32(s.P[0].X), float32(s.P[0].Y))
		case QuadTo:
			r.QuadTo(float32(s.P[0].X), float32(s.P[0].Y), float32(s.P[1].X), float32(s.P[1].Y))
		case CubeTo:
			r.CubeTo(float32(s.P[0].X), float32(s.P[0].Y), float32(s.P[1].X), float32(s.P[1].Y), float32(s.P[2].X), float32(s.P[2].Y))
		case Close:
			r.ClosePath()
		}
	}
	out := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(out, out.Bounds(), image.Opaque, image.Point{})
	return out
}
