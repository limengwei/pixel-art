// Command pixelart is PixelIcon: a pixel-art icon browser over the Pixel
// Art Atlas dataset (tokens.npy rendered on demand, a prebuilt search
// index embedded in the binary), with an embedded MCP server for searching
// and exporting icons. Run with --mcp for a standalone stdio MCP server.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"

	"pixelart/internal/dataset"
	"pixelart/internal/index"
	"pixelart/internal/mcp"
	"pixelart/internal/npy"
	"pixelart/internal/store"
	"pixelart/internal/ui"
)

func main() {
	// The search index ships inside the binary; extract it once.
	dbPath, err := index.Ensure()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化内置索引失败: %v\n", err)
		os.Exit(1)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	// The dataset (pixel-art-16x16-1.4m) lives in a fixed cache directory;
	// the first-launch welcome screen offers to download it.
	var ds *npy.Dataset
	if dataset.Ready() {
		if d, err := npy.Open(dataset.Dir()); err == nil {
			ds = d
		} else {
			fmt.Fprintf(os.Stderr, "[数据集] %v（仅影响图片显示与导出）\n", err)
		}
	}

	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		if err := mcp.ServeStdio(st, ds); err != nil {
			fmt.Fprintf(os.Stderr, "MCP stdio: %v\n", err)
			os.Exit(1)
		}
		return
	}

	port := mcp.PortFromSettings(st)
	for _, arg := range os.Args[1:] {
		if p, err := strconv.Atoi(arg); err == nil && p > 0 {
			port = p
		}
	}

	// Embedded MCP HTTP server for external clients.
	go func() {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		if err := mcp.ServeHTTP(addr, st, ds); err != nil {
			fmt.Fprintf(os.Stderr, "[MCP] %v\n", err)
		}
	}()

	nsgo.Run(func(app *core.Scope) {
		desktop.OpenWindow(app, desktop.WindowOptions{
			Title: "PixelIcon · 像素图标库", Width: 1280, Height: 800, Key: "main",
		}, func(cx *core.Scope) core.Node {
			return ui.Main(cx, &ui.Deps{Store: st, Dataset: ds})
		})
	}, nsgo.Appearance(core.AppearanceDark))
}
