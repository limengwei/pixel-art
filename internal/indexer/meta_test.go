package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

// writeMeta writes small captions/objects arrays covering row 1332.
func writeMeta(t *testing.T, dir string) {
	t.Helper()
	// 1333 entries; row 1332 is our aa battery.
	caps := `["sprite zero"`
	objs := `["thing zero"`
	for i := 1; i <= 1332; i++ {
		if i == 1332 {
			caps += `, "brand new aa battery"`
			objs += `, "aa battery"`
		} else {
			caps += `, "filler caption"`
			objs += `, "filler"`
		}
	}
	caps += "]"
	objs += "]"
	os.WriteFile(filepath.Join(dir, "captions.json"), []byte(caps), 0o644)
	os.WriteFile(filepath.Join(dir, "objects.json"), []byte(objs), 0o644)
}

func TestLoadMeta(t *testing.T) {
	dir := t.TempDir()
	writeMeta(t, dir)
	m, err := LoadMeta(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Len() != 1333 {
		t.Fatalf("len = %d", m.Len())
	}
	cap, sub, ok := m.Get(1332)
	if !ok || cap != "brand new aa battery" || sub != "aa battery" {
		t.Fatalf("Get(1332) = %q %q %v", cap, sub, ok)
	}
	if _, _, ok := m.Get(-1); ok {
		t.Fatal("Get(-1) should miss")
	}
	if _, _, ok := m.Get(999999); ok {
		t.Fatal("Get(999999) should miss")
	}
}
