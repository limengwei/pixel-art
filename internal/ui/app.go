// Package ui builds the PixelIcon desktop interface with NSGo.
package ui

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/dataset"
	"pixelart/internal/npy"
	"pixelart/internal/store"
	"pixelart/internal/translate"
)

// Deps carries what the UI needs from main.
type Deps struct {
	Store *store.Store
	// ReleaseSingleton frees the single-instance lock, so an in-app restart
	// can spawn the next process before this one quits.
	ReleaseSingleton func()
	// OnBooted runs once the background boot finished, with the dataset
	// it opened (nil when the files failed their integrity check).
	OnBooted func(*npy.Dataset)
}

// pageSize is how many icons one background page fetch loads.
const pageSize = 200

// cellSize is one grid cell's icon box.
const cellSize = 76

// Main renders the whole app window content. The window opens instantly
// with a splash; the integrity check (which hashes the dataset when a
// manifest exists) and the dataset open run in the background and flip the
// UI to the browser once done.
func Main(cx *core.Scope, deps *Deps) core.Node {
	st := deps.Store

	// --- state (filled by the boot goroutine below) ---
	querySig := core.NewSignal(cx, "")
	subjectSig := core.NewSignal(cx, "") // "" = all subjects
	filterSig := core.NewSignal(cx, "")  // sidebar subject filter
	totalSig := core.NewSignal(cx, 0)
	refreshSig := core.NewSignal(cx, 0) // bumped when a page arrives
	subjectsSig := core.NewSignalFunc(cx, []store.Subject(nil), nil)
	allCountSig := core.NewSignal(cx, 0)
	detailSig := core.NewSignal(cx, store.Icon{})
	detailOpenSig := core.NewSignal(cx, false)
	settingsOpenSig := core.NewSignal(cx, false)
	bootSig := core.NewSignal(cx, false)   // splash until true
	welcomeSig := core.NewSignal(cx, false) // incomplete data after boot
	problemSig := core.NewSignal(cx, "")
	sizeSig := core.NewSignal(cx, 128)
	toaster := ui.NewToaster(cx)

	// The dataset is reopened in place when the first-run download finishes;
	// spriteOf reads it through this variable each call.
	var ds *npy.Dataset

	// The current query's row source. Rows resolve lazily through its page
	// cache, so the grid can claim the whole result as its count without
	// ever holding it in memory.
	var pager *iconPager

	// --- sprite decoding (on demand, cached inside the dataset) ---
	spriteOf := func(id int64) *image.RGBA {
		if ds == nil {
			return nil
		}
		img, err := ds.Row(id)
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

	// --- first-run: the deploy finished (dataset + index in place) ---
	onDatasetReady := func() {
		if d, err := npy.Open(dataset.Dir()); err == nil {
			ds = d
		}
		subjectsSig.Set(st.Subjects())
		allCountSig.Set(st.TotalIcons())
		welcomeSig.Set(false)
		runSearch()
		toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: "部署完成",
			Description: fmt.Sprintf("共 %d 个图标已可搜索", st.TotalIcons())})
	}

	// --- background boot: integrity check (hashes when a manifest exists)
	// and dataset open, so the window can show the splash immediately. ---
	go func() {
		filesOK, fileProblems := dataset.Check()
		var problems []string
		if !filesOK {
			problems = fileProblems
		}
		if indexDone := st.TotalIcons(); indexDone != dataset.ExpectedIcons {
			problems = append(problems, fmt.Sprintf("搜索索引不完整（%d / %d）", indexDone, dataset.ExpectedIcons))
		}
		var dsNew *npy.Dataset
		if filesOK {
			if d, err := npy.Open(dataset.Dir()); err == nil {
				dsNew = d
			}
			// Backfill: installs deployed before hash manifests existed.
			if !dataset.HasManifest() {
				go func() { _ = dataset.WriteManifest() }()
			}
		}
		cx.Post(func() {
			ds = dsNew
			subjectsSig.Set(st.Subjects())
			allCountSig.Set(st.TotalIcons())
			if len(problems) > 0 {
				problemSig.Set(strings.Join(problems, "；"))
				welcomeSig.Set(true)
			}
			bootSig.Set(true)
			runSearch()
			if deps.OnBooted != nil {
				go deps.OnBooted(ds)
			}
		})
	}()

	// --- detail helpers ---
	closeDetail := func() { detailOpenSig.Set(false) }

	// Layout: splash until boot completes, then the browser.
	return core.View{Class: appClass, Children: []core.Node{
		core.Show{When: func() bool { return !bootSig.Get() }, Then: func(cx *core.Scope) core.Node {
			return splashPanel()
		}},
		core.Show{When: bootSig.Get, Then: func(cx *core.Scope) core.Node {
			return core.View{Class: "w-full h-full flex-row min-h-0", Children: []core.Node{
				sidebar(cx, sidebarState{
					subjects: subjectsSig, allCount: allCountSig, filter: filterSig,
					subject: subjectSig, onPick: onSubjectPick,
				}),
				core.View{Class: "flex-1 flex-col min-w-0", Children: []core.Node{
					toolbar(cx, toolbarState{
						query: querySig, onChange: onQueryChange, onSubmit: onSubmitSearch,
						onSettings: func() { settingsOpenSig.Set(true) },
					}),
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
					return settingsPanel(cx, deps, toaster, func() { settingsOpenSig.Set(false) })
				}},
				core.Show{When: welcomeSig.Get, Then: func(cx *core.Scope) core.Node {
					return welcomePanel(cx, welcomeState{st: st, problem: problemSig.Get(), onReady: onDatasetReady})
				}},
			}}
		}},
		ui.Toasts{Toaster: toaster},
	}}
}

// splashPanel is the boot screen: spinner plus a line of status text.
func splashPanel() core.Node {
	return core.View{Class: "w-full h-full flex-col items-center justify-center gap-4 bg-neutral-950", Children: []core.Node{
		ui.ProgressRing{Indeterminate: func() bool { return true }, Size: 40},
		core.Text{Content: "正在启动 PixelIcon…", Style: core.Style{FontSize: 14}},
		core.Text{Content: "校验数据完整性并加载数据集（约 3 秒）", Style: core.Style{FontSize: 11, Color: core.RGB(0x737373)}},
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
