// Package memory emulates the printer's object storage (R:, E:, B: …) that
// ~DY and ~DU write to. Objects survive restarts; fonts are registered with
// the renderer so ^A@ and ^CW can use them.
package memory

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/StirlingMarketingGroup/go-zpl/render"
)

var namePattern = regexp.MustCompile(`^([A-Z]):([A-Z0-9_\-. ]{1,64})$`)

type Memory struct {
	Dir string

	mu    sync.RWMutex
	names []string
}

// Open loads all stored objects from dir and registers the fonts among them.
func Open(dir string) (*Memory, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	m := &Memory{Dir: dir}
	drives, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, drive := range drives {
		if !drive.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, drive.Name()))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			name := drive.Name() + ":" + f.Name()
			if f.IsDir() || !namePattern.MatchString(name) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, drive.Name(), f.Name()))
			if err != nil {
				return nil, err
			}
			if err := m.register(name, data); err != nil {
				log.Printf("stored object %s: %v", name, err)
			}
		}
	}
	return m, nil
}

// Store saves an object under its printer path, e.g. "E:ARIAL.TTF".
func (m *Memory) Store(name string, data []byte) error {
	name = strings.ToUpper(name)
	match := namePattern.FindStringSubmatch(name)
	if match == nil {
		return fmt.Errorf("invalid object name %q", name)
	}
	dir := filepath.Join(m.Dir, match[1])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, match[2]), data, 0o644); err != nil {
		return err
	}
	return m.register(name, data)
}

// Names lists the stored objects.
func (m *Memory) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.names...)
}

func (m *Memory) register(name string, data []byte) error {
	m.mu.Lock()
	if i := sort.SearchStrings(m.names, name); i == len(m.names) || m.names[i] != name {
		m.names = append(m.names, name)
		sort.Strings(m.names)
	}
	m.mu.Unlock()

	if isFont(name) {
		return render.RegisterFont(name, data)
	}
	return nil
}

func isFont(name string) bool {
	switch strings.ToUpper(filepath.Ext(name)) {
	case ".TTF", ".OTF", ".TTE":
		return true
	}
	return false
}
