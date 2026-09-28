package printer

import (
	"bytes"
	"image/color"
	"image/png"
	"net"
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
	images, err := Render([]byte("^XA^FO10,10^A0N,30,30^FDone^FS^XZ\n^XA^FO10,10^A0N,30,30^FDtwo^FS^XZ"), zpl.DPI203)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}
}

func TestRenderIsOneBit(t *testing.T) {
	images, err := Render([]byte("^XA^PW200^LL60^FO10,10^A0N,30,30^FDInk^FS^XZ"), zpl.DPI203)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(images[0]))
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
