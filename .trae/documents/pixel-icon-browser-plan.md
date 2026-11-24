# PixelIcon 像素图标浏览客户端 - 实施计划

## Context

基于 NSGo 框架（github.com/nsgo-dev/nsgo v0.3.0，Go 原生 UI 引擎）开发一个像素图标浏览桌面客户端，替代参考项目 iconstore（Wails3+Vue，位于 D:\workspace\go\src\iconstore）的技术栈。数据源为 Pixel Art Atlas 1.4M 数据集：

- **图标目录**：`D:\workspace\game\morizhanche\assets\icons\all`（48×48 PNG，当前 ~55 万文件且**持续增长中**，最终 1,409,404 个）
- **文件名模式**：`{subject}_{row_id:08d}_{截断描述}.png`，如 `aa_battery_0001332_brand_new_aa_battery.png`
- **元数据**：`C:\Users\Administrator\Downloads\pixel-art-16x16-1.4m\captions.json` + `objects.json`（平行数组，**已验证** row_id=数组下标，提供完整英文 caption 和 subject，共 6,194 个 subject）
- 用户确认：MCP 内嵌 HTTP（桌面应用启动即提供）+ `--mcp` stdio 模式；图标目录/元数据目录可配置；中文 UI

## 技术要点（已验证）

| 项 | 结论 |
|---|---|
| Go 工具链 | 本机 go1.25.3 + GOTOOLCHAIN=auto，go1.27.4 已缓存，满足 nsgo 要求的 1.27.3+ |
| SQLite | `modernc.org/sqlite`（纯 Go 无 CGO），WAL 模式 |
| MCP | 直接用 `github.com/modelcontextprotocol/go-sdk` v1.8.0（nsgo 同版本依赖）：`mcp.NewServer` + `mcp.AddTool` + `StreamableHTTPHandler`（HTTP）/ `mcp.StdioTransport`（stdio） |
| 虚拟网格 | `base.List{Count, Row, Key, Columns, EstimatedSize, OnLoadMore, Empty}`（base\list.go，列数 Columns int） |
| 搜索框 | `ui.Input{Value func() string, OnChange func(string), OnSubmit, Placeholder}` |
| 图片 | `core.Image{Source: core.ImageSource{URI: "file:///D:/.../xx.png"}}`（core/fileurl.go 支持 file: URL；用 `url.URL{Scheme:"file", Path: filepath.ToSlash(p)}` 构造并转义空格）；`cx.SetImageCacheLimit` 调大缓存 |
| 剪贴板 | `cx.SetClipboard(text)` 复制路径、`cx.SetClipboardImage(pngBytes)` 复制图片（core/services.go:130,140） |
| 导出缩放 | `golang.org/x/image/draw.NearestNeighbor`（像素画必须最近邻） |
| 中文搜索 | 索引时用 iconstore 的 en→zh 词典（icon_translate.go 约 900 词条）把 subject/caption 词翻译拼接为 zh 标签存 FTS |

## 文件布局（D:\workspace\go\src\pixel-art，module 名 `pixelart`）

```
main.go                    入口：--mcp/--port 参数分发；nsgo.Run(App, WindowSize(1280,800), Appearance(dark))
internal/store/store.go    Open(dbPath)：建表/FTS/触发器/迁移；设置读写（meta 表）
internal/store/search.go   Search(q, subject, page, size)、GetIcon(id)、Subjects()（分类计数）、DeleteNotIn
internal/indexer/parse.go  文件名解析 regexp `^(.+?)_(\d{8})(?:_(.*))?\.png$`
internal/indexer/meta.go   流式加载 captions.json/objects.json（json.Decoder 按元素解码，~80MB 内存）
internal/indexer/indexer.go Run()：后台增量索引，Progress{Phase,Done,Total} 回调
internal/translate/dict.go 从 iconstore icon_translate.go 复制 enToZh 词典 + translateTags()
internal/exporter/export.go Export(srcPath, size, outDir)：读 PNG→最近邻缩放→写文件
internal/mcp/server.go     go-sdk 封装：NewServer+3 工具；ServeHTTP(addr)、ServeStdio()
internal/ui/app.go         App 骨架：信号定义（query/subject/items/progress/detail/toast）+ 布局
internal/ui/sidebar.go     左侧分类列表（含分类过滤输入框）
internal/ui/grid.go        图标网格 + 搜索栏 + 进度条
internal/ui/detail.go      详情面板（大图预览 + 导出 + 复制）
internal/ui/settings.go    设置窗口（图标目录/元数据目录/MCP 端口）
internal/ui/toast.go       轻量 toast（信号驱动的悬浮 View）
```

## DB Schema

```sql
CREATE TABLE icons(
  id INTEGER PRIMARY KEY,                -- row_id (0..1409403)
  subject TEXT NOT NULL, filename TEXT NOT NULL,
  caption TEXT DEFAULT '', caption_zh TEXT DEFAULT '',
  file_size INTEGER DEFAULT 0, file_mtime INTEGER DEFAULT 0);
CREATE INDEX idx_icons_subject ON icons(subject);
CREATE VIRTUAL TABLE icons_fts USING fts5(
  subject, caption, caption_zh, content='icons', content_rowid='rowid');
-- ai/ad/au 三个同步触发器（照抄 iconstore icon_service.go 的 FTS 模式）
CREATE TABLE subjects(subject TEXT PRIMARY KEY, cnt INTEGER);  -- 收尾时 GROUP BY 重建
CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT);          -- 设置项 + last_scan + scan 计数
```

DB 路径：`%APPDATA%/pixelart/icons.db`（os.MkdirAll；用 os.UserConfigDir）。

## 索引流水线（应对 55万→141万持续增长）

1. 启动即 `go indexer.Run(...)`，不阻塞 UI；进度经 `progressSig` 更新（每 1 万行 Set 一次）。
2. 枚举：`os.ReadDir`（Go 原生，比 PowerShell 快得多）。首次全量；之后增量：与 DB 中 filename+mtime 对比（加载现有 filename→(id,mtime) map，一次 SQL），仅插入差异行。提取增长期每次启动自动 delta 扫（秒级）。
3. 批量写入：5000 行/事务 `INSERT OR REPLACE`（id=row_id 天然幂等）；caption 从 meta.go 内存数组查，缺失则用文件名残余部分；caption_zh = 词典翻译 tags 拼接。
4. 收尾：`DELETE FROM icons WHERE id NOT IN (本次扫描集合)`（分批 5 万删一次避免大事务）；`INSERT INTO subjects SELECT subject, count(*) ... GROUP BY`；记 last_scan。
5. UI「重新扫描」按钮 → 强制全量对比（含删除清理）。

## UI（中文、暗色，参考 iconstore 布局）

```
App(root) 1280×800 暗色
├─ 左侧栏 220px：「全部图标(N)」+ 分类过滤输入框 + base.List(纵向虚拟)
│    Row = 分类按钮（subject + 计数），选中 → subjectSig
├─ 主区
│    ├─ 工具栏：ui.Input 搜索（300ms 防抖 + OnSubmit 立即搜）| 重新扫描按钮 | 设置按钮
│    ├─ 进度条行：索引中 X/Y（ContentFunc 读 progressSig，扫完隐藏）
│    └─ base.List{Columns:10, EstimatedSize:96, Count:iconsSig,
│         OnLoadMore: 下一页(SQL OFFSET)，Key: id,
│         Row: ui.Image{Source: file URI} + 悬浮无 UI（点击 → detailSig）}
└─ 详情面板（点图标出现，右侧滑入或弹层）：
     256px 大图（最近邻）+ subject/caption/row_id/文件路径
     + 尺寸选择(48/64/128/256/512) + 「导出 PNG」(desktop 文件夹选择) + 「复制图片」+ 「复制路径」
Toast：操作反馈（已导出到 xxx / 已复制）
```

搜索：FTS5 `MATCH '"词"*'` 前缀匹配 + subject 过滤 + `ORDER BY rank LIMIT/OFFSET`（pageSize 200，OnLoadMore 追加到 iconsSig）。空查询按 id 顺序浏览。

## MCP 工具（内嵌 HTTP :9394 可配置 + --mcp stdio）

| 工具 | 参数 | 返回 |
|---|---|---|
| search_icons | query, subject, page, pageSize(默认20) | 文本列表：id / subject / caption |
| get_icon | id | caption、subject、文件绝对路径、尺寸信息 |
| export_icon | id, size(默认48), directory(默认临时目录) | 导出文件绝对路径（最近邻缩放） |

桌面模式：goroutine 起 `http.Server` 挂 `StreamableHTTPHandler`（127.0.0.1:9394）。stdio 模式：`--mcp` 参数，独立打开 DB 后 `mcp.StdioTransport` 长驻。

## 实施步骤

1. **脚手架**：`go mod init pixelart`、`go get github.com/nsgo-dev/nsgo@v0.3.0 modernc.org/sqlite github.com/modelcontextprotocol/go-sdk@v1.8.0 golang.org/x/image`、`go mod tidy`、跑通最小 nsgo 窗口（验证工具链切换）
2. **store + indexer**：建库建表、文件名解析、meta 加载、批量索引、增量/清理、翻译词典；单测：文件名解析、FTS 搜索（小样本目录）
3. **exporter + mcp**：最近邻导出、3 个 MCP 工具、HTTP/stdio 两种 transport；curl/echo 验证
4. **UI**：App 骨架 + 信号 → 侧栏/网格/搜索/进度 → 详情/导出/复制 → 设置窗口 + toast
5. **联调验证**（见下）

## 验证

```powershell
cd D:\workspace\go\src\pixel-art
go build ./...                        # 工具链自动切 go1.27.4
go test ./internal/...                # 解析/搜索单测
go run .                              # 桌面窗口：首扫进度、中文"电池"/英文"battery"均命中、侧栏分类过滤、滚动加载
# MCP stdio：
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}' | go run . --mcp
# MCP HTTP（先 initialize 拿 session id）：
curl -X POST http://127.0.0.1:9394/mcp -H "Content-Type: application/json" -d '{...tools/call search_icons...}'
```

清单：
- [ ] 首扫 55 万+文件 UI 不卡、进度条推进；中途 kill 重启续扫幂等
- [ ] 中英文搜索命中；侧栏 6194 分类计数正确；点击过滤生效
- [ ] 详情导出 512px 像素边缘锐利（最近邻）；复制图片/路径可用
- [ ] MCP 三工具 HTTP 与 stdio 均可用；export_icon 落盘正确
- [ ] 设置（目录/端口）持久化，重启生效；「重新扫描」能清理已删除文件
- [ ] 新文件（目录持续增长）启动时 delta 扫描秒级完成并出现在浏览中

## 风险与对策

- **file: URI 加载失败**（空格/特殊字符）：url.URL 构造自动转义；兜底方案 `os.ReadFile` + `ImageSource{Data}` + 自建 LRU
- **nsgo API 早期不稳定**：以模块源码签名（base\list.go、core\image.go、core\services.go）为准，不凭记忆写
- **扫描期间 DB 锁竞争**：WAL + busy_timeout 5000ms；索引 goroutine 独立连接，UI 查询只读
- **词典覆盖率有限**：zh 标签是增强非必需，缺失不阻塞索引
