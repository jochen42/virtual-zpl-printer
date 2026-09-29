// Package store persists received print jobs on disk, one directory per job.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

const (
	metaFile = "meta.json"
	zplFile  = "job.zpl"
	pdfFile  = "labels.pdf"
)

var idPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}(-[0-9]+)?$`)

// ErrNotFound is returned for unknown or malformed print IDs.
var ErrNotFound = errors.New("print not found")

// Print is the metadata of one received job.
type Print struct {
	ID         string    `json:"id"`
	ReceivedAt time.Time `json:"receivedAt"`
	RemoteAddr string    `json:"remoteAddr"`
	Bytes      int       `json:"bytes"`
	// Images are file names inside the print directory, one per rendered label.
	Images []string `json:"images"`
	// PDF is the file name of the labels as a vector PDF, if rendered.
	PDF string `json:"pdf,omitempty"`
	// Stored lists objects the job saved on the printer, e.g. fonts from ~DY.
	Stored []string `json:"stored,omitempty"`
	Error  string   `json:"error,omitempty"`
}

// Job is a received job to save.
type Job struct {
	ReceivedAt time.Time
	RemoteAddr string
	Data       []byte
	Images     [][]byte // rendered PNGs
	PDF        []byte
	Stored     []string
	Err        error
}

// Store keeps an in-memory index of the prints found in Dir.
type Store struct {
	Dir string

	mu     sync.RWMutex
	prints map[string]Print
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{Dir: dir, prints: map[string]Print{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name(), metaFile))
		if err != nil {
			continue
		}
		var p Print
		if json.Unmarshal(raw, &p) == nil && p.ID == e.Name() {
			s.prints[p.ID] = p
		}
	}
	return s, nil
}

// Save writes the raw job, its rendered images and the metadata.
func (s *Store) Save(job Job) (Print, error) {
	receivedAt := job.ReceivedAt
	s.mu.Lock()
	defer s.mu.Unlock()

	id := receivedAt.UTC().Format("20060102T150405.000000000")
	for n := 2; ; n++ {
		if _, taken := s.prints[id]; !taken {
			break
		}
		id = fmt.Sprintf("%s-%d", receivedAt.UTC().Format("20060102T150405.000000000"), n)
	}

	dir := filepath.Join(s.Dir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Print{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, zplFile), job.Data, 0o644); err != nil {
		return Print{}, err
	}

	p := Print{ID: id, ReceivedAt: receivedAt, RemoteAddr: job.RemoteAddr, Bytes: len(job.Data), Images: []string{}, Stored: job.Stored}
	for i, img := range job.Images {
		name := fmt.Sprintf("label-%d.png", i+1)
		if err := os.WriteFile(filepath.Join(dir, name), img, 0o644); err != nil {
			return Print{}, err
		}
		p.Images = append(p.Images, name)
	}
	if job.PDF != nil {
		if err := os.WriteFile(filepath.Join(dir, pdfFile), job.PDF, 0o644); err != nil {
			return Print{}, err
		}
		p.PDF = pdfFile
	}
	if job.Err != nil {
		p.Error = job.Err.Error()
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Print{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, metaFile), raw, 0o644); err != nil {
		return Print{}, err
	}
	s.prints[id] = p
	return p, nil
}

// List returns all prints, newest first.
func (s *Store) List() []Print {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Print, 0, len(s.prints))
	for _, p := range s.prints {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReceivedAt.Equal(out[j].ReceivedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].ReceivedAt.After(out[j].ReceivedAt)
	})
	return out
}

func (s *Store) Get(id string) (Print, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.prints[id]
	if !ok {
		return Print{}, ErrNotFound
	}
	return p, nil
}

// ZPL returns the raw job data.
func (s *Store) ZPL(id string) ([]byte, error) {
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(s.Dir, id, zplFile))
}

// PrintDir returns the on-disk directory of a print.
func (s *Store) PrintDir(id string) (string, error) {
	if _, err := s.Get(id); err != nil {
		return "", err
	}
	return filepath.Join(s.Dir, id), nil
}

// PDF returns the print's labels as PDF.
func (s *Store) PDF(id string) ([]byte, error) {
	p, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if p.PDF == "" {
		return nil, ErrNotFound
	}
	return os.ReadFile(filepath.Join(s.Dir, id, p.PDF))
}

// ImagePath returns the on-disk path of one of the print's images.
func (s *Store) ImagePath(id, name string) (string, error) {
	p, err := s.Get(id)
	if err != nil {
		return "", err
	}
	for _, img := range p.Images {
		if img == name {
			return filepath.Join(s.Dir, id, name), nil
		}
	}
	return "", ErrNotFound
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.prints[id]; !ok {
		return ErrNotFound
	}
	if err := os.RemoveAll(filepath.Join(s.Dir, id)); err != nil {
		return err
	}
	delete(s.prints, id)
	return nil
}
