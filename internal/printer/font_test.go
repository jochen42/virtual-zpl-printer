package printer

import (
	"testing"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
)

func TestParsesNamedFontsAndAliases(t *testing.T) {
	labels, err := zpl.ParseAll("^XA^CWx,e:geist.ttf^FDa^FS^XZ^XA^A@R,40,0,E:ARIAL.TTF^FDb^FS^AXN,30^FDc^FS^XZ")
	if err != nil {
		t.Fatal(err)
	}
	var fonts []*zpl.ScalableFont
	for _, l := range labels {
		for _, c := range l.Commands() {
			if f, ok := c.(*zpl.ScalableFont); ok {
				fonts = append(fonts, f)
			}
		}
	}
	want := []zpl.ScalableFont{
		{Font: zpl.FontNamed, Orientation: zpl.OrientationRotated90, Height: 40, Name: "E:ARIAL.TTF"},
		// ^CW from the previous format still applies; width stays proportional.
		{Font: zpl.FontNamed, Orientation: zpl.OrientationNormal, Height: 30, Name: "E:GEIST.TTF"},
	}
	if len(fonts) != len(want) {
		t.Fatalf("got %d font commands, want %d", len(fonts), len(want))
	}
	for i, w := range want {
		if *fonts[i] != w {
			t.Errorf("font %d = %+v, want %+v", i, *fonts[i], w)
		}
	}
}
