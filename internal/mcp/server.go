// Package mcp exposes the icon store as Model Context Protocol tools:
// searching icons, reading one icon's details, and exporting PNGs rendered
// on demand from the dataset.
package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"pixelart/internal/exporter"
	"pixelart/internal/npy"
	"pixelart/internal/store"
)

// New builds the MCP server with all tools registered. ds may be nil, in
// which case export_icon reports the dataset as unavailable.
func New(st *store.Store, ds *npy.Dataset) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "pixel-art", Version: "1.2.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_icons",
		Description: "搜索像素图标。支持中文或英文关键词（匹配主体、英文描述与中文标签），可按主体过滤，分页返回。返回每个图标的 id、subject 和 caption。",
	}, searchIcons(st))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_icon",
		Description: "按 id 获取一个图标的详情：主体、完整描述、原始 16x16 像素尺寸与数据集行号。",
	}, getIcon(st))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "export_icon",
		Description: "导出图标为 PNG 文件（从数据集按需渲染，最近邻采样保持像素锐利）。可指定尺寸与输出目录，返回导出文件的绝对路径。",
	}, exportIconTool(st, ds))

	return server
}

// ParamDoc describes one tool parameter for display in the settings panel.
type ParamDoc struct{ Name, Desc string }

// ToolDoc describes one MCP tool for display in the settings panel.
type ToolDoc struct {
	Name   string
	Desc   string
	Params []ParamDoc
}

// ToolsDoc documents every tool the server exposes, mirroring the
// registrations in New.
var ToolsDoc = []ToolDoc{
	{
		Name:   "search_icons",
		Desc:   "搜索像素图标，支持中英文关键词（主体、英文描述与中文标签），分页返回 id、subject 和 caption。",
		Params: []ParamDoc{
			{Name: "query", Desc: "搜索关键词，中英文均可"},
			{Name: "subject", Desc: "可选，按主体过滤，如 'aa battery'"},
			{Name: "page", Desc: "可选，页码，从 1 开始"},
			{Name: "pageSize", Desc: "可选，每页数量，默认 20"},
		},
	},
	{
		Name:   "get_icon",
		Desc:   "按 id 获取一个图标的详情：主体、完整描述与数据集行号。",
		Params: []ParamDoc{
			{Name: "id", Desc: "图标 id（search_icons 返回的数字）"},
		},
	},
	{
		Name:   "export_icon",
		Desc:   "导出图标为 PNG 文件（最近邻采样保持像素锐利），返回导出文件的绝对路径。",
		Params: []ParamDoc{
			{Name: "id", Desc: "图标 id"},
			{Name: "size", Desc: "可选，导出尺寸（像素），0 表示原始 16x16"},
			{Name: "directory", Desc: "可选，输出目录，默认临时目录"},
		},
	},
}

type searchArgs struct {
	Query    string `json:"query" jsonschema:"搜索关键词，中英文均可"`
	Subject  string `json:"subject,omitempty" jsonschema:"按主体过滤，如 'aa battery'"`
	Page     int    `json:"page,omitempty" jsonschema:"页码，从 1 开始"`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"每页数量，默认 20"`
}

func searchIcons(st *store.Store) func(context.Context, *mcp.CallToolRequest, searchArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in searchArgs) (*mcp.CallToolResult, any, error) {
		page, size := in.Page, in.PageSize
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 100 {
			size = 20
		}
		res := st.Search(in.Query, in.Subject, page, size)
		text := fmt.Sprintf("共 %d 个图标", res.Total)
		if in.Subject != "" {
			text += fmt.Sprintf("（主体 %q）", in.Subject)
		}
		pages := (res.Total + size - 1) / size
		text += fmt.Sprintf("，第 %d/%d 页：\n", page, pages)
		for _, ic := range res.Icons {
			text += fmt.Sprintf("- id=%d | %s | %s\n", ic.ID, ic.Subject, ic.Caption)
		}
		if res.Total > page*size {
			text += fmt.Sprintf("\n还有更多结果，可用 page=%d 获取下一页。", page+1)
		}
		return textResult(text), nil, nil
	}
}

type getIconArgs struct {
	ID int64 `json:"id" jsonschema:"图标 id（search_icons 返回的数字）"`
}

func getIcon(st *store.Store) func(context.Context, *mcp.CallToolRequest, getIconArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in getIconArgs) (*mcp.CallToolResult, any, error) {
		ic, ok := st.GetIcon(in.ID)
		if !ok {
			return errorResult(fmt.Sprintf("未找到 id=%d 的图标", in.ID)), nil, nil
		}
		text := fmt.Sprintf("id: %d\n主体: %s\n描述: %s\n数据集行: tokens.npy 第 %d 行（16x16 原始像素）",
			ic.ID, ic.Subject, ic.Caption, ic.ID)
		return textResult(text), nil, nil
	}
}

type exportIconArgs struct {
	ID        int64  `json:"id" jsonschema:"图标 id"`
	Size      int    `json:"size,omitempty" jsonschema:"导出尺寸（像素），0 表示原始 16x16"`
	Directory string `json:"directory,omitempty" jsonschema:"输出目录，默认临时目录"`
}

func exportIconTool(st *store.Store, ds *npy.Dataset) func(context.Context, *mcp.CallToolRequest, exportIconArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, in exportIconArgs) (*mcp.CallToolResult, any, error) {
		if ds == nil {
			return errorResult("数据集（tokens.npy）不可用，无法渲染图标"), nil, nil
		}
		ic, ok := st.GetIcon(in.ID)
		if !ok {
			return errorResult(fmt.Sprintf("未找到 id=%d 的图标", in.ID)), nil, nil
		}
		img, err := ds.Row(in.ID)
		if err != nil {
			return errorResult(err.Error()), nil, nil
		}
		name := fmt.Sprintf("%s_%07d", sanitizeName(ic.Subject), ic.ID)
		path, err := exporter.ExportPNG(img, name, in.Size, in.Directory)
		if err != nil {
			return errorResult(err.Error()), nil, nil
		}
		return textResult(path), nil, nil
	}
}

// sanitizeName makes a subject safe for a file name.
func sanitizeName(s string) string {
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

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// ServeHTTP runs the MCP streamable HTTP server on addr (e.g.
// "127.0.0.1:9394") until the process exits. It is meant to run in a
// goroutine next to the desktop UI.
func ServeHTTP(addr string, st *store.Store, ds *npy.Dataset) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return New(st, ds) }, nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("MCP HTTP 监听失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[MCP] listening on http://%s/mcp\n", addr)
	return http.Serve(ln, mux)
}

// ServeStdio runs the MCP server over stdio for `--mcp` mode.
func ServeStdio(st *store.Store, ds *npy.Dataset) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintf(os.Stderr, "pixel-art MCP (stdio): %d icons indexed\n", st.TotalIcons())
	return New(st, ds).Run(ctx, &mcp.StdioTransport{})
}

// PortFromSettings reads the configured port, defaulting to 9394.
func PortFromSettings(st *store.Store) int {
	v := st.GetSetting("mcp_port", "9394")
	p, err := strconv.Atoi(v)
	if err != nil || p <= 0 || p > 65535 {
		return 9394
	}
	return p
}
