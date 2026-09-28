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
)

// ErrNoLabel means the job contained no printable ^XA…^XZ block.
var ErrNoLabel = errors.New("no printable ^XA…^XZ label found")

// inkThreshold is the luminance below which an anti-aliased pixel becomes a
// burned dot. Thermal heads print 1-bit with some dot gain, so partially
// covered pixels count as ink.
var inkThreshold uint8 = 170

// Render parses every ^XA…^XZ block in data and renders each to a PNG.
// Labels that fail to render are skipped; their errors are joined.
func Render(data []byte, dpi zpl.DPI) (images [][]byte, err error) {
	labels, err := zpl.ParseAll(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing ZPL: %w", err)
	}
	if len(labels) == 0 {
		return nil, ErrNoLabel
	}

	renderer := render.New(dpi).WithIgnoreLabelHome(true)
	var errs []error
	for i, label := range labels {
		img, err := renderer.Render(label)
		if err != nil {
			errs = append(errs, fmt.Errorf("label %d: %w", i+1, err))
			continue
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, toThermal(img)); err != nil {
			errs = append(errs, fmt.Errorf("label %d: %w", i+1, err))
			continue
		}
		images = append(images, buf.Bytes())
	}
	return images, errors.Join(errs...)
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
