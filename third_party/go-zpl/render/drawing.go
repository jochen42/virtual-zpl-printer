package render

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
	"github.com/StirlingMarketingGroup/go-zpl/internal/maxicode"
)

// Drawing is the vector form of a rendered label: the same shapes as the
// bitmap from Render, as paths in dots with the Y axis pointing down. Ops are
// painted in order onto a white label.
type Drawing struct {
	Width, Height int
	Ops           []Op
}

// Paint says how an Op marks the label.
type Paint int

const (
	PaintBlack Paint = iota
	PaintWhite
	// PaintInvert flips black and white underneath (^FR).
	PaintInvert
)

// Op is one painting operation. Exactly one of Path or Mask is set.
type Op struct {
	Paint Paint
	// Path is filled with the non-zero winding rule when Fill is set, and
	// stroked with line width Stroke when Stroke > 0.
	Path   Path
	Fill   bool
	Stroke float64
	// Clip, if set, limits the op to this path.
	Clip Path
	// Mask paints its set pixels.
	Mask *Bitmap
}

// Bitmap is a 1-bit image placed at X, Y in dots. Rows are Stride bytes,
// most significant bit first; a set bit marks the pixel.
type Bitmap struct {
	X, Y, Width, Height, Stride int
	Bits                        []byte
}

// Path is a sequence of subpaths.
type Path []Segment

type SegmentOp int

const (
	MoveTo SegmentOp = iota
	LineTo
	QuadTo // P[0] control, P[1] end
	CubeTo // P[0], P[1] controls, P[2] end
	Close
)

type Segment struct {
	Op SegmentOp
	P  [3]Point
}

type Point struct{ X, Y float64 }

func (p *Path) moveTo(x, y float64) { *p = append(*p, Segment{Op: MoveTo, P: [3]Point{{x, y}}}) }
func (p *Path) lineTo(x, y float64) { *p = append(*p, Segment{Op: LineTo, P: [3]Point{{x, y}}}) }
func (p *Path) close()              { *p = append(*p, Segment{Op: Close}) }

func (p *Path) rect(x, y, w, h float64) {
	p.moveTo(x, y)
	p.lineTo(x+w, y)
	p.lineTo(x+w, y+h)
	p.lineTo(x, y+h)
	p.close()
}

// ellipse adds an ellipse from four cubic Béziers, clockwise on the label
// (Y down). A negative ry runs counter-clockwise, which cuts a hole.
func (p *Path) ellipse(cx, cy, rx, ry float64) {
	const k = 0.5522847498 // 4/3·(√2−1)
	kx, ky := k*rx, k*ry
	p.moveTo(cx+rx, cy)
	*p = append(*p,
		Segment{Op: CubeTo, P: [3]Point{{cx + rx, cy + ky}, {cx + kx, cy + ry}, {cx, cy + ry}}},
		Segment{Op: CubeTo, P: [3]Point{{cx - kx, cy + ry}, {cx - rx, cy + ky}, {cx - rx, cy}}},
		Segment{Op: CubeTo, P: [3]Point{{cx - rx, cy - ky}, {cx - kx, cy - ry}, {cx, cy - ry}}},
		Segment{Op: CubeTo, P: [3]Point{{cx + kx, cy - ry}, {cx + rx, cy - ky}, {cx + rx, cy}}},
	)
	p.close()
}

func (p *Path) polygon(pts ...Point) {
	for i, pt := range pts {
		if i == 0 {
			p.moveTo(pt.X, pt.Y)
		} else {
			p.lineTo(pt.X, pt.Y)
		}
	}
	p.close()
}

// affine maps (x, y) to (A·x + C·y + E, B·x + D·y + F).
type affine struct{ A, B, C, D, E, F float64 }

func (m affine) apply(x, y float64) Point {
	return Point{m.A*x + m.C*y + m.E, m.B*x + m.D*y + m.F}
}

func (m affine) path(p Path) Path {
	out := make(Path, len(p))
	for i, s := range p {
		out[i].Op = s.Op
		for j, pt := range s.P {
			out[i].P[j] = m.apply(pt.X, pt.Y)
		}
	}
	return out
}

// recorder collects the Drawing while a canvas renders.
type recorder struct{ d Drawing }

func (c *canvas) record(op Op) {
	if c.vec != nil && (len(op.Path) > 0 || op.Mask != nil) {
		c.vec.d.Ops = append(c.vec.d.Ops, op)
	}
}

// shapePaint is the paint of a graphic drawn with setPixel.
func (c *canvas) shapePaint(isWhite bool) Paint {
	switch {
	case c.fieldReverse:
		return PaintInvert
	case isWhite:
		return PaintWhite
	default:
		return PaintBlack
	}
}

func (c *canvas) recordRects(paint Paint, rects ...image.Rectangle) {
	if c.vec == nil {
		return
	}
	var p Path
	for _, r := range rects {
		if !r.Empty() {
			p.rect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
		}
	}
	c.record(Op{Paint: paint, Path: p, Fill: true})
}

func (c *canvas) recordBox(box *zpl.GraphicBox) {
	if c.vec == nil {
		return
	}
	x, y, w, h, t := c.curX, c.curY, box.Width, box.Height, box.Thickness
	paint := c.shapePaint(box.Color == zpl.LineColorWhite)
	switch {
	case w == 0:
		c.recordRects(paint, image.Rect(x, y, x+t, y+h))
	case h == 0:
		c.recordRects(paint, image.Rect(x, y, x+w, y+t))
	case t >= w/2 || t >= h/2:
		c.recordRects(paint, image.Rect(x, y, x+w, y+h))
	default:
		c.recordRects(paint,
			image.Rect(x, y, x+w, y+t),
			image.Rect(x, y+h-t, x+w, y+h),
			image.Rect(x, y+t, x+t, y+h-t),
			image.Rect(x+w-t, y+t, x+w, y+h-t),
		)
	}
}

// recordRing records an elliptical ring centred on pixel (cx, cy). The
// raster tests pixel offsets against the radii; ringSlack widens the ring so
// pixels on its edge stay covered.
func (c *canvas) recordRing(paint Paint, cx, cy int, rx, ry, innerRx, innerRy float64) {
	if c.vec == nil {
		return
	}
	var p Path
	x, y := float64(cx)+0.5, float64(cy)+0.5
	p.ellipse(x, y, rx+ringSlack, ry+ringSlack)
	if innerRx > ringSlack && innerRy > ringSlack {
		p.ellipse(x, y, innerRx-ringSlack, -(innerRy - ringSlack))
	}
	c.record(Op{Paint: paint, Path: p, Fill: true})
}

const ringSlack = 0.3

func (c *canvas) recordCircle(circle *zpl.GraphicCircle) {
	r := circle.Diameter / 2
	inner := max(r-circle.Thickness, 0)
	c.recordRing(c.shapePaint(circle.Color == zpl.LineColorWhite),
		c.curX+r, c.curY+r, float64(r), float64(r), float64(inner), float64(inner))
}

func (c *canvas) recordEllipse(e *zpl.GraphicEllipse) {
	rx, ry := e.Width/2, e.Height/2
	innerRx, innerRy := max(rx-e.Thickness, 1), max(ry-e.Thickness, 1)
	c.recordRing(c.shapePaint(e.Color == zpl.LineColorWhite),
		c.curX+rx, c.curY+ry, float64(rx), float64(ry), float64(innerRx), float64(innerRy))
}

func (c *canvas) recordDiagonalLine(line *zpl.GraphicDiagonalLine) {
	if c.vec == nil || line.Height <= 0 {
		return
	}
	x, y, w, h := float64(c.curX), float64(c.curY), float64(line.Width), float64(line.Height)
	half := float64(line.Thickness / 2)
	span := 2*half + 1
	// Row dy covers x from lineX-half to lineX+half+1, with lineX taken at the
	// row's centre, half a row below its top.
	slope := w / h
	top, bottom := x-half-slope/2, x+w-half-slope/2
	if line.Orientation != zpl.DiagonalRightLeaning {
		top, bottom = x+w-1-half+slope/2, x-1-half+slope/2
	}
	var p Path
	p.polygon(Point{top, y}, Point{top + span, y}, Point{bottom + span, y + h}, Point{bottom, y + h})
	c.record(Op{Paint: c.shapePaint(line.Color == zpl.LineColorWhite), Path: p, Fill: true})
}

// recordImage records an image drawn with draw.Over at (x, y) as rectangles:
// dark pixels black, light opaque pixels white. Barcode images are module
// aligned, so rows of equal runs merge into few rectangles.
func (c *canvas) recordImage(img image.Image, x, y int) {
	if c.vec == nil {
		return
	}
	type run struct {
		x0, x1 int
		black  bool
	}
	b := img.Bounds()
	open := map[run]int{} // run → first row
	var black, white []image.Rectangle
	flush := func(r run, y0, y1 int) {
		rect := image.Rect(x+r.x0-b.Min.X, y+y0-b.Min.Y, x+r.x1-b.Min.X, y+y1-b.Min.Y)
		if r.black {
			black = append(black, rect)
		} else {
			white = append(white, rect)
		}
	}
	for py := b.Min.Y; py <= b.Max.Y; py++ {
		var runs []run
		if py < b.Max.Y {
			for px := b.Min.X; px < b.Max.X; {
				col := color.NRGBAModel.Convert(img.At(px, py)).(color.NRGBA)
				if col.A == 0 {
					px++
					continue
				}
				isBlack := color.GrayModel.Convert(col).(color.Gray).Y < 128
				end := px + 1
				for end < b.Max.X {
					next := color.NRGBAModel.Convert(img.At(end, py)).(color.NRGBA)
					if next.A == 0 || (color.GrayModel.Convert(next).(color.Gray).Y < 128) != isBlack {
						break
					}
					end++
				}
				runs = append(runs, run{px, end, isBlack})
				px = end
			}
		}
		seen := map[run]bool{}
		for _, r := range runs {
			seen[r] = true
			if _, ok := open[r]; !ok {
				open[r] = py
			}
		}
		for r, y0 := range open {
			if !seen[r] {
				flush(r, y0, py)
				delete(open, r)
			}
		}
	}
	c.recordRects(PaintWhite, white...)
	c.recordRects(PaintBlack, black...)
}

// recordGraphicField records ^GF data as a bitmap mask.
func (c *canvas) recordGraphicField(gf *zpl.GraphicField) {
	if c.vec == nil || gf.BytesPerRow <= 0 {
		return
	}
	var bits []byte
	switch gf.Format {
	case zpl.GraphicFieldBinary:
		bits = append(bits, gf.BinaryData...)
	case zpl.GraphicFieldASCII:
		var nibbles []byte
		for i := 0; i < len(gf.Data); i++ {
			ch := gf.Data[i]
			switch {
			case ch >= '0' && ch <= '9':
				nibbles = append(nibbles, ch-'0')
			case ch >= 'A' && ch <= 'F':
				nibbles = append(nibbles, ch-'A'+10)
			case ch >= 'a' && ch <= 'f':
				nibbles = append(nibbles, ch-'a'+10)
			}
		}
		for i := 0; i < len(nibbles); i += 2 {
			v := nibbles[i] << 4
			if i+1 < len(nibbles) {
				v |= nibbles[i+1]
			}
			bits = append(bits, v)
		}
	}
	rows := (len(bits) + gf.BytesPerRow - 1) / gf.BytesPerRow
	if rows == 0 {
		return
	}
	bits = append(bits, make([]byte, rows*gf.BytesPerRow-len(bits))...)
	c.record(Op{Paint: PaintBlack, Mask: &Bitmap{
		X: c.curX, Y: c.curY, Width: gf.BytesPerRow * 8, Height: rows, Stride: gf.BytesPerRow, Bits: bits,
	}})
}

// textOp builds the glyph outlines of a text field, laid out exactly like
// fontManager.drawText.
func (fm *fontManager) textOp(text string, x, y int, f zpl.Font, height, width int, orient zpl.Orientation, reverse, useBaseline bool) (Op, bool) {
	face, err := fm.getFace(f, height)
	if err != nil || text == "" {
		return Op{}, false
	}
	text = substituteMissingSpaces(face, text)
	cjkFace, _ := fm.getCJKFace(height)

	scaleX := 1.0
	if width != 0 {
		scaleX = float64(width) / float64(height)
	}
	scaleX *= fontWidthScale(f, height)
	boldness := fontBoldness(f, height)
	baselineAdjust := fontBaselineAdjust(f, height)

	mainFont, mainSize := fm.getFont(f), float64(height)*fontScale(f, height)+fontSizeAdjust(f, height)

	// Glyphs in the text box: the box's top-left is (0, 0), the baseline is
	// at ascent+baselineAdjust.
	metrics := face.Metrics()
	maxAscent, maxDescent := metrics.Ascent, metrics.Descent
	var glyphs Path
	var pen float64
	var totalWidth int64
	type placed struct {
		r    rune
		cjk  bool
		penX float64
	}
	var placedGlyphs []placed
	for _, r := range text {
		cur, isCJK := face, false
		if !hasGlyph(face, r) && cjkFace != nil && hasGlyph(cjkFace, r) {
			cur, isCJK = cjkFace, true
		}
		bounds, adv, ok := cur.GlyphBounds(r)
		if ok {
			totalWidth += int64(adv)
			if -bounds.Min.Y > maxAscent {
				maxAscent = -bounds.Min.Y
			}
			if bounds.Max.Y > maxDescent {
				maxDescent = bounds.Max.Y
			}
		}
		if ok {
			placedGlyphs = append(placedGlyphs, placed{r, isCJK, pen})
		}
		a, _ := cur.GlyphAdvance(r)
		pen += float64(a) / 64
	}
	ascent := maxAscent.Round()
	textHeight := (maxAscent + maxDescent).Round()
	naturalWidth := int((totalWidth + 32) >> 6)
	if naturalWidth == 0 || textHeight == 0 {
		return Op{}, false
	}

	baseline := float64(ascent + baselineAdjust)
	for _, g := range placedGlyphs {
		otf, size := mainFont, mainSize
		if g.cjk {
			otf, size = fm.fontCJK, float64(height)
		}
		for b := 0; b <= boldness; b++ {
			glyphOutline(&glyphs, otf, size, g.r, g.penX+float64(b), baseline)
		}
	}

	// Map the text box onto the label; see the drawText* functions.
	scaledWidth := naturalWidth
	if scaleX != 1.0 {
		scaledWidth = max(int(math.Round(float64(naturalWidth)*scaleX)), 1)
	}
	s := float64(scaledWidth) / float64(naturalWidth)
	fx, fy, asc, th, sw := float64(x), float64(y), float64(ascent), float64(textHeight), float64(scaledWidth)
	clip := true
	var m affine
	switch orient {
	case zpl.OrientationRotated90:
		if useBaseline {
			m = affine{0, s, -1, 0, fx + asc + 1, fy}
		} else {
			m = affine{0, s, -1, 0, fx + th, fy}
		}
	case zpl.OrientationRotated180:
		if useBaseline {
			m = affine{-s, 0, 0, -1, fx + 1, fy + asc + 1}
		} else {
			m = affine{-s, 0, 0, -1, fx + sw, fy + th}
		}
	case zpl.OrientationRotated270:
		if useBaseline {
			m = affine{0, -s, 1, 0, fx - asc, fy - 1}
		} else {
			m = affine{0, -s, 1, 0, fx, fy + sw}
		}
	default:
		top := fy
		if useBaseline {
			top -= asc
		}
		m = affine{s, 0, 0, 1, fx, top}
		// Unscaled normal text is drawn straight onto the label, unclipped.
		clip = scaleX != 1.0
	}

	op := Op{Paint: PaintBlack, Path: m.path(glyphs), Fill: true}
	if reverse {
		op.Paint = PaintWhite
	}
	if clip {
		var box Path
		box.rect(0, 0, float64(naturalWidth), th)
		op.Clip = m.path(box)
	}
	return op, true
}

// glyphOutline appends rune r's outline with its origin at (x, y).
func glyphOutline(p *Path, f *sfnt.Font, size float64, r rune, x, y float64) {
	var buf sfnt.Buffer
	idx, err := f.GlyphIndex(&buf, r)
	if err != nil {
		return
	}
	segs, err := f.LoadGlyph(&buf, idx, fixed.Int26_6(0.5+size*64), nil)
	if err != nil {
		return
	}
	pt := func(v fixed.Point26_6) Point { return Point{x + float64(v.X)/64, y + float64(v.Y)/64} }
	started := false
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if started {
				p.close()
			}
			started = true
			*p = append(*p, Segment{Op: MoveTo, P: [3]Point{pt(s.Args[0])}})
		case sfnt.SegmentOpLineTo:
			*p = append(*p, Segment{Op: LineTo, P: [3]Point{pt(s.Args[0])}})
		case sfnt.SegmentOpQuadTo:
			*p = append(*p, Segment{Op: QuadTo, P: [3]Point{pt(s.Args[0]), pt(s.Args[1])}})
		case sfnt.SegmentOpCubeTo:
			*p = append(*p, Segment{Op: CubeTo, P: [3]Point{pt(s.Args[0]), pt(s.Args[1]), pt(s.Args[2])}})
		}
	}
	if started {
		p.close()
	}
}

// text draws a text field and records its outlines.
func (c *canvas) text(text string, x, y int, f zpl.Font, height, width int, orient zpl.Orientation, reverse, useBaseline bool) {
	c.fontMgr.drawText(c.img, text, x, y, f, height, width, orient, reverse, useBaseline)
	if c.vec != nil {
		if op, ok := c.fontMgr.textOp(text, x, y, f, height, width, orient, reverse, useBaseline); ok {
			c.record(op)
		}
	}
}

func (c *canvas) recordMaxiCode(grid *maxicode.SymbolGrid, multiplier float64, x, y int) {
	if c.vec == nil {
		return
	}
	g := grid.Geometry(multiplier)
	ox, oy := float64(x), float64(y)
	var rings Path
	for _, r := range g.RingRadii {
		rings.ellipse(ox+g.CenterX, oy+g.CenterY, r, r)
	}
	c.record(Op{Paint: PaintBlack, Path: rings, Stroke: g.RingWidth})
	var hexes Path
	for _, h := range g.Hexagons {
		var pts []Point
		for _, p := range h {
			pts = append(pts, Point{ox + p[0], oy + p[1]})
		}
		hexes.polygon(pts...)
	}
	c.record(Op{Paint: PaintBlack, Path: hexes, Fill: true})
}
