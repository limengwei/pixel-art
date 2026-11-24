package ui

import (
	"sync"

	"pixelart/internal/store"
)

// iconPager serves icon rows by absolute index for the virtualized grid,
// loading them from SQLite one page at a time and keeping a bounded LRU
// cache. The grid's Count is the whole result set, so nothing is ever
// accumulated in memory no matter how far the user scrolls.
type iconPager struct {
	st       *store.Store
	query    string
	subject  string
	total    int
	pageSize int

	mu      sync.Mutex
	pages   map[int][]store.Icon // page number -> rows
	lru     []int                // page numbers, least recent first
	loading map[int]bool
}

// maxCachedPages bounds the row cache (600 pages x 200 rows = 120k rows).
const maxCachedPages = 600

// newPager prepares a pager for one query. Rows load lazily via ensure.
func newPager(st *store.Store, query, subject string, total int) *iconPager {
	return &iconPager{
		st: st, query: query, subject: subject, total: total,
		pageSize: pageSize,
		pages:    make(map[int][]store.Icon),
		loading:  make(map[int]bool),
	}
}

// row returns icon i when its page is cached.
func (p *iconPager) row(i int) (store.Icon, bool) {
	if i < 0 || i >= p.total {
		return store.Icon{}, false
	}
	pg := i / p.pageSize
	p.mu.Lock()
	defer p.mu.Unlock()
	rows, ok := p.pages[pg]
	if !ok {
		return store.Icon{}, false
	}
	p.touch(pg)
	return rows[i-pg*p.pageSize], true
}

// ensure loads page pg asynchronously when needed; onLoaded runs once the
// page is available (from the loading goroutine).
func (p *iconPager) ensure(pg int, onLoaded func()) {
	if pg < 0 || pg >= (p.total+p.pageSize-1)/p.pageSize {
		return
	}
	p.mu.Lock()
	if _, ok := p.pages[pg]; ok || p.loading[pg] {
		p.mu.Unlock()
		return
	}
	p.loading[pg] = true
	p.mu.Unlock()

	go func() {
		res := p.st.Search(p.query, p.subject, pg+1, p.pageSize)
		p.mu.Lock()
		p.pages[pg] = res.Icons
		delete(p.loading, pg)
		p.touch(pg)
		for len(p.lru) > maxCachedPages {
			oldest := p.lru[0]
			p.lru = p.lru[1:]
			delete(p.pages, oldest)
		}
		p.mu.Unlock()
		onLoaded()
	}()
}

// touch moves pg to the fresh end of the LRU order.
func (p *iconPager) touch(pg int) {
	for i, v := range p.lru {
		if v == pg {
			p.lru = append(p.lru[:i], p.lru[i+1:]...)
			break
		}
	}
	p.lru = append(p.lru, pg)
}
