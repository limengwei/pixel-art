package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"pixelart/internal/mcp"
)

// settingsPanel is a centered modal overlay editing the app's settings: the
// MCP HTTP port, plus the MCP service's address, client config and tool
// documentation. The search index and dataset live in fixed cache
// locations, so neither is configurable.
func settingsPanel(cx *core.Scope, deps *Deps, toaster *ui.Toaster, open *core.Signal[bool]) core.Node {
	st := deps.Store
	// The port the running server answers on; a saved change only takes
	// effect after a restart, so it stays what the app started with.
	effective := mcp.PortFromSettings(st)
	portSig := core.NewSignal(cx, strconv.Itoa(effective))
	restartSig := core.NewSignal(cx, false)

	mcpAddr := fmt.Sprintf("http://127.0.0.1:%d/mcp", effective)
	mcpConfig := fmt.Sprintf("{\n  \"mcpServers\": {\n    \"pixel-art\": {\n      \"url\": \"%s\"\n    }\n  }\n}", mcpAddr)

	// copyChip renders a small pressable chip that copies src to the
	// clipboard and toasts about it.
	copyChip := func(label string, src func() string) core.Node {
		return ui.Press{
			OnPress: func() {
				cx.SetClipboard(src())
				toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: "已复制", Description: src()})
			},
			Label: label,
			Style: core.Style{Padding: core.Symmetric(2, 8), CornerRadius: 4, Background: core.RGB(0x262626)},
			StyleFunc: func() core.Style {
				return core.Style{Hover: &core.Style{Background: core.RGB(0x2a2a2e)}}
			},
			Children: []core.Node{core.Text{Content: label, Style: core.Style{FontSize: 10, Color: core.RGB(0xa3a3a3)}}},
		}
	}

	validPort := func() (int, bool) {
		p, err := strconv.Atoi(strings.TrimSpace(portSig.Get()))
		return p, err == nil && p >= 1 && p <= 65535
	}
	portValid := func() bool {
		_, ok := validPort()
		return ok
	}

	save := func() {
		p, ok := validPort()
		if !ok {
			return
		}
		if err := st.SetSetting("mcp_port", strconv.Itoa(p)); err != nil {
			toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: "保存失败", Description: err.Error()})
			return
		}
		// The panel stays open under the restart dialog; 稍后 closes both.
		restartSig.Set(true)
	}

	// restart spawns the next process and quits this one, handing the
	// singleton lock over first so the new instance can take it.
	restart := func() {
		restartSig.Set(false)
		if deps.ReleaseSingleton != nil {
			deps.ReleaseSingleton()
		}
		exe, err := os.Executable()
		if err == nil {
			if err := exec.Command(exe, os.Args[1:]...).Start(); err == nil {
				cx.Quit()
				return
			}
		}
		toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: "重启失败", Description: "请手动重启应用"})
		open.Set(false)
	}

	restartLater := func() {
		restartSig.Set(false)
		open.Set(false)
	}

	labelStyle := core.Style{FontSize: 11, Color: core.RGB(0xa3a3a3)}
	monoValue := core.Style{FontSize: 11, FontFamily: core.FontMonospace,
		Color: core.RGB(0xe5e5e5), Background: core.RGB(0x18181b),
		Padding: core.Symmetric(6, 8), CornerRadius: 6}

	// ui.Dialog rides the driver's modal layer, which blocks input to the
	// app behind it — a hand-rolled absolute overlay lets clicks through.
	return core.View{Children: []core.Node{
		// Saving the port asks for a restart; the panel stays under it
		// until answered.
		ui.AlertDialog{
			Open: restartSig.Get,
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					restartLater()
				}
			},
			Title:       "重启应用",
			Description: "MCP 端口已保存，重启应用后生效。",
			Actions: []ui.AlertAction{
				{Text: "稍后", OnPress: restartLater},
				{Text: "立即重启", Default: true, OnPress: restart},
			},
		},
		ui.Dialog{
			Open: open.Get,
			OnOpenChange: func(o bool, _ base.Reason) {
				if !o {
					open.Set(false)
				}
			},
			Title: "设置",

			// core.Scroll with HideIndicator: the dialog scrollbar would
			// cover the copy chips at the rows' right edge.
			Children: []core.Node{
				core.Scroll{
					HideIndicator: true,
					Style:         core.Style{Width: core.Pct(100), MinHeight: core.Pt(0), MaxHeight: core.Pt(520), Gap: 16},
					Children: []core.Node{
						// MCP port.
						core.View{Class: "gap-1 flex-col", Children: []core.Node{
							core.Text{Content: "MCP HTTP 端口", Style: labelStyle},
							ui.Input{Value: portSig.Get, OnChange: portSig.Set, Style: core.Style{FontSize: 12}},
							core.Show{When: func() bool { return !portValid() }, Then: func(cx *core.Scope) core.Node {
								return core.Text{Content: "端口须为 1–65535 的整数", Style: core.Style{FontSize: 10, Color: core.RGB(0xf87171)}}
							}},
							core.Text{Content: fmt.Sprintf("当前生效端口：%d（重启应用后切换）", effective),
								Style: core.Style{FontSize: 10, Color: core.RGB(0x737373)}},
						}},

						// MCP service: address and client config (both copyable),
						// plus the tool docs.
						core.View{Class: "gap-2 flex-col pt-3 border-t border-neutral-800", Children: []core.Node{
							core.Text{Content: "MCP 服务（随应用自动启动，关闭窗口后托盘常驻）", Style: core.Style{FontSize: 12}},
							core.View{Class: "gap-1 flex-col", Children: []core.Node{
								core.View{Class: "flex-row items-center justify-between", Children: []core.Node{
									core.Text{Content: "HTTP 地址", Style: core.Style{FontSize: 10, Color: core.RGB(0x737373)}},
									copyChip("复制", func() string { return mcpAddr }),
								}},
								core.Text{Content: mcpAddr, Style: monoValue},
							}},
							core.View{Class: "gap-1 flex-col", Children: []core.Node{
								core.View{Class: "flex-row items-center justify-between", Children: []core.Node{
									core.Text{Content: "MCP 配置", Style: core.Style{FontSize: 10, Color: core.RGB(0x737373)}},
									copyChip("复制", func() string { return mcpConfig }),
								}},
								core.View{Style: core.Style{Background: core.RGB(0x18181b), Padding: core.All(8), CornerRadius: 6},
									Children: mcpConfigLines(mcpAddr)},
							}},
							core.Text{Content: "可用方法", Style: core.Style{FontSize: 10, Color: core.RGB(0x737373)}},
							core.View{Class: "gap-2 flex-col", Children: toolDocs()},
						}},

						// Read-only app information.
						core.View{Class: "gap-2 flex-col pt-3 border-t border-neutral-800", Children: []core.Node{
							core.Text{Content: fmt.Sprintf("图标总数：%d · 分类数：%d", st.TotalIcons(), len(st.Subjects())), Style: labelStyle},
							core.View{Class: "gap-1 flex-col", Children: []core.Node{
								core.Text{Content: "数据来源", Style: core.Style{FontSize: 10, Color: core.RGB(0x737373)}},
								core.Text{Content: "https://huggingface.co/datasets/Angais/pixel-art-atlas-1.4m", Style: core.Style{FontSize: 10, Color: core.RGB(0x737373)}},
							}},
						}},
					},
				},
			},

			Footer: []core.Node{
				ui.Button{Text: "取消", OnPress: func() { open.Set(false) }},
				ui.Button{Text: "保存", Disabled: func() bool { return !portValid() }, OnPress: save},
			},
		},
	}}
}

// mcpConfigLines renders the example MCP client config, one monospace Text
// per line (multi-line strings are not a Text's to promise).
func mcpConfigLines(addr string) []core.Node {
	lines := []string{
		"{",
		`  "mcpServers": {`,
		`    "pixel-art": {`,
		fmt.Sprintf(`      "url": "%s"`, addr),
		"    }",
		"  }",
		"}",
	}
	style := core.Style{FontSize: 11, FontFamily: core.FontMonospace, Color: core.RGB(0xe5e5e5)}
	out := make([]core.Node, 0, len(lines))
	for _, l := range lines {
		out = append(out, core.Text{Content: l, Style: style})
	}
	return out
}

// toolDocs renders the MCP tools' name, purpose and parameters as compact
// cards.
func toolDocs() []core.Node {
	out := make([]core.Node, 0, len(mcp.ToolsDoc))
	for _, t := range mcp.ToolsDoc {
		rows := []core.Node{
			core.Text{Content: t.Name, Style: core.Style{FontSize: 11, FontFamily: core.FontMonospace, Color: core.RGB(0xe5e5e5)}},
			core.Text{Content: t.Desc, Style: core.Style{FontSize: 10, Color: core.RGB(0xa3a3a3)}},
		}
		for _, p := range t.Params {
			rows = append(rows, core.Text{Content: fmt.Sprintf("· %s：%s", p.Name, p.Desc),
				Style: core.Style{FontSize: 10, Color: core.RGB(0x737373), Padding: core.Symmetric(0, 8)}})
		}
		out = append(out, core.View{Class: "gap-1 flex-col",
			Style:    core.Style{Background: core.RGB(0x18181b), Padding: core.All(8), CornerRadius: 6},
			Children: rows})
	}
	return out
}
