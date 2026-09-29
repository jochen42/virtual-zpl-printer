package printer

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
	"github.com/StirlingMarketingGroup/go-zpl/render"

	"github.com/jochen42/virtual-zpl-printer/internal/pdf"
)

// ErrNoLabel means the job contained no printable ^XA…^XZ block.
var ErrNoLabel = errors.New("no printable ^XA…^XZ label found")

// inkThreshold is the luminance below which an anti-aliased pixel becomes a
// burned dot. Thermal heads print 1-bit with some dot gain, so partially
// covered pixels count as ink.
var inkThreshold uint8 = 170

// Output is a rendered job.
type Output struct {
	Images [][]byte // one 1-bit PNG per label
	PDF    []byte   // vector PDF, one page per label
}

// Render parses every ^XA…^XZ block in data and renders each to a PNG, plus
// a vector PDF with one page per label. Labels that fail to render
// are skipped; their errors are joined.
func Render(data []byte, dpi zpl.DPI) (out Output, err error) {
	labels, err := zpl.ParseAll(string(data))
	if err != nil {
		return out, fmt.Errorf("parsing ZPL: %w", err)
	}
	if len(labels) == 0 {
		return out, ErrNoLabel
	}

	renderer := render.New(dpi).WithIgnoreLabelHome(true)
	var errs []error
	var drawings []*render.Drawing
	for i, label := range labels {
		img, drawing, err := renderer.RenderDrawing(label)
		if err != nil {
			errs = append(errs, fmt.Errorf("label %d: %w", i+1, err))
			continue
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, toThermal(img)); err != nil {
			errs = append(errs, fmt.Errorf("label %d: %w", i+1, err))
			continue
		}
		out.Images = append(out.Images, buf.Bytes())
		drawings = append(drawings, drawing)
	}
	if len(drawings) > 0 {
		var buf bytes.Buffer
		if err := pdf.Write(&buf, drawings, int(dpi)); err != nil {
			errs = append(errs, fmt.Errorf("writing PDF: %w", err))
		} else {
			out.PDF = buf.Bytes()
		}
	}
	return out, errors.Join(errs...)
}

var thermalPalette = color.Palette{color.White, color.Black}

// toThermal converts img to a 1-bit image as a thermal printer would print it.
func toThermal(img image.Image) *image.Paletted {
	b := img.Bounds()
	out := image.NewPaletted(b, thermalPalette)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y < inkThreshold {
				out.SetColorIndex(x, y, 1)
			}
		}
	}
	return out
}
