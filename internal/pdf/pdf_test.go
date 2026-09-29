package pdf

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/StirlingMarketingGroup/go-zpl/render"
)

func TestWrite(t *testing.T) {
	var box render.Path
	box = append(box,
		render.Segment{Op: render.MoveTo, P: [3]render.Point{{X: 10, Y: 10}}},
		render.Segment{Op: render.LineTo, P: [3]render.Point{{X: 50, Y: 10}}},
		render.Segment{Op: render.QuadTo, P: [3]render.Point{{X: 50, Y: 50}, {X: 10, Y: 50}}},
		render.Segment{Op: render.Close},
	)
	// 4x6 inch label at 203 dpi.
	d := &render.Drawing{Width: 812, Height: 1218, Ops: []render.Op{
		{Paint: render.PaintBlack, Path: box, Fill: true},
		{Paint: render.PaintInvert, Path: box, Fill: true},
		{Paint: render.PaintBlack, Mask: &render.Bitmap{X: 5, Y: 6, Width: 9, Height: 2, Stride: 3, Bits: []byte{0xff, 0x80, 0, 0x55, 0, 0}}},
	}}

	var buf bytes.Buffer
	if err := Write(&buf, []*render.Drawing{d, d}, 203); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"%PDF-1.4", "/Count 2", "/MediaBox [0 0 288 432]", "/ImageMask true /Width 9 /Height 2", "/BM /Difference", "%%EOF"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}

	page := inflate(t, out, "<<  /Filter /FlateDecode")
	for _, want := range []string{
		"0.35467980295566504 0 0 -0.35467980295566504 0 432 cm", // dots, Y down
		"10 10 m\n50 10 l\n50 36.667",                           // quad raised to cubic
		"/Inv gs",
		"9 0 0 -2 5 8 cm /M0 Do",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("content missing %q:\n%s", want, page)
		}
	}
}

func TestMaskBitsTrimsStride(t *testing.T) {
	got := maskBits(&render.Bitmap{Width: 9, Height: 2, Stride: 3, Bits: []byte{1, 2, 3, 4, 5, 6}})
	if want := []byte{1, 2, 4, 5}; !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// inflate returns the first stream whose dictionary starts with dict.
func inflate(t *testing.T, pdf, dict string) string {
	t.Helper()
	m := regexp.MustCompile(regexp.QuoteMeta(dict) + ` /Length (\d+) >>\nstream\n`).FindStringIndex(pdf)
	if m == nil {
		t.Fatalf("no stream %q", dict)
	}
	r, err := zlib.NewReader(strings.NewReader(pdf[m[1]:]))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
