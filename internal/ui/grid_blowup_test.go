package ui

import (
	"image"
	"runtime"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"pixelart/internal/store"
)

// TestGridCountBlowup mounts the grid as the app does (inside the flex
// column) with a huge count and a row source that always misses, guarding
// against the list losing virtualization again.
func TestGridCountBlowup(t *testing.T) {
	if testing.Short() {
		t.Skip("allocation probe")
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		totalSig := core.NewSignal(cx, maxGridRows)
		refreshSig := core.NewSignal(cx, 0)
		grid := iconGrid(cx, gridState{
			total:   totalSig,
			refresh: refreshSig,
			row:     func(int) (store.Icon, bool) { return store.Icon{}, false },
			ensure:  func(int) {},
			onOpen:  func(store.Icon) {},
			spriteOf: func(int64) *image.RGBA { return nil },
		})
		return core.View{Class: "w-full h-full flex-col", Children: []core.Node{
			core.View{Class: "flex-1 min-h-0", Children: []core.Node{grid}},
		}}
	})
	app.Advance(0)

	runtime.GC()
	runtime.ReadMemStats(&after)
	allocMB := float64(after.TotalAlloc-before.TotalAlloc) / 1e6
	heapMB := float64(after.HeapAlloc) / 1e6
	t.Logf("count=%d allocated=%.1fMB heap=%.1fMB", maxGridRows, allocMB, heapMB)
	if allocMB > 500 {
		t.Fatalf("grid with count %d allocated %.0fMB — the list stopped virtualizing", maxGridRows, allocMB)
	}
}

// TestSidebarSubjectsBlowup mounts the sidebar with many subjects.
func TestSidebarSubjectsBlowup(t *testing.T) {
	if testing.Short() {
		t.Skip("allocation probe")
	}
	subs := make([]store.Subject, 6194)
	for i := range subs {
		subs[i] = store.Subject{Name: string(rune('a'+i%26)) + "item", Count: 100}
	}
	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		subjectsSig := core.NewSignalFunc(cx, subs, nil)
		all := core.NewSignal(cx, len(subs))
		filter := core.NewSignal(cx, "")
		subject := core.NewSignal(cx, "")
		side := sidebar(cx, sidebarState{
			subjects: subjectsSig, allCount: all, filter: filter,
			subject: subject, onPick: func(string) {},
		})
		return core.View{Class: "w-full h-full flex-row", Children: []core.Node{side}}
	})
	app.Advance(0)

	var after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&after)
	allocMB := float64(after.TotalAlloc-before.TotalAlloc) / 1e6
	t.Logf("subjects=%d allocated=%.1fMB", len(subs), allocMB)
	if allocMB > 500 {
		t.Fatalf("sidebar allocated %.0fMB", allocMB)
	}
}
