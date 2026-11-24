// Command pixelart is PixelIcon: a pixel-art icon browser over the Pixel
// Art Atlas dataset (tokens.npy rendered on demand, captions indexed into
// SQLite), with an embedded MCP server for searching and exporting icons.
// Run with --mcp for a standalone stdio MCP server.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"

	"pixelart/internal/indexer"
	"pixelart/internal/mcp"
	"pixelart/internal/npy"
	"pixelart/internal/store"
	"pixelart/internal/ui"
)

func main() {
	if err := os.MkdirAll(filepath.Dir(indexer.DBPath()), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "创建数据目录失败: %v\n", err)
	}
	st, err := store.Open(indexer.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	metaDir := st.GetSetting("meta_dir", indexer.DefaultMetaDir)
	dataset, dsErr := npy.Open(metaDir)
	if dsErr != nil {
		fmt.Fprintf(os.Stderr, "[数据集] %v（仅影响图片显示与导出）\n", dsErr)
	}

	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		if err := mcp.ServeStdio(st, dataset); err != nil {
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
		if err := mcp.ServeHTTP(addr, st, dataset); err != nil {
			fmt.Fprintf(os.Stderr, "[MCP] %v\n", err)
		}
	}()

	nsgo.Run(func(app *core.Scope) {
		desktop.OpenWindow(app, desktop.WindowOptions{
			Title: "PixelIcon · 像素图标库", Width: 1280, Height: 800, Key: "main",
		}, func(cx *core.Scope) core.Node {
			return ui.Main(cx, &ui.Deps{Store: st, Dataset: dataset, MetaDir: metaDir})
		})
	}, nsgo.Appearance(core.AppearanceDark))
}
