package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"pixelart/internal/store"
)

// writeMetaJSON writes captions.json and objects.json arrays.
func writeMetaJSON(t *testing.T, dir string, captions, objects []string) {
	t.Helper()
	if len(captions) != len(objects) {
		t.Fatal("test arrays must be parallel")
	}
	caps := "["
	objs := "["
	for i, c := range captions {
		if i > 0 {
			caps += ","
			objs += ","
		}
		caps += quote(c)
		objs += quote(objects[i])
	}
	caps += "]"
	objs += "]"
	os.WriteFile(filepath.Join(dir, "captions.json"), []byte(caps), 0o644)
	os.WriteFile(filepath.Join(dir, "objects.json"), []byte(objs), 0o644)
}

func quote(s string) string {
	out := "\""
	for _, r := range s {
		switch r {
		case '"':
			out += "\\\""
		case '\\':
			out += "\\\\"
		default:
			out += string(r)
		}
	}
	return out + "\""
}

func TestRunBuildsIndex(t *testing.T) {
	metaDir := t.TempDir()
	writeMetaJSON(t, metaDir,
		[]string{"brand new aa battery", "pink dress form", "cute star", ""},
		[]string{"aa battery", "dress form", "star", ""}) // row 3 has no subject: skipped

	db := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if !NeedsBuild(st) {
		t.Fatal("fresh store should need a build")
	}
	var done Progress
	if err := Run(metaDir, st, func(p Progress) { done = p }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if st.TotalIcons() != 3 {
		t.Fatalf("indexed %d icons, want 3", st.TotalIcons())
	}
	if done.Note == "" {
		t.Fatal("final progress note should summarize the build")
	}
	if NeedsBuild(st) {
		t.Fatal("built store should not need a build")
	}

	// English search.
	if res := st.Search("battery", "", 1, 20); res.Total != 1 || res.Icons[0].ID != 0 {
		t.Fatalf("search battery = %+v", res.Icons)
	}
	// Subject browse.
	if res := st.Search("", "star", 1, 20); res.Total != 1 || res.Icons[0].Caption != "cute star" {
		t.Fatalf("browse star = %+v", res.Icons)
	}
	// Chinese search via translated tags (star -> 星).
	if res := st.Search("星", "", 1, 20); res.Total != 1 {
		t.Fatalf("search 星 total = %d, want 1", res.Total)
	}
	// Subjects table.
	if subs := st.Subjects(); len(subs) != 3 {
		t.Fatalf("subjects = %+v", subs)
	}
	// Running again is idempotent.
	if err := Run(metaDir, st, func(Progress) {}); err != nil {
		t.Fatal(err)
	}
	if st.TotalIcons() != 3 {
		t.Fatalf("rebuild changed count to %d", st.TotalIcons())
	}
}

func TestRunMissingDir(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := Run(t.TempDir(), st, func(Progress) {}); err == nil {
		t.Fatal("missing captions.json should fail")
	}
}
