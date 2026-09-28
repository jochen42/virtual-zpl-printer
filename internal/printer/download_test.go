package printer

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"testing"
)

func TestExtractDownloads(t *testing.T) {
	binary := []byte("bin^XZ~\x00,data") // contains ZPL control characters
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write([]byte("zipped"))
	_ = zw.Close()

	job := []byte("^XA~DYE:FONT.TTF,B,T,13,,")
	job = append(job, binary...)
	job = append(job, "^XZ~DYR:LOGO,A,G,2,1,CAFE^XA^FDhi^FS^XZ"...)
	job = append(job, "~DUR:BIG.FNT,3,414243"...)
	job = append(job, "~dyE:Z,A,T,6,,:Z64:"+base64.StdEncoding.EncodeToString(z.Bytes())+":ABCD^XZ"...)

	rest, downloads, err := extractDownloads(job)
	if err != nil {
		t.Fatal(err)
	}
	if want := "^XA^XZ^XA^FDhi^FS^XZ^XZ"; string(rest) != want {
		t.Fatalf("rest = %q, want %q", rest, want)
	}
	want := []Download{
		{"E:FONT.TTF", binary},
		{"R:LOGO.GRF", []byte{0xCA, 0xFE}},
		{"R:BIG.FNT", []byte("ABC")},
		{"E:Z.TTF", []byte("zipped")},
	}
	if len(downloads) != len(want) {
		t.Fatalf("got %d downloads, want %d", len(downloads), len(want))
	}
	for i, w := range want {
		if downloads[i].Name != w.Name || !bytes.Equal(downloads[i].Data, w.Data) {
			t.Errorf("download %d = %s %q, want %s %q", i, downloads[i].Name, downloads[i].Data, w.Name, w.Data)
		}
	}
}

func TestExtractDownloadsTruncatedBinary(t *testing.T) {
	_, _, err := extractDownloads([]byte("~DYE:F.TTF,B,T,100,,short"))
	if err == nil {
		t.Fatal("expected error for truncated binary download")
	}
}
