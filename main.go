// Command pixelart is PixelIcon: a pixel-art icon browser over the Pixel
// Art Atlas dataset (tokens.npy rendered on demand, search index built by
// the first-launch deploy), with an embedded MCP server for searching and
// exporting icons. Run with --mcp for a standalone stdio MCP server.
package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"

	"pixelart/internal/dataset"
	"pixelart/internal/mcp"
	"pixelart/internal/npy"
	"pixelart/internal/store"
	"pixelart/internal/ui"
)

//go:embed assets/IconWindows.png
var trayIcon []byte

// singleInstanceAddr is the local lock the GUI holds to stay a single
// instance: the first process listens on it, later ones ask it (over the
// same socket) to show the running window and exit.
const singleInstanceAddr = "127.0.0.1:9396"

// acquireSingleInstance holds the singleton lock. When another instance
// already runs it pokes that instance to show its window and reports false.
// activate is invoked (from a goroutine) whenever a later instance asks;
// release frees the lock.
func acquireSingleInstance(activate func()) (release func(), ok bool) {
	ln, err := net.Listen("tcp", singleInstanceAddr)
	if err != nil {
		// Tell the running instance to come to the front.
		if c, derr := net.DialTimeout("tcp", singleInstanceAddr, time.Second); derr == nil {
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			_, _ = c.Write([]byte("show\n"))
			_ = c.Close()
		}
		return nil, false
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return // released or closing
			}
			go func() {
				line, _ := bufio.NewReader(c).ReadString('\n')
				_ = c.Close()
				if strings.TrimSpace(line) == "show" {
					activate()
				}
			}()
		}
	}()
	return func() { _ = ln.Close() }, true
}

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

	// Only one GUI instance: a later start asks the running one to show its
	// window and exits instead of launching a second copy.
	activateCh := make(chan struct{}, 1)
	releaseSingleton, first := acquireSingleInstance(func() {
		select {
		case activateCh <- struct{}{}:
		default:
		}
	})
	if !first {
		return
	}

	// The GUI opens immediately with a splash; the integrity check and
	// dataset open run in the background and call OnBooted (which starts
	// the embedded MCP HTTP server with the opened dataset).
	nsgo.Run(func(app *core.Scope) {
		win := desktop.OpenWindow(app, desktop.WindowOptions{
			Title: "PixelIcon · 像素图标库", Width: 1280, Height: 800, Key: "main",
		}, func(cx *core.Scope) core.Node {
			return ui.Main(cx, &ui.Deps{Store: st, ReleaseSingleton: releaseSingleton,
				OnBooted: func(ds *npy.Dataset) {
					go func() {
						addr := fmt.Sprintf("127.0.0.1:%d", port)
						if err := mcp.ServeHTTP(addr, st, ds); err != nil {
							fmt.Fprintf(os.Stderr, "[MCP] %v\n", err)
						}
					}()
				}})
		})

		// A later instance asked us to come to the front.
		go func() {
			for range activateCh {
				app.Post(win.Show)
			}
		}()

		// Closing the window hides it to the tray instead of quitting, so the
		// embedded MCP server keeps serving external clients; the tray menu
		// shows the window or quits.
		win.OnClosing(app, func(c *core.WindowClosing) {
			c.Cancel()
			win.Hide()
		})
		desktop.NewTray(app, desktop.TrayOptions{
			Icon:    trayIcon,
			Tooltip: fmt.Sprintf("PixelIcon · 像素图标库 · MCP :%d", port),
			OnClick: win.Show,
			Menu: func() []core.MenuItem {
				return []core.MenuItem{
					{Label: "显示窗口", OnSelect: win.Show},
					{Separator: true},
					{Label: "退出", OnSelect: app.Quit},
				}
			},
		})
	}, nsgo.Appearance(core.AppearanceDark))
}
