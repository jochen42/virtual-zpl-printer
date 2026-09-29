package maxicode

import "github.com/fogleman/gg"

// SymbolGrid represents a 33-row by 30-column MaxiCode symbol as a boolean grid.
type SymbolGrid [30 * 33]bool

// SetModule sets the module at the given row and column to the specified value.
func (s *SymbolGrid) SetModule(row, column int, value bool) {
	s[30*row+column] = value
}

// GetModule returns the value of the module at the given row and column.
func (s *SymbolGrid) GetModule(row, column int) bool {
	return s[30*row+column]
}

// Draw renders the symbol grid as hexagonal modules with a bullseye center pattern.
func (s *SymbolGrid) Draw(multiplier float64) *gg.Context {
	g := s.Geometry(multiplier)
	dc := gg.NewContext(int(28*multiplier), int(26.8*multiplier))

	// Central bullseye patterns.
	dc.SetLineWidth(g.RingWidth)
	for _, r := range g.RingRadii {
		dc.DrawCircle(g.CenterX, g.CenterY, r)
		dc.SetRGB(0, 0, 0)
		dc.Stroke()
	}

	// Hexagons
	for _, hex := range g.Hexagons {
		dc.MoveTo(hex[0][0], hex[0][1])
		for _, p := range hex[1:] {
			dc.LineTo(p[0], p[1])
		}
		dc.Fill()
	}

	return dc
}

// Geometry is the symbol's shapes in pixels at a scale of multiplier.
type Geometry struct {
	CenterX, CenterY float64
	// RingRadii are the bullseye circles, stroked with RingWidth.
	RingRadii [3]float64
	RingWidth float64
	// Hexagons are the dark modules as filled polygons.
	Hexagons [][6][2]float64
}

// Geometry returns the symbol's shapes, as drawn by Draw.
func (s *SymbolGrid) Geometry(multiplier float64) Geometry {
	g := Geometry{
		CenterX:   13.64 * multiplier,
		CenterY:   13.43 * multiplier,
		RingRadii: [3]float64{3.54 * multiplier, 2.20 * multiplier, 0.85 * multiplier},
		RingWidth: 0.67 * multiplier,
	}
	for row := 0; row < 33; row++ {
		for column := 0; column < 30; column++ {
			if s.GetModule(row, column) {
				rowOffset := 0.88
				if (row & 1) == 1 {
					rowOffset = 1.32
				}

				hexRectX := (float64(column)*0.88 + rowOffset) * multiplier
				hexRectY := (float64(row)*0.76 + 0.76) * multiplier
				hexRectW := 0.76 * multiplier
				hexRectH := 0.88 * multiplier

				g.Hexagons = append(g.Hexagons, [6][2]float64{
					{hexRectX + hexRectW*0.5, hexRectY},
					{hexRectX + hexRectW, hexRectY + hexRectH*0.25},
					{hexRectX + hexRectW, hexRectY + hexRectH*0.75},
					{hexRectX + hexRectW*0.5, hexRectY + hexRectH},
					{hexRectX, hexRectY + hexRectH*0.75},
					{hexRectX, hexRectY + hexRectH*0.25},
				})
			}
		}
	}
	return g
}

// SaveToPNG renders the symbol grid and saves it as a PNG file.
func (s *SymbolGrid) SaveToPNG(multiplier float64, path string) error {
	return s.Draw(multiplier).SavePNG(path)
}
