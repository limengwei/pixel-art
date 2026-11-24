package ui

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/dataset"
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

// welcomeState feeds the first-launch overlay.
type welcomeState struct {
	onReady func() // dataset became ready: reopen and refresh
}

// welcomePanel is shown when the dataset is not installed yet: banner,
// Chinese introduction, and the one-shot download + extract.
func welcomePanel(cx *core.Scope, s welcomeState) core.Node {
	stateSig := core.NewSignal(cx, "idle") // idle | running | error
	progSig := core.NewSignal(cx, dataset.Progress{})

	start := func() {
		if stateSig.Get() == "running" {
			return
		}
		stateSig.Set("running")
		go func() {
			err := dataset.Download(context.Background(), func(p dataset.Progress) {
				cx.Post(func() { progSig.Set(p) })
			})
			cx.Post(func() {
				if err != nil {
					stateSig.Set("error")
					progSig.Set(dataset.Progress{Phase: "失败", Note: err.Error()})
					return
				}
				s.onReady()
			})
		}()
	}

	return core.View{Class: "absolute inset-0 bg-neutral-950 z-40 flex-col items-center justify-center gap-5 p-6", Children: []core.Node{
		core.View{Class: "w-[720px] max-w-full rounded-2xl border border-neutral-800 bg-neutral-900 p-6 gap-5 flex-col items-center", Children: []core.Node{

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

			core.Text{Content: "欢迎来到 PixelIcon · 像素图标库", Style: core.Style{FontSize: 20}},

			core.View{Class: "gap-1.5 flex-col items-center", Children: []core.Node{
				core.Text{Content: "1,409,404 个 16×16 透明背景的合成游戏精灵图，覆盖 6,194 个主题分类，附带英文描述。", Style: core.Style{FontSize: 12, Color: core.RGB(0xa3a3a3), TextAlign: core.TextAlignCenter}},
				core.Text{Content: "数据集来自 Hugging Face（CC0 1.0 协议），约 780 MB；下载到应用缓存目录并自动解压，仅需一次。", Style: core.Style{FontSize: 12, Color: core.RGB(0xa3a3a3), TextAlign: core.TextAlignCenter}},
			}},

			// download button / progress / error
			core.Show{When: func() bool { return stateSig.Get() != "running" }, Then: func(cx *core.Scope) core.Node {
				label := "下载数据集（约 780 MB）"
				if stateSig.Get() == "error" {
					label = "重试下载"
				}
				return core.View{Class: "gap-2 flex-col items-center", Children: []core.Node{
					ui.Button{Text: label, OnPress: start},
					core.Show{When: func() bool { return stateSig.Get() == "error" }, Then: func(cx *core.Scope) core.Node {
						return core.Text{Content: progSig.Get().Note, Style: core.Style{FontSize: 11, Color: core.RGB(0xf87171)}}
					}},
				}}
			}},
			core.Show{When: func() bool { return stateSig.Get() == "running" }, Then: func(cx *core.Scope) core.Node {
				p := progSig.Get()
				return core.View{Class: "w-[480px] max-w-full gap-2 flex-col items-center", Children: []core.Node{
					core.Text{Content: progressText(p), Style: core.Style{FontSize: 12, Color: core.RGB(0xa3a3a3)}},
					ui.Progress{Value: func() float32 {
						q := progSig.Get()
						if q.Total <= 0 {
							return 0
						}
						v := float32(q.Done) / float32(q.Total)
						if v > 1 {
							v = 1
						}
						return v
					}},
				}}
			}},

			core.Text{Content: fmt.Sprintf("缓存位置：%s", dataset.Dir()), Style: core.Style{FontSize: 10, Color: core.RGB(0x525252)}},
		}},
	}}
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
