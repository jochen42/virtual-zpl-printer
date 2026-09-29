package printer

import (
	"bytes"
	"image/color"
	"image/png"
	"net"
	"strings"
	"testing"
	"time"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
)

func TestReadJobEndsAfterIdleWhenLabelComplete(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() { _, _ = client.Write([]byte("^xa^fdhi^fs^xz")) }()

	start := time.Now()
	data, err := readJob(server)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "^xa^fdhi^fs^xz" {
		t.Fatalf("got %q", data)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("did not end on idle after a complete label")
	}
}

func TestRenderMultipleLabels(t *testing.T) {
	out, err := Render([]byte("^XA^FO10,10^A0N,30,30^FDone^FS^XZ\n^XA^FO10,10^A0N,30,30^FDtwo^FS^XZ"), zpl.DPI203)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Images) != 2 {
		t.Fatalf("got %d images, want 2", len(out.Images))
	}
}

func TestRenderPDF(t *testing.T) {
	out, err := Render([]byte("^XA^FO10,10^A0N,30,30^FDone^FS^XZ\n^XA^FO10,10^BY2^BCN,50^FD123^FS^XZ"), zpl.DPI203)
	if err != nil {
		t.Fatal(err)
	}
	pdf := string(out.PDF)
	if !strings.HasPrefix(pdf, "%PDF-") || !strings.Contains(pdf, "/Count 2") {
		t.Fatalf("want a 2-page PDF, got %.100q", pdf)
	}
	// Text and barcodes are paths, not embedded bitmaps.
	if strings.Contains(pdf, "/Subtype /Image") {
		t.Fatal("PDF contains images")
	}
}

func TestRenderIsOneBit(t *testing.T) {
	out, err := Render([]byte("^XA^PW200^LL60^FO10,10^A0N,30,30^FDInk^FS^XZ"), zpl.DPI203)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out.Images[0]))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[color.Gray]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			seen[color.GrayModel.Convert(img.At(x, y)).(color.Gray)] = true
		}
	}
	if len(seen) != 2 || !seen[color.Gray{0}] || !seen[color.Gray{255}] {
		t.Fatalf("want only black and white pixels, got %v", seen)
	}
}
