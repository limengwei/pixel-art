package ui

import (
	"fmt"
	"image"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/store"
)

// robotSVG is the toolbar MCP button's icon (explicit fill, not tinted).
var robotSVG = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" fill="#E5E5E5" viewBox="0 0 24 24"><path d="M13.5 2C13.5 2.44425 13.3069 2.84339 13 3.11805V5H18C19.6569 5 21 6.34315 21 8V18C21 19.6569 19.6569 21 18 21H6C4.34315 21 3 19.6569 3 18V8C3 6.34315 4.34315 5 6 5H11V3.11805C10.6931 2.84339 10.5 2.44425 10.5 2C10.5 1.17157 11.1716 0.5 12 0.5C12.8284 0.5 13.5 1.17157 13.5 2ZM0 10H2V16H0V10ZM24 10H22V16H24V10ZM9 14.5C9.82843 14.5 10.5 13.8284 10.5 13C10.5 12.1716 9.82843 11.5 9 11.5C8.17157 11.5 7.5 12.1716 7.5 13C7.5 13.8284 8.17157 14.5 9 14.5ZM16.5 13C16.5 12.1716 15.8284 11.5 15 11.5C14.1716 11.5 13.5 12.1716 13.5 13C13.5 13.8284 14.1716 14.5 15 14.5C15.8284 14.5 16.5 13.8284 16.5 13Z"/></svg>`))

// toolbarState is the top bar's inputs.
type toolbarState struct {
	query      *core.Signal[string]
	onChange   func(string)
	onSubmit   func()
	onSettings func()
}

// toolbar renders the search box and the MCP settings button (robot icon
// plus the label).
func toolbar(cx *core.Scope, s toolbarState) core.Node {
	return core.View{Class: "p-3 gap-2 flex-row items-center", Children: []core.Node{
		core.View{Class: "flex-1 min-w-0", Children: []core.Node{
			ui.Input{
				Value:       s.query.Get,
				OnChange:    s.onChange,
				OnSubmit:    func(string) { s.onSubmit() },
				Placeholder: "搜索图标（中英文均可，如 battery / 电池）…",
				AutoFocus:   true,
			},
		}},
		ui.Press{
			OnPress: s.onSettings,
			Label:   "MCP 设置",
			Style: core.Style{Padding: core.Symmetric(6, 12), CornerRadius: 6,
				Background: core.RGB(0x262626), Direction: core.Row, Gap: 6,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			StyleFunc: func() core.Style {
				return core.Style{Hover: &core.Style{Background: core.RGB(0x2a2a2e)}}
			},
			Children: []core.Node{
				core.Icon{SVG: robotSVG, Style: core.Style{Width: core.Pt(18), Height: core.Pt(18)}},
				core.Text{Content: "MCP", Style: core.Style{FontSize: 12, Color: core.RGB(0xe5e5e5)}},
			},
		},
	}}
}

// maxGridRows caps what the grid claims as its count. NSGo's List is
// validated in the hundreds-to-thousands range (its DataTable exists for
// millions), so the browser never grows past this; deeper results need a
// keyword. The pager still serves any of the first maxGridRows rows.
const maxGridRows = 100_000

// gridState feeds the virtualized grid from the pager.
type gridState struct {
	total    *core.Signal[int] // size of the whole result set
	refresh  *core.Signal[int] // bumped when a page arrives
	ref      *core.Ref         // the list's scroll handle
	row      func(i int) (store.Icon, bool)
	ensure   func(page int)
	onOpen   func(store.Icon)
	spriteOf func(id int64) *image.RGBA
}

// iconGrid renders the virtualized grid. Count is the result total capped
// at maxGridRows; rows resolve through the pager's page cache, so
// scrolling never accumulates memory. Cells whose page is still loading
// show a placeholder and trigger the load.
func iconGrid(cx *core.Scope, s gridState) core.Node {
	cell := func(i int) core.Node {
		return ui.Press{
			OnPress: func() {
				if ic, ok := s.row(i); ok {
					s.onOpen(ic)
				}
			},
			// The cell fills its (flexible) column width so window resizes
			// and the detail panel never overlap cells; the image inside
			// scales down with Contain.
			Style: core.Style{Width: core.Pct(100), Height: core.Pt(cellSize), CornerRadius: 8,
				Background: core.RGB(0x1c1c1e), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			StyleFunc: func() core.Style {
				return core.Style{Hover: &core.Style{Background: core.RGB(0x2a2a2e)}}
			},
			Children: []core.Node{
				core.Image{
					SourceFunc: func() core.ImageSource {
						s.refresh.Get() // depend on page arrivals
						ic, ok := s.row(i)
						if !ok {
							s.ensure(i / pageSize)
							return core.ImageSource{}
						}
						img := s.spriteOf(ic.ID)
						if img == nil {
							return core.ImageSource{}
						}
						return core.ImageSource{Pixels: img, Size: core.Size{Width: core.Points(img.Bounds().Dx()), Height: core.Points(img.Bounds().Dy())}}
					},
					Sampling:   core.ImageSamplingNearest,
					ResizeMode: core.Contain,
					Style:      core.Style{Width: core.Pct(100), Height: core.Pct(100)},
				},
			},
		}
	}
	return core.View{Class: "flex-1 min-h-0", Children: []core.Node{base.List{
		Columns:       10,
		EstimatedSize: cellSize + 12,
		Gap:           8,
		Ref:           s.ref,
		ListStyle:     core.Style{Padding: core.All(12)},
		// The outer view must be flex-bounded or base.List grows to its
		// content height and the list virtualizes every row.
		Style: core.Style{FlexGrow: 1, FlexBasis: core.Pct(0), FlexShrink: 1, Width: core.Pct(100)},
		Count: func() int { return min(s.total.Get(), maxGridRows) },
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			s.refresh.Get() // depend on page arrivals
			i := row.Index()
			if _, ok := s.row(i); !ok {
				s.ensure(i / pageSize)
			}
			return cell(i)
		},
		Empty: core.Text{Content: "暂无图标", Style: core.Style{FontSize: 13, Color: core.RGB(0x737373)}},
	}}}
}

// repoURL is the project's GitHub repository, opened in the system browser.
const repoURL = "https://github.com/limengwei/pixel-art"

// githubSVG is GitHub's octocat mark (Simple Icons' SiGithub path).
var githubSVG = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#8a8a8e" d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg>`))

// statusBar shows the result count and the browse cap on the left, and the
// GitHub repository entry on the right.
func statusBar(cx *core.Scope, total *core.Signal[int]) core.Node {
	return core.View{Class: "px-3 py-2 border-t border-neutral-800 flex-row items-center justify-between text-xs text-neutral-500", Children: []core.Node{
		core.Text{ContentFunc: func() string {
			n := total.Get()
			if n > maxGridRows {
				return fmt.Sprintf("共 %d 个图标 · 浏览前 %d 个，输入关键词查看其余结果", n, maxGridRows)
			}
			return fmt.Sprintf("共 %d 个图标 · 虚拟滚动按需加载", n)
		}},
		ui.Press{
			OnPress: func() { cx.OpenURL(repoURL, nil) },
			Label:   "GitHub 仓库",
			Style:   core.Style{Padding: core.All(4), CornerRadius: 4},
			StyleFunc: func() core.Style {
				return core.Style{Hover: &core.Style{Background: core.RGB(0x262626)}}
			},
			Children: []core.Node{
				core.Icon{SVG: githubSVG, Style: core.Style{Width: core.Pt(16), Height: core.Pt(16)}},
			},
		},
	}}
}
