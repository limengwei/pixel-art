package ui

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/store"
	"pixelart/internal/translate"
)

// sidebarState is the left column's inputs.
type sidebarState struct {
	subjects *core.Signal[[]store.Subject]
	allCount *core.Signal[int]
	filter   *core.Signal[string]
	subject  *core.Signal[string]
	onPick   func(string)
}

// sidebar renders the subject list with counts and a filter box.
func sidebar(cx *core.Scope, s sidebarState) core.Node {
	return core.View{Class: "w-60 flex-col border-r border-neutral-800 bg-neutral-900 min-h-0", Children: []core.Node{
		core.View{Class: "p-3 gap-2 flex-col", Children: []core.Node{
			ui.Press{
				OnPress: func() { s.onPick("") },
				Style: core.Style{Width: core.Pct(100), Padding: core.Symmetric(5, 8), CornerRadius: 6,
					Background: core.RGB(0x262626), FontSize: 13, TextAlign: core.TextAlignLeft},
				StyleFunc: func() core.Style {
					if s.subject.Get() == "" {
						return core.Style{Background: core.RGB(0x3f3f46)}
					}
					return core.Style{}
				},
				Children: []core.Node{
					core.Text{ContentFunc: func() string {
						return fmt.Sprintf("全部图标（%d）", s.allCount.Get())
					}},
				},
			},
			ui.Input{
				Value:       s.filter.Get,
				OnChange:    s.filter.Set,
				Placeholder: "过滤分类…",
			},
		}},
		core.View{Class: "flex-1 min-h-0", Children: []core.Node{subjectList(s)}},
	}}
}

func subjectList(s sidebarState) base.List {
	visible := func() []store.Subject { return visibleSubjects(s.subjects.Get(), s.filter.Get()) }
	return base.List{
		Count:         func() int { return len(visible()) },
		EstimatedSize: 28,
		Gap:           2,
		ListStyle:     core.Style{Padding: core.All(4)},
		// Flex-bound so the list virtualizes instead of growing to its
		// content height (see iconGrid).
		Style: core.Style{FlexGrow: 1, FlexBasis: core.Pct(0), FlexShrink: 1, Width: core.Pct(100)},
		Empty: core.Text{Content: "无匹配分类", Style: core.Style{FontSize: 12, Color: core.RGB(0x737373)}},
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			return ui.Press{
				OnPress: func() { s.onPick(visible()[row.Index()].Name) },
				Style: core.Style{Width: core.Pct(100), Padding: core.Symmetric(4, 8),
					CornerRadius: 6, Background: core.RGB(0x262626)},
				StyleFunc: func() core.Style {
					if s.subject.Get() == visible()[row.Index()].Name {
						return core.Style{Background: core.RGB(0x3f3f46)}
					}
					return core.Style{}
				},
				Children: []core.Node{
					core.Text{ContentFunc: func() string {
						name := visible()[row.Index()].Name
						if zh := translate.Subject(name); zh != "" {
							return name + "(" + zh + ")"
						}
						return name
					},
						Style: core.Style{FontSize: 12, TextOverflow: core.EllipsizeTail, Color: core.RGB(0xd4d4d4)}},
					core.Text{ContentFunc: func() string { return fmt.Sprintf("(%d)", visible()[row.Index()].Count) },
						Style: core.Style{FontSize: 10, Color: core.RGB(0x737373)}},
				},
			}
		},
		OnActivate: func(i int) { s.onPick(visible()[i].Name) },
	}
}
