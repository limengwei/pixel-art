package ui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pixelart/internal/store"
)

func newPagerTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/pager.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	icons := make([]store.Icon, 0, 1000)
	for i := 0; i < 1000; i++ {
		icons = append(icons, store.Icon{ID: int64(i), Subject: "s", Caption: "c", CaptionZh: ""})
	}
	if err := st.InsertBatch(icons); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestPagerLoadsPagesLazily(t *testing.T) {
	st := newPagerTestStore(t)
	p := newPager(st, "", "", 1000)

	if _, ok := p.row(5); ok {
		t.Fatal("row before any load should miss")
	}
	var loaded atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)
	p.ensure(0, func() { loaded.Add(1); wg.Done() })
	wg.Wait()

	if got, ok := p.row(5); !ok || got.ID != 5 {
		t.Fatalf("row 5 = %+v %v", got, ok)
	}
	if got, ok := p.row(199); !ok || got.ID != 199 {
		t.Fatalf("row 199 = %+v %v", got, ok)
	}
	if _, ok := p.row(200); ok {
		t.Fatal("row 200 is page 1, should miss")
	}
	// Duplicate ensure does not load twice.
	p.ensure(0, func() { t.Fatal("already cached") })
	// Out-of-range pages are ignored.
	p.ensure(99, func() { t.Fatal("out of range") })
}

func TestPagerLRUOrder(t *testing.T) {
	st := newPagerTestStore(t)
	p := newPager(st, "", "", 1000)
	p.mu.Lock()
	p.pages[0] = make([]store.Icon, 10)
	p.pages[1] = make([]store.Icon, 10)
	p.lru = []int{0, 1}
	p.mu.Unlock()
	p.touch(0)
	p.mu.Lock()
	order := append([]int(nil), p.lru...)
	p.mu.Unlock()
	if len(order) != 2 || order[0] != 1 || order[1] != 0 {
		t.Fatalf("lru order = %v, want [1 0]", order)
	}
}

func TestPagerEnsureIsAsyncAndDeduped(t *testing.T) {
	st := newPagerTestStore(t)
	p := newPager(st, "", "", 1000)
	done := make(chan struct{}, 4)
	p.ensure(2, func() { done <- struct{}{} })
	p.ensure(2, func() { t.Fatal("duplicate load ran") })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ensure did not finish")
	}
}
