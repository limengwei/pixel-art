package index

import (
	"os"
	"testing"

	"pixelart/internal/store"
)

func TestEnsureExtractsUsableDB(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: extracts the embedded index")
	}
	os.Remove(DBPath())
	os.Remove(DBPath() + ".ok")

	p, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("extracted db missing or empty: %v", err)
	}

	st, err := store.Open(p)
	if err != nil {
		t.Fatalf("open extracted db: %v", err)
	}
	defer st.Close()
	if n := st.TotalIcons(); n < 1_400_000 {
		t.Errorf("TotalIcons = %d, want >= 1400000", n)
	}
	if n := len(st.Subjects()); n != 6194 {
		t.Errorf("Subjects = %d, want 6194", n)
	}

	// A second Ensure must be a no-op that still succeeds.
	if _, err := Ensure(); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
}
