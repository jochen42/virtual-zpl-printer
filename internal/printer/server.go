// Package printer implements a raw TCP (JetDirect / port 9100) printer that
// accepts ZPL jobs.
package printer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	zpl "github.com/StirlingMarketingGroup/go-zpl"

	"github.com/jochen42/virtual-zpl-printer/internal/memory"
	"github.com/jochen42/virtual-zpl-printer/internal/store"
)

const (
	maxJobSize = 64 << 20
	// Senders usually close the connection after a job, but some keep it open.
	// Once a complete label arrived, a short pause ends the job.
	idleAfterLabel = 750 * time.Millisecond
	idleTimeout    = 30 * time.Second
)

type Server struct {
	Addr  string
	DPI   zpl.DPI
	Store *store.Store
	// Memory receives objects stored by ~DY / ~DU.
	Memory *memory.Memory
	// OnPrint is called after a job has been saved.
	OnPrint func(store.Print)
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	log.Printf("printer listening on %s", ln.Addr())
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	receivedAt := time.Now()
	remote := conn.RemoteAddr().String()

	data, err := readJob(conn)
	if err != nil {
		log.Printf("job from %s: %v", remote, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return
	}

	job := store.Job{ReceivedAt: receivedAt, RemoteAddr: remote, Data: data}
	rest, downloads, dlErr := extractDownloads(data)
	errs := []error{dlErr}
	for _, dl := range downloads {
		if err := s.Memory.Store(dl.Name, dl.Data); err != nil {
			errs = append(errs, fmt.Errorf("storing %s: %w", dl.Name, err))
			continue
		}
		job.Stored = append(job.Stored, dl.Name)
	}
	images, renderErr := Render(rest, s.DPI)
	if errors.Is(renderErr, ErrNoLabel) && len(downloads) > 0 {
		// Download-only job.
		renderErr = nil
	}
	job.Images = images
	job.Err = errors.Join(append(errs, renderErr)...)

	p, err := s.Store.Save(job)
	if err != nil {
		log.Printf("saving job from %s: %v", remote, err)
		return
	}
	log.Printf("job %s from %s: %d bytes, %d label(s), stored %v", p.ID, remote, p.Bytes, len(p.Images), p.Stored)
	if p.Error != "" {
		log.Printf("job %s: %s", p.ID, p.Error)
	}
	if s.OnPrint != nil {
		s.OnPrint(p)
	}
}

func readJob(conn net.Conn) ([]byte, error) {
	var buf bytes.Buffer
	chunk := make([]byte, 32<<10)
	for {
		idle := idleTimeout
		if hasCompleteLabel(buf.Bytes()) {
			idle = idleAfterLabel
		}
		_ = conn.SetReadDeadline(time.Now().Add(idle))
		n, err := conn.Read(chunk)
		buf.Write(chunk[:n])
		if buf.Len() > maxJobSize {
			return buf.Bytes(), errors.New("job exceeds size limit, truncated")
		}
		switch {
		case err == nil:
		case errors.Is(err, io.EOF), errors.Is(err, os.ErrDeadlineExceeded):
			return buf.Bytes(), nil
		default:
			return buf.Bytes(), err
		}
	}
}

// hasCompleteLabel reports whether the last ^XA in data is followed by ^XZ.
func hasCompleteLabel(data []byte) bool {
	upper := bytes.ToUpper(data)
	start := bytes.LastIndex(upper, []byte("^XA"))
	return start >= 0 && bytes.Contains(upper[start:], []byte("^XZ"))
}
