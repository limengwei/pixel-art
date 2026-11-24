// Package ui builds the PixelIcon desktop interface with NSGo.
package ui

import (
	"image"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/indexer"
	"pixelart/internal/npy"
	"pixelart/internal/store"
	"pixelart/internal/translate"
)

// Deps carries what the UI needs from main.
type Deps struct {
	Store   *store.Store
	Dataset *npy.Dataset // may be nil when tokens.npy is unavailable
	MetaDir string
}

// pageSize is how many icons one background page fetch loads.
const pageSize = 200

// cellSize is one grid cell's icon box.
const cellSize = 76

// indexerProgress aliases the indexer's progress payload for the UI files.
type indexerProgress = indexer.Progress

// Main renders the whole app window content.
func Main(cx *core.Scope, deps *Deps) core.Node {
	st := deps.Store

	// --- state ---
	querySig := core.NewSignal(cx, "")
	subjectSig := core.NewSignal(cx, "") // "" = all subjects
	filterSig := core.NewSignal(cx, "")  // sidebar subject filter
	totalSig := core.NewSignal(cx, 0)
	refreshSig := core.NewSignal(cx, 0) // bumped when a page arrives
	subjectsSig := core.NewSignalFunc(cx, st.Subjects(), nil)
	allCountSig := core.NewSignal(cx, st.TotalIcons())
	progressSig := core.NewSignal(cx, indexer.Progress{})
	scanningSig := core.NewSignal(cx, false)
	detailSig := core.NewSignal(cx, store.Icon{})
	detailOpenSig := core.NewSignal(cx, false)
	settingsOpenSig := core.NewSignal(cx, false)
	sizeSig := core.NewSignal(cx, 128)
	toaster := ui.NewToaster(cx)

	// The current query's row source. Rows resolve lazily through its page
	// cache, so the grid can claim the whole result as its count without
	// ever holding it in memory.
	var pager *iconPager

	// --- sprite decoding (on demand, cached inside the dataset) ---
	spriteOf := func(id int64) *image.RGBA {
		if deps.Dataset == nil {
			return nil
		}
		img, err := deps.Dataset.Row(id)
		if err != nil {
			return nil
		}
		return img
	}

	// --- search ---
	searchGen := 0
	runSearch := func() {
		searchGen++
		myGen := searchGen
		q, sub := querySig.Get(), subjectSig.Get()
		go func() {
			res := st.Search(q, sub, 1, 1) // probe: the total is all we need
			p := newPager(st, q, sub, res.Total)
			cx.Post(func() {
				if myGen != searchGen {
					return
				}
				pager = p
				totalSig.Set(p.total)
				refreshSig.Set(refreshSig.Get() + 1)
			})
		}()
	}

	var stopDebounce func() bool
	onQueryChange := func(v string) {
		querySig.Set(v)
		if stopDebounce != nil {
			stopDebounce()
		}
		stopDebounce = cx.AfterFunc(300*time.Millisecond, runSearch)
	}
	onSubmitSearch := func() {
		if stopDebounce != nil {
			stopDebounce()
		}
		runSearch()
	}
	onSubjectPick := func(subject string) {
		subjectSig.Set(subject)
		runSearch()
	}

	// --- indexing ---
	rebuild := func() {
		if scanningSig.Get() {
			toaster.Show(ui.ToastContent{Type: ui.ToastPlain, Title: "正在索引中", Description: "请等待当前任务完成"})
			return
		}
		scanningSig.Set(true)
		metaDir := st.GetSetting("meta_dir", indexer.DefaultMetaDir)
		go func() {
			err := indexer.Run(metaDir, st, func(p indexer.Progress) {
				cx.Post(func() { progressSig.Set(p) })
			})
			cx.Post(func() {
				scanningSig.Set(false)
				if err != nil {
					toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: "索引失败", Description: err.Error(), Duration: 10 * time.Second})
				} else {
					toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: "索引完成", Description: progressSig.Get().Note})
				}
				subjectsSig.Set(st.Subjects())
				allCountSig.Set(st.TotalIcons())
				runSearch()
			})
		}()
	}

	// --- startup: count the whole set, and build the index when empty. ---
	runSearch()
	if indexer.NeedsBuild(st) {
		rebuild()
	}

	// --- detail helpers ---
	closeDetail := func() { detailOpenSig.Set(false) }

	// Layout.
	return core.View{Class: appClass, Children: []core.Node{
		sidebar(cx, sidebarState{
			subjects: subjectsSig, allCount: allCountSig, filter: filterSig,
			subject: subjectSig, onPick: onSubjectPick,
		}),
		core.View{Class: "flex-1 flex-col min-w-0", Children: []core.Node{
			toolbar(cx, toolbarState{
				query: querySig, onChange: onQueryChange, onSubmit: onSubmitSearch,
				onRebuild: rebuild, onSettings: func() { settingsOpenSig.Set(true) },
			}),
			progressRow(cx, progressSig, scanningSig),
			iconGrid(cx, gridState{
				total:   totalSig,
				refresh: refreshSig,
				row: func(i int) (store.Icon, bool) {
					if pager == nil {
						return store.Icon{}, false
					}
					return pager.row(i)
				},
				ensure: func(pg int) {
					if pager == nil {
						return
					}
					pager.ensure(pg, func() {
						cx.Post(func() { refreshSig.Set(refreshSig.Get() + 1) })
					})
				},
				onOpen: func(ic store.Icon) {
					detailSig.Set(ic)
					detailOpenSig.Set(true)
				},
				spriteOf: spriteOf,
			}),
			statusBar(cx, totalSig),
		}},
		core.Show{When: detailOpenSig.Get, Then: func(cx *core.Scope) core.Node {
			return detailPanel(cx, detailState{
				icon: detailSig, size: sizeSig, toaster: toaster, st: st,
				onClose: closeDetail, spriteOf: spriteOf,
			})
		}},
		core.Show{When: settingsOpenSig.Get, Then: func(cx *core.Scope) core.Node {
			return settingsPanel(cx, deps, toaster, func() { settingsOpenSig.Set(false) }, rebuild)
		}},
		ui.Toasts{Toaster: toaster},
	}}
}

const appClass = "w-full h-full flex-row bg-neutral-950 text-neutral-100 overflow-hidden"

// visibleSubjects filters subjects by a case-insensitive substring over the
// English name or its Chinese translation.
func visibleSubjects(all []store.Subject, filter string) []store.Subject {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return all
	}
	out := make([]store.Subject, 0, 64)
	for _, s := range all {
		if strings.Contains(strings.ToLower(s.Name), filter) ||
			strings.Contains(translate.Subject(s.Name), filter) {
			out = append(out, s)
		}
	}
	return out
}
