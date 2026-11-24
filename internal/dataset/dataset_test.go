package dataset

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip builds a dataset-shaped zip under path.
func writeZip(t *testing.T, path string, withTopFolder bool) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	prefix := ""
	if withTopFolder {
		prefix = Name + "/"
	}
	for name, data := range map[string]string{
		"tokens.npy":     strings.Repeat("t", 40<<20), // > 16MB to exercise chunked progress
		"captions.json":  `["a","b"]`,
		"palette.npy":    "p",
		"objects.json":   `["x","y"]`,
		"README.md":      "readme",
		"LICENSE":        "cc0",
		"split.npy":      "s",
		"banner.png":     "b",
	} {
		w, err := zw.Create(prefix + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractAtWithTopFolder(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, Name)
	zipPath := filepath.Join(t.TempDir(), "ds.zip")
	writeZip(t, zipPath, true)

	var reports int
	var lastDone int64
	var final Progress
	err := extractAt(base, dir, zipPath, func(p Progress) {
		reports++
		if p.Done < lastDone {
			t.Errorf("progress went backwards: %d < %d", p.Done, lastDone)
		}
		lastDone = p.Done
		if p.Phase != "解压" {
			t.Errorf("phase = %q, want 解压", p.Phase)
		}
		final = p
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.Done != final.Total || final.Total == 0 {
		t.Errorf("final progress = %d/%d, want equal and nonzero", final.Done, final.Total)
	}
	if reports < 2 {
		t.Errorf("reports = %d, want chunked progress (>= 2)", reports)
	}
	if _, err := os.Stat(filepath.Join(dir, "tokens.npy")); err != nil {
		t.Fatalf("tokens.npy missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "captions.json")); err == nil {
		t.Error("captions.json should live inside the dataset dir, not base")
	}
}

func TestExtractAtHoistsLooseFiles(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, Name)
	zipPath := filepath.Join(t.TempDir(), "ds.zip")
	writeZip(t, zipPath, false)

	if err := extractAt(base, dir, zipPath, func(Progress) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tokens.npy")); err != nil {
		t.Fatalf("loose tokens.npy not hoisted into dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "tokens.npy")); err == nil {
		t.Error("tokens.npy should have been moved out of base")
	}
}
