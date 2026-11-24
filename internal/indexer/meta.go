package indexer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Meta holds the dataset's parallel caption and subject arrays, indexed by
// row id (the 8-digit number in each file name).
type Meta struct {
	captions []string
	objects  []string
}

// LoadMeta streams captions.json and objects.json from dir into memory.
// Missing files return an error the caller may ignore.
func LoadMeta(dir string, progress func(done, total int)) (*Meta, error) {
	if dir == "" {
		return nil, fmt.Errorf("meta dir empty")
	}
	m := &Meta{}
	var err error
	if m.captions, err = loadStrings(filepath.Join(dir, "captions.json"), progress); err != nil {
		return nil, err
	}
	if m.objects, err = loadStrings(filepath.Join(dir, "objects.json"), nil); err != nil {
		return nil, err
	}
	if len(m.captions) != len(m.objects) {
		return nil, fmt.Errorf("captions (%d) and objects (%d) lengths differ", len(m.captions), len(m.objects))
	}
	return m, nil
}

// Get returns the caption and subject for a row id.
func (m *Meta) Get(id int64) (caption, subject string, ok bool) {
	if m == nil || id < 0 || id >= int64(len(m.captions)) {
		return "", "", false
	}
	return m.captions[id], m.objects[id], true
}

// Len returns the number of rows the metadata covers.
func (m *Meta) Len() int {
	if m == nil {
		return 0
	}
	return len(m.captions)
}

// loadStrings decodes a JSON array of strings, streaming element by element
// and interning duplicates (objects.json repeats each subject ~230 times).
func loadStrings(path string, progress func(done, total int)) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return nil, fmt.Errorf("%s: not a JSON array", path)
	}
	out := make([]string, 0, 1_410_000)
	pool := make(map[string]string, 8_000)
	for dec.More() {
		var s string
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if interned, ok := pool[s]; ok {
			s = interned
		} else {
			pool[s] = s
		}
		out = append(out, s)
		if progress != nil && len(out)%200_000 == 0 {
			progress(len(out), 0)
		}
	}
	return out, nil
}
