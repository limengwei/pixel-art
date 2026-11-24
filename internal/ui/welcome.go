package ui

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/dataset"
	"pixelart/internal/indexer"
	"pixelart/internal/store"
)

//go:embed banner.png
var bannerRaw []byte

// bannerRGBA decodes the embedded dataset banner once for display.
var bannerRGBA = func() *image.RGBA {
	img, err := png.Decode(bytes.NewReader(bannerRaw))
	if err != nil {
		return nil
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return rgba
}()

// welcomeState feeds the first-launch deploy overlay.
type welcomeState struct {
	st      *store.Store
	problem string             // integrity problems found at startup ("" = fresh)
	open    *core.Signal[bool] // dialog visibility
	onReady func()             // data complete: reopen dataset, refresh and search
}

// deployProgress is one step of the deploy pipeline (download → extract →
// build index) as shown in the progress row.
type deployProgress struct {
	phase string
	text  string
	frac  float32 // 0..1; 0 = indeterminate
}

// welcomePanel is shown when the app's data is incomplete: banner, Chinese
// introduction, and a one-click deploy that downloads, extracts and indexes
// the dataset.
func welcomePanel(cx *core.Scope, s welcomeState) core.Node {
	stateSig := core.NewSignal(cx, "idle") // idle | running | error
	progSig := core.NewSignal(cx, deployProgress{})

	post := func(p deployProgress) { cx.Post(func() { progSig.Set(p) }) }
	fail := func(err error) {
		cx.Post(func() {
			stateSig.Set("error")
			progSig.Set(deployProgress{phase: "失败", text: err.Error()})
		})
	}

	start := func() {
		if stateSig.Get() == "running" {
			return
		}
		stateSig.Set("running")
		go func() {
			// 1. Download + extract (skipped when the files fully check out;
			// a half-downloaded or half-extracted dataset is re-fetched).
			if ok, _ := dataset.Check(); !ok {
				post(deployProgress{phase: "下载", text: "准备下载…"})
				if err := dataset.Download(context.Background(), func(p dataset.Progress) {
					post(deployProgress{phase: p.Phase, text: progressText(p), frac: fracOf(p.Done, p.Total)})
				}); err != nil {
					fail(err)
					return
				}
			}
			// 2. Build the search index (idempotent; fills a half-built one).
			if err := indexer.Run(dataset.Dir(), s.st, func(p indexer.Progress) {
				post(deployProgress{phase: "构建索引", text: indexProgressText(p), frac: indexFrac(p)})
			}); err != nil {
				fail(err)
				return
			}
			// 3. Persist file hashes so later startups verify content, not
			// just size. Failure only downgrades to size-only verification.
			_ = dataset.WriteManifest()
			cx.Post(func() { s.onReady() })
		}()
	}

	// ui.Dialog rides the driver's modal layer, which blocks input to the
	// app behind it — a hand-rolled absolute overlay lets clicks through.
	// The welcome cannot be dismissed: deploying is the only way on.
	return ui.Dialog{
		Open:                   s.open.Get,
		OnOpenChange:           func(open bool, _ base.Reason) {},
		NoCloseOnEscape:        true,
		NoCloseOnBackdropPress: true,
		Title:                  "欢迎来到 PixelIcon · 像素图标库",
		Style:                  core.Style{MaxWidth: core.Pct(90)},

		Children: []core.Node{
			// banner
			core.Image{
				SourceFunc: func() core.ImageSource {
					if bannerRGBA == nil {
						return core.ImageSource{}
					}
					b := bannerRGBA.Bounds()
					return core.ImageSource{Pixels: bannerRGBA,
						Size: core.Size{Width: core.Points(b.Dx()), Height: core.Points(b.Dy())}}
				},
				ResizeMode: core.Contain,
				Style:      core.Style{Width: core.Pct(100)},
			},

			core.View{Class: "gap-1.5 flex-col items-center", Children: []core.Node{
				core.Text{Content: "1,409,404 个 16×16 透明背景的合成游戏精灵图，覆盖 6,194 个主题分类，附带英文描述。", Style: core.Style{FontSize: 12, Color: core.RGB(0xa3a3a3), TextAlign: core.TextAlignCenter}},
				core.Text{Content: "首次部署需下载数据集（压缩包约 253 MB，来自 Hugging Face，CC0 1.0）并构建搜索索引（约 3–5 分钟），仅需一次。", Style: core.Style{FontSize: 12, Color: core.RGB(0xa3a3a3), TextAlign: core.TextAlignCenter}},
			}},

			// integrity problems found at startup (half downloads, half
			// built index), if any
			core.Show{When: func() bool { return s.problem != "" }, Then: func(cx *core.Scope) core.Node {
				return core.View{Class: "w-[560px] max-w-full rounded-lg border border-amber-800 bg-amber-950/40 px-3 py-2", Children: []core.Node{
					core.Text{Content: "检测到数据不完整：", Style: core.Style{FontSize: 11, Color: core.RGB(0xfbbf24)}},
					core.Text{Content: s.problem, Style: core.Style{FontSize: 11, Color: core.RGB(0xfbbf24)}},
				}}
			}},

			// deploy button / progress / error
			core.Show{When: func() bool { return stateSig.Get() != "running" }, Then: func(cx *core.Scope) core.Node {
				label := "部署数据集（下载 · 解压 · 构建索引）"
				if dataset.Ready() {
					label = "构建搜索索引"
				}
				if stateSig.Get() == "error" {
					label = "重试部署"
				}
				return core.View{Class: "gap-2 flex-col items-center", Children: []core.Node{
					ui.Button{Text: label, OnPress: start},
					core.Show{When: func() bool { return stateSig.Get() == "error" }, Then: func(cx *core.Scope) core.Node {
						return core.Text{Content: progSig.Get().text, Style: core.Style{FontSize: 11, Color: core.RGB(0xf87171)}}
					}},
				}}
			}},
			core.Show{When: func() bool { return stateSig.Get() == "running" }, Then: func(cx *core.Scope) core.Node {
				return core.View{Class: "w-[480px] max-w-full gap-2 flex-col items-center", Children: []core.Node{
					core.Text{ContentFunc: func() string { return progSig.Get().text },
						Style: core.Style{FontSize: 12, Color: core.RGB(0xa3a3a3)}},
					// ui.Progress takes a 0..100 percentage (base.Progress
					// clamps), so scale the 0..1 fraction.
					ui.Progress{Value: func() float32 { return progSig.Get().frac * 100 },
						Indeterminate: func() bool { return progSig.Get().frac <= 0 },
						Style:         core.Style{Width: core.Pct(100)}},
				}}
			}},

			core.Text{Content: fmt.Sprintf("缓存位置：%s", dataset.Dir()), Style: core.Style{FontSize: 10, Color: core.RGB(0x525252)}},
		},
	}
}

// indexProgressText renders an indexer.Progress line. The metadata phase
// reports bytes (file size based); the write phase reports rows.
func indexProgressText(p indexer.Progress) string {
	if p.Note != "" {
		return p.Note
	}
	if p.Total > 0 {
		if p.Phase == "加载元数据" {
			return fmt.Sprintf("%s %s / %s", p.Phase, humanBytes(int64(p.Done)), humanBytes(int64(p.Total)))
		}
		return fmt.Sprintf("%s %d / %d", p.Phase, p.Done, p.Total)
	}
	if p.Done > 0 {
		return fmt.Sprintf("%s %d", p.Phase, p.Done)
	}
	return p.Phase
}

// indexFrac maps an indexer.Progress to 0..1 (0 = indeterminate).
func indexFrac(p indexer.Progress) float32 {
	if p.Total <= 0 {
		return 0
	}
	return min(float32(p.Done)/float32(p.Total), 1)
}

// progressText renders a download/extract Progress line.
func progressText(p dataset.Progress) string {
	switch p.Phase {
	case "下载":
		if p.Total > 0 && p.Done > 0 {
			return fmt.Sprintf("下载中 %s / %s", humanBytes(p.Done), humanBytes(p.Total))
		}
		if p.Note != "" {
			return p.Note
		}
		return "下载中…"
	case "解压":
		if p.Total > 0 {
			return fmt.Sprintf("解压中 %s / %s", humanBytes(p.Done), humanBytes(p.Total))
		}
		return "解压中…"
	default:
		if p.Note != "" {
			return p.Note
		}
		return p.Phase
	}
}

// fracOf maps a done/total pair to 0..1 (0 = indeterminate).
func fracOf(done, total int64) float32 {
	if total <= 0 {
		return 0
	}
	return min(float32(done)/float32(total), 1)
}

// humanBytes formats a byte count for display.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
