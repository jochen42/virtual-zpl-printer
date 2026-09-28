# Local patches

Vendored from github.com/StirlingMarketingGroup/go-zpl v0.1.8 (MIT). Tests, docs site, Rust and CLI were dropped.

- `^A@o,h,w,d:f.x` parses to `ScalableFont{Font: FontNamed, Name: …}`.
- `^CWa,d:f.x` parses to `FontIdentifier`.
- `render.RegisterFont(name, data)` registers a TTF/OTF under its printer path; `^A@` and `^CW` resolve against it, unknown names fall back to font 0 (`render/custom_font.go`).
