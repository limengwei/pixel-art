package ui

import (
	"strconv"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// settingsPanel is a modal overlay editing the app's settings: the MCP port.
// The search index is embedded in the app and the dataset lives in a fixed
// cache directory, so neither is configurable anymore.
func settingsPanel(cx *core.Scope, deps *Deps, toaster *ui.Toaster, onClose func()) core.Node {
	st := deps.Store
	port := core.NewSignal(cx, st.GetSetting("mcp_port", "9394"))

	save := func() {
		if p, err := strconv.Atoi(port.Get()); err == nil && p > 0 && p < 65536 {
			st.SetSetting("mcp_port", port.Get())
		}
		onClose()
		toaster.Show(ui.ToastContent{Type: ui.ToastSuccess, Title: "设置已保存",
			Description: "MCP 端口重启后生效"})
	}

	return core.View{Class: "absolute inset-0 bg-black/60 z-50 flex-row items-center justify-center", Children: []core.Node{
		core.View{Class: "w-[460px] rounded-xl bg-neutral-900 border border-neutral-700 p-5 gap-4 flex-col", Children: []core.Node{
			core.Text{Content: "设置", Style: core.Style{FontSize: 16}},

			settingField("MCP HTTP 端口（重启后生效）", port),

			core.View{Class: "flex-row justify-end gap-2", Children: []core.Node{
				ui.Button{Text: "取消", OnPress: onClose},
				ui.Button{Text: "保存", OnPress: save},
			}},
		}},
	}}
}

func settingField(label string, sig *core.Signal[string]) core.Node {
	return core.View{Class: "gap-1 flex-col", Children: []core.Node{
		core.Text{Content: label, Style: core.Style{FontSize: 11, Color: core.RGB(0xa3a3a3)}},
		ui.Input{Value: sig.Get, OnChange: sig.Set, Style: core.Style{FontSize: 12}},
	}}
}
