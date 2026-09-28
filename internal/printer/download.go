package printer

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Download is an object a job stores on the printer (~DY, ~DU).
type Download struct {
	Name string // printer path, e.g. "E:ARIAL.TTF"
	Data []byte
}

// extensions maps the ~DY extension parameter to a file extension.
var extensions = map[byte]string{
	'B': "BMP", 'E': "TTE", 'G': "GRF", 'P': "PNG", 'T': "TTF", 'X': "PCX", 'H': "HTM", 'N': "NRD", 'S': "SVG",
}

// extractDownloads removes ~DY and ~DU commands from data. They can carry raw
// binary that would derail the ZPL parser.
func extractDownloads(data []byte) (rest []byte, downloads []Download, err error) {
	var out bytes.Buffer
	var errs []string
	for {
		start := indexDownload(data)
		if start < 0 {
			out.Write(data)
			break
		}
		out.Write(data[:start])

		var dl Download
		var end int
		var perr error
		if bytes.EqualFold(data[start+1:start+3], []byte("DY")) {
			dl, end, perr = parseDY(data[start:])
		} else {
			dl, end, perr = parseDU(data[start:])
		}
		if perr != nil {
			errs = append(errs, perr.Error())
		} else {
			downloads = append(downloads, dl)
		}
		data = data[start+end:]
	}
	if len(errs) > 0 {
		err = fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return out.Bytes(), downloads, err
}

func indexDownload(data []byte) int {
	for i := 0; i+3 <= len(data); i++ {
		if data[i] != '~' || (data[i+1]|0x20) != 'd' {
			continue
		}
		if c := data[i+2] | 0x20; c == 'y' || c == 'u' {
			return i
		}
	}
	return -1
}

// splitParams reads n comma-separated parameters after the 3-byte command and
// returns them with the offset of the data that follows.
func splitParams(cmd []byte, n int) ([]string, int, error) {
	params := make([]string, 0, n)
	pos := 3
	for len(params) < n {
		i := bytes.IndexByte(cmd[pos:], ',')
		if i < 0 || i > 256 {
			return nil, 0, fmt.Errorf("%s: incomplete parameters", cmd[:3])
		}
		params = append(params, strings.TrimSpace(string(cmd[pos:pos+i])))
		pos += i + 1
	}
	return params, pos, nil
}

// parseDY parses ~DYd:f,b,x,t,w,data.
func parseDY(cmd []byte) (Download, int, error) {
	params, pos, err := splitParams(cmd, 5)
	if err != nil {
		return Download{}, len(cmd), err
	}
	name := strings.ToUpper(params[0])
	if !strings.Contains(name, ":") {
		name = "R:" + name
	}
	if !strings.Contains(name[strings.Index(name, ":"):], ".") && params[2] != "" {
		if ext, ok := extensions[strings.ToUpper(params[2])[0]]; ok {
			name += "." + ext
		}
	}
	size, err := strconv.Atoi(params[3])
	if err != nil || size < 0 {
		return Download{}, len(cmd), fmt.Errorf("~DY %s: invalid size %q", name, params[3])
	}

	var data []byte
	var n int
	switch strings.ToUpper(params[1]) {
	case "B":
		if pos+size > len(cmd) {
			return Download{}, len(cmd), fmt.Errorf("~DY %s: got %d of %d bytes", name, len(cmd)-pos, size)
		}
		data, n = cmd[pos:pos+size], size
	case "A", "":
		data, n, err = decodeASCII(cmd[pos:])
		if err != nil {
			return Download{}, pos + n, fmt.Errorf("~DY %s: %w", name, err)
		}
	default:
		return Download{}, pos, fmt.Errorf("~DY %s: unsupported format %q", name, params[1])
	}
	return Download{Name: name, Data: data}, pos + n, nil
}

// parseDU parses ~DUd:o.x,s,data (s bytes as hex).
func parseDU(cmd []byte) (Download, int, error) {
	params, pos, err := splitParams(cmd, 2)
	if err != nil {
		return Download{}, len(cmd), err
	}
	name := strings.ToUpper(params[0])
	if !strings.Contains(name, ":") {
		name = "R:" + name
	}
	data, n, err := decodeASCII(cmd[pos:])
	if err != nil {
		return Download{}, pos + n, fmt.Errorf("~DU %s: %w", name, err)
	}
	return Download{Name: name, Data: data}, pos + n, nil
}

// decodeASCII decodes hex, :B64: or :Z64: payloads up to the next command.
func decodeASCII(b []byte) ([]byte, int, error) {
	end := bytes.IndexAny(b, "^~")
	if end < 0 {
		end = len(b)
	}
	payload := strings.Join(strings.Fields(string(b[:end])), "")

	upper := strings.ToUpper(payload)
	if strings.HasPrefix(upper, ":B64:") || strings.HasPrefix(upper, ":Z64:") {
		body := payload[5:]
		if i := strings.IndexByte(body, ':'); i >= 0 {
			body = body[:i] // drop the CRC
		}
		raw, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return nil, end, err
		}
		if strings.HasPrefix(upper, ":Z64:") {
			zr, err := zlib.NewReader(bytes.NewReader(raw))
			if err != nil {
				return nil, end, err
			}
			defer zr.Close()
			raw, err = io.ReadAll(zr)
			if err != nil {
				return nil, end, err
			}
		}
		return raw, end, nil
	}
	raw, err := hex.DecodeString(payload)
	return raw, end, err
}
