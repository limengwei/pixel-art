package ui

import (
	"fmt"
	"image"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/store"
)

// toolbarState is the top bar's inputs.
type toolbarState struct {
	query      *core.Signal[string]
	onChange   func(string)
	onSubmit   func()
	onSettings func()
}

// toolbar renders the search box and action buttons.
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
		ui.Button{Text: "设置", OnPress: s.onSettings},
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

// statusBar shows the result count and the browse cap.
func statusBar(cx *core.Scope, total *core.Signal[int]) core.Node {
	return core.View{Class: "px-3 py-2 border-t border-neutral-800 text-xs text-neutral-500", Children: []core.Node{
		core.Text{ContentFunc: func() string {
			n := total.Get()
			if n > maxGridRows {
				return fmt.Sprintf("共 %d 个图标 · 浏览前 %d 个，输入关键词查看其余结果", n, maxGridRows)
			}
			return fmt.Sprintf("共 %d 个图标 · 虚拟滚动按需加载", n)
		}},
	}}
}
