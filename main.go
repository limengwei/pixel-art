// Command pixelart is PixelIcon: a pixel-art icon browser over the Pixel
// Art Atlas dataset (tokens.npy rendered on demand, search index built by
// the first-launch deploy), with an embedded MCP server for searching and
// exporting icons. Run with --mcp for a standalone stdio MCP server.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"

	"pixelart/internal/dataset"
	"pixelart/internal/mcp"
	"pixelart/internal/npy"
	"pixelart/internal/store"
	"pixelart/internal/ui"
)

func main() {
	// The search index lives next to the dataset in the cache dir; the
	// first-launch welcome screen offers a one-click deploy that downloads
	// the dataset and builds the index.
	if err := os.MkdirAll(filepath.Dir(dataset.DBPath()), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "创建数据目录失败: %v\n", err)
	}
	os.Remove(dataset.PartialZipPath()) // clean a half download left by a killed run
	st, err := store.Open(dataset.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	// stdio MCP boots synchronously (no UI to show a splash).
	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		var ds *npy.Dataset
		if ok, problems := dataset.Check(); ok {
			if d, err := npy.Open(dataset.Dir()); err == nil {
				ds = d
			}
		} else {
			fmt.Fprintf(os.Stderr, "[数据集] 文件不完整：%s\n", strings.Join(problems, "；"))
		}
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

	// The GUI opens immediately with a splash; the integrity check and
	// dataset open run in the background and call OnBooted (which starts
	// the embedded MCP HTTP server with the opened dataset).
	nsgo.Run(func(app *core.Scope) {
		desktop.OpenWindow(app, desktop.WindowOptions{
			Title: "PixelIcon · 像素图标库", Width: 1280, Height: 800, Key: "main",
		}, func(cx *core.Scope) core.Node {
			return ui.Main(cx, &ui.Deps{Store: st, OnBooted: func(ds *npy.Dataset) {
				go func() {
					addr := fmt.Sprintf("127.0.0.1:%d", port)
					if err := mcp.ServeHTTP(addr, st, ds); err != nil {
						fmt.Fprintf(os.Stderr, "[MCP] %v\n", err)
					}
				}()
			}})
		})
	}, nsgo.Appearance(core.AppearanceDark))
}
