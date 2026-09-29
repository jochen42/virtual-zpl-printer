// Package pdf writes rendered labels as a vector PDF, one page per label,
// sized to the physical label so it prints 1:1.
package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/StirlingMarketingGroup/go-zpl/render"
)

// Write encodes drawings as PDF pages. dpi converts dots to the page size.
func Write(w io.Writer, drawings []*render.Drawing, dpi int) error {
	if len(drawings) == 0 {
		return fmt.Errorf("no labels")
	}
	if dpi <= 0 {
		return fmt.Errorf("invalid dpi %d", dpi)
	}

	var d doc
	catalog, pages := d.reserve(), d.reserve()
	var kids []int
	for _, drawing := range drawings {
		kids = append(kids, d.page(drawing, pages, dpi))
	}
	d.set(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	var refs bytes.Buffer
	for _, k := range kids {
		fmt.Fprintf(&refs, " %d 0 R", k)
	}
	d.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%s ] /Count %d >>", refs.String(), len(kids)))
	return d.writeTo(w, catalog)
}

// doc collects numbered PDF objects.
type doc struct{ objs [][]byte }

func (d *doc) reserve() int { d.objs = append(d.objs, nil); return len(d.objs) }

func (d *doc) set(id int, body string) { d.objs[id-1] = []byte(body) }

// stream adds a Flate-compressed stream object.
func (d *doc) stream(dict string, data []byte) int {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(data)
	_ = zw.Close()
	id := d.reserve()
	var b bytes.Buffer
	fmt.Fprintf(&b, "<< %s /Filter /FlateDecode /Length %d >>\nstream\n", dict, z.Len())
	b.Write(z.Bytes())
	b.WriteString("\nendstream")
	d.objs[id-1] = b.Bytes()
	return id
}

func (d *doc) writeTo(w io.Writer, root int) error {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(d.objs))
	for i, body := range d.objs {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", i+1)
		buf.Write(body)
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(d.objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(d.objs)+1, root, xref)
	_, err := w.Write(buf.Bytes())
	return err
}

// page adds one label page and returns its object number.
func (d *doc) page(drawing *render.Drawing, parent, dpi int) int {
	scale := 72 / float64(dpi)
	wPt, hPt := float64(drawing.Width)*scale, float64(drawing.Height)*scale

	var c content
	// Work in dots with Y pointing down, like the renderer.
	c.printf("%s 0 0 %s 0 %s cm\n", exact(scale), exact(-scale), exact(hPt))
	// A white label, so inverted paint has something to invert.
	c.printf("1 g 0 0 %d %d re f\n", drawing.Width, drawing.Height)

	var masks []int
	for _, op := range drawing.Ops {
		c.printf("q\n")
		if op.Clip != nil {
			c.path(op.Clip)
			c.printf("W n\n")
		}
		switch op.Paint {
		case render.PaintBlack:
			c.printf("0 g 0 G\n")
		case render.PaintWhite:
			c.printf("1 g 1 G\n")
		case render.PaintInvert:
			c.printf("/Inv gs 1 g 1 G\n")
		}
		switch {
		case op.Mask != nil:
			m := op.Mask
			id := d.stream(fmt.Sprintf("/Type /XObject /Subtype /Image /ImageMask true /Width %d /Height %d /BitsPerComponent 1 /Decode [1 0]",
				m.Width, m.Height), maskBits(m))
			c.printf("%d 0 0 %d %d %d cm /M%d Do\n", m.Width, -m.Height, m.X, m.Y+m.Height, len(masks))
			masks = append(masks, id)
		case op.Fill && op.Stroke > 0:
			c.path(op.Path)
			c.printf("%s w B\n", num(op.Stroke))
		case op.Fill:
			c.path(op.Path)
			c.printf("f\n")
		case op.Stroke > 0:
			c.path(op.Path)
			c.printf("%s w S\n", num(op.Stroke))
		}
		c.printf("Q\n")
	}

	contents := d.stream("", c.Bytes())
	var xobjects bytes.Buffer
	for i, id := range masks {
		fmt.Fprintf(&xobjects, " /M%d %d 0 R", i, id)
	}
	pageID := d.reserve()
	d.set(pageID, fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Contents %d 0 R "+
		"/Resources << /ExtGState << /Inv << /Type /ExtGState /BM /Difference >> >> /XObject <<%s >> >> >>",
		parent, exact(wPt), exact(hPt), contents, xobjects.String()))
	return pageID
}

// maskBits returns the mask's rows, trimmed to whole bytes per row.
func maskBits(m *render.Bitmap) []byte {
	stride := (m.Width + 7) / 8
	if stride == m.Stride {
		return m.Bits
	}
	out := make([]byte, 0, stride*m.Height)
	for y := 0; y < m.Height; y++ {
		out = append(out, m.Bits[y*m.Stride:y*m.Stride+stride]...)
	}
	return out
}

type content struct{ bytes.Buffer }

func (c *content) printf(format string, args ...any) { fmt.Fprintf(c, format, args...) }

func (c *content) path(p render.Path) {
	var cur, start render.Point
	pt := func(q render.Point) string { return num(q.X) + " " + num(q.Y) }
	for _, s := range p {
		switch s.Op {
		case render.MoveTo:
			cur, start = s.P[0], s.P[0]
			c.printf("%s m\n", pt(cur))
		case render.LineTo:
			cur = s.P[0]
			c.printf("%s l\n", pt(cur))
		case render.QuadTo:
			// Raise to a cubic: controls lie 2/3 of the way to the quad control.
			q, end := s.P[0], s.P[1]
			c1 := render.Point{X: cur.X + 2.0/3*(q.X-cur.X), Y: cur.Y + 2.0/3*(q.Y-cur.Y)}
			c2 := render.Point{X: end.X + 2.0/3*(q.X-end.X), Y: end.Y + 2.0/3*(q.Y-end.Y)}
			cur = end
			c.printf("%s %s %s c\n", pt(c1), pt(c2), pt(end))
		case render.CubeTo:
			cur = s.P[2]
			c.printf("%s %s %s c\n", pt(s.P[0]), pt(s.P[1]), pt(cur))
		case render.Close:
			cur = start
			c.printf("h\n")
		}
	}
}

// exact formats f without rounding, for the page size and scale, where
// rounding would shift shapes across a large label.
func exact(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// num formats f with up to three decimals.
func num(f float64) string {
	s := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(f, 'f', 3, 64), "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}
