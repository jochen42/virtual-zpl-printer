# Local patches

Vendored from github.com/StirlingMarketingGroup/go-zpl v0.1.8 (MIT). Tests, docs site, Rust and CLI were dropped.

- `^A@o,h,w,d:f.x` parses to `ScalableFont{Font: FontNamed, Name: …}`.
- `^CWa,d:f.x` parses to `FontIdentifier`.
- `render.RegisterFont(name, data)` registers a TTF/OTF under its printer path; `^A@` and `^CW` resolve against it, unknown names fall back to font 0 (`render/custom_font.go`).
- Space characters (Unicode Zs) a font has no glyph for, e.g. U+202F, render as U+0020 instead of the missing-glyph box (`render/font.go`).
- `Renderer.RenderDrawing` also returns the label as vector shapes (`render.Drawing`: paths, glyph outlines, `^GF` bitmaps) for PDF output. Each draw site records its shape next to the raster drawing, which is unchanged (`render/drawing.go`). MaxiCode shapes come from `SymbolGrid.Geometry`, which `Draw` now uses too.
