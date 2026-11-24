package ui

import (
	"fmt"
	"image"
	"path/filepath"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/exporter"
	"pixelart/internal/store"
)

// detailState is the right-hand detail panel's inputs.
type detailState struct {
	icon     *core.Signal[store.Icon]
	size     *core.Signal[int]
	toaster  *ui.Toaster
	st       *store.Store
	onClose  func()
	spriteOf func(id int64) *image.RGBA
}

var exportSizes = []int{16, 32, 48, 64, 128, 256, 512}

// detailPanel shows one icon large with its metadata and export actions.
// Export goes through the system save dialog (cx.SaveFile), remembering
// the last folder it saved in.
func detailPanel(cx *core.Scope, s detailState) core.Node {
	doExport := func() {
		ic := s.icon.Get()
		img := s.spriteOf(ic.ID)
		if img == nil {
			s.toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: "导出失败", Description: "数据集不可用"})
			return
		}
		name := fmt.Sprintf("%s_%07d", sanitizeFileName(ic.Subject), ic.ID)
		data, fileName, err := exporter.PNG(img, name, s.size.Get())
		if err != nil {
			s.toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: "导出失败", Description: err.Error()})
			return
		}
		cx.SaveFile(core.SaveOptions{
			Name:  fileName,
			MIME:  "image/png",
			Data:  data,
			Start: s.st.GetSetting("last_save_dir", ""),
		}, func(saved bool, path string, err error) {
			switch {
			case err != nil:
				s.toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: "导出失败", Description: err.Error()})
			case saved:
				if dir := filepath.Dir(path); dir != "" {
					s.st.SetSetting("last_save_dir", dir)
				}
				s.toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: "已导出", Description: path, Duration: 8 * time.Second})
			}
		})
	}

	return core.View{Class: "w-80 flex-col border-l border-neutral-800 bg-neutral-900", Children: []core.Node{
		// Header.
		core.View{Class: "p-3 flex-row items-center justify-between border-b border-neutral-800", Children: []core.Node{
			core.Text{Content: "图标详情", Style: core.Style{FontSize: 14}},
			ui.Button{Text: "关闭", OnPress: s.onClose},
		}},

		// Body.
		core.View{Class: "flex-1 min-h-0 overflow-y-auto p-4 gap-4 flex-col", Children: []core.Node{
			core.View{Class: "flex-row justify-center", Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(256), Height: core.Pt(256), CornerRadius: 12,
					Background: core.RGB(0x1c1c1e), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{
					core.Image{
						SourceFunc: func() core.ImageSource {
							ic := s.icon.Get()
							img := s.spriteOf(ic.ID)
							if img == nil {
								return core.ImageSource{}
							}
							return core.ImageSource{Pixels: img, Size: core.Size{Width: core.Points(img.Bounds().Dx()), Height: core.Points(img.Bounds().Dy())}}
						},
						Sampling:   core.ImageSamplingNearest,
						ResizeMode: core.Contain,
						Style:      core.Style{Width: core.Pt(224), Height: core.Pt(224)},
					},
				}},
			}},

			infoRows(s),

			core.Text{Content: "导出尺寸", Style: core.Style{FontSize: 12, Color: core.RGB(0xa3a3a3)}},
			core.View{Class: "flex-row gap-2 flex-wrap", Children: sizeButtons(s)},

			core.View{Class: "gap-2 flex-col", Children: []core.Node{
				ui.Button{TextFunc: func() string {
					return fmt.Sprintf("导出 PNG（%dpx）", s.size.Get())
				}, OnPress: doExport},
				ui.Button{Text: "复制图片", OnPress: func() {
					ic := s.icon.Get()
					img := s.spriteOf(ic.ID)
					if img == nil {
						s.toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: "复制失败", Description: "数据集不可用"})
						return
					}
					data, err := exporter.EncodePNG(img)
					if err != nil {
						s.toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: "复制失败", Description: err.Error()})
						return
					}
					cx.SetClipboardImage(data)
					s.toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: "已复制图片"})
				}},
				ui.Button{Text: "复制 ID", OnPress: func() {
					cx.SetClipboard(fmt.Sprintf("%d", s.icon.Get().ID))
					s.toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: "已复制 ID"})
				}},
			}},
		}},
	}}
}

func infoRows(s detailState) core.Node {
	row := func(label string, valueFunc func() string) core.Node {
		return core.View{Class: "gap-1 flex-col", Children: []core.Node{
			core.Text{Content: label, Style: core.Style{FontSize: 10, Color: core.RGB(0x737373)}},
			core.Text{ContentFunc: valueFunc, Style: core.Style{FontSize: 12, TextOverflow: core.EllipsizeTail, Color: core.RGB(0xe5e5e5)}},
		}}
	}
	return core.View{Class: "gap-3 flex-col", Children: []core.Node{
		core.Text{ContentFunc: func() string {
			i := s.icon.Get()
			if i.Subject == "" && i.Caption == "" {
				return ""
			}
			return i.Caption
		}, Style: core.Style{FontSize: 14}},
		row("主体 Subject", func() string { return s.icon.Get().Subject }),
		row("ID（数据集行号）", func() string { return fmt.Sprintf("%d", s.icon.Get().ID) }),
		row("源", func() string { return "tokens.npy 按需解码（16×16）" }),
	}}
}

func sizeButtons(s detailState) []core.Node {
	out := make([]core.Node, 0, len(exportSizes))
	for _, size := range exportSizes {
		size := size
		out = append(out, ui.Press{
			OnPress: func() { s.size.Set(size) },
			Style: core.Style{Padding: core.Symmetric(4, 10), CornerRadius: 6,
				Background: core.RGB(0x262626), FontSize: 12},
			StyleFunc: func() core.Style {
				if s.size.Get() == size {
					return core.Style{Background: core.RGB(0x3f3f46), Color: core.RGB(0xffffff)}
				}
				return core.Style{}
			},
			Children: []core.Node{core.Text{Content: fmt.Sprintf("%d", size)}},
		})
	}
	return out
}

// sanitizeFileName makes a subject safe for a file name.
func sanitizeFileName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case ' ', '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			out = append(out, '_')
		default:
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "icon"
	}
	return string(out)
}
