package printer

import (
	"bytes"
	"errors"
	"fmt"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
	"github.com/StirlingMarketingGroup/go-zpl/render"
)

// ErrNoLabel means the job contained no printable ^XA…^XZ block.
var ErrNoLabel = errors.New("no printable ^XA…^XZ label found")

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
		var buf bytes.Buffer
		if err := renderer.RenderPNG(label, &buf); err != nil {
			errs = append(errs, fmt.Errorf("label %d: %w", i+1, err))
			continue
		}
		images = append(images, buf.Bytes())
	}
	return images, errors.Join(errs...)
}
