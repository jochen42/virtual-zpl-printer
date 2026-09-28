package printer

import (
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
