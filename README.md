# pixel-art · 像素图标库

基于 [Pixel Art Atlas](https://huggingface.co/datasets/Angais/pixel-art-atlas-1.4m) 数据集的像素图标浏览器与检索工具。140 万个 16×16 合成游戏精灵图，内置预构建的中文搜索索引，支持 MCP 协议供外部工具（如 IDE、AI 助手）直接检索和导出图标。

## 功能

- **图标浏览**：虚拟滚动网格，按需从 `tokens.npy` 渲染精灵图，流畅浏览全部 1,409,404 个图标
- **中英文搜索**：内置 SQLite + FTS5 全文索引，英文描述与中文标签均可检索（如 `battery` / `电池`）
- **分类侧栏**：6,194 个主题分类，中英双语显示，支持中文过滤
- **详情与导出**：任意尺寸放大预览，PNG 一键导出
- **MCP 服务**：内嵌 MCP 服务器（stdio + HTTP），供 AI 工具搜索、导出图标

## 数据集（pixel-art-16x16-1.4m）

应用仅支持这一个数据集：

- 1,409,404 个 16×16 透明背景的合成游戏精灵图，覆盖 6,194 个主题分类，附带英文描述
- 图像由 Krea 2 Turbo 与像素画 LoRA 从文本提示生成，以 CC0 1.0 协议发布
- `tokens.npy` 存储 256 个调色板索引（行主序），索引 0 为透明，其余索引从 `palette.npy` 取 RGB 颜色
- 描述为生成时的提示词，与图像内容大致对应但不完全精确（颜色、数量、细节可能有出入），使用时请以图像为准
- 原始数据集共 1,412,908 张，过滤后保留 1,405,308 张训练图与 4,096 张验证图

## 首次启动

首次打开（或数据不完整）时会出现欢迎页，展示数据集横幅与介绍。点击「部署数据集」，应用会一键完成三步（仅需一次，数据集已存在时自动跳过前两步）：

1. 从 Hugging Face 下载数据集压缩包（约 253 MB，解压后约 770 MB）
2. 自动解压到缓存目录
3. 构建搜索索引（约 3–5 分钟，索引约 211 MB）

```
https://huggingface.co/datasets/Angais/pixel-art-atlas-1.4m/resolve/main/pixel-art-16x16-1.4m.zip?download=true
```

部署完成后即可正常浏览。数据集与索引的固定位置：

| 内容 | 路径（Windows） |
| --- | --- |
| 搜索索引（部署时构建） | `%LocalAppData%\pixelart\icons.db` |
| 数据集目录 | `%LocalAppData%\pixelart\pixel-art-16x16-1.4m\` |

应用不再支持其他数据集目录。若索引或数据集缺失，启动时会重新检测并展示欢迎页，重新点「部署数据集」即可修复（重建索引是幂等的）。部署完成后会生成 `manifest.json`（SHA-256 清单）用于后续启动的内容校验；老版本部署的数据在下次启动通过大小校验后会自动补生成。

## MCP 服务

启动后自动在本机开启 MCP HTTP 服务（默认端口 9394，工具栏「MCP」按钮打开设置页，可查看/复制接入地址、客户端 JSON 配置与工具文档，端口可修改、重启生效），另可用 `pixel-art.exe --mcp` 以 stdio 模式运行。提供图标搜索与导出工具，供支持 MCP 的客户端接入。

**关闭主窗口不会退出程序**：应用最小化到系统托盘继续提供 MCP 服务，单击托盘图标恢复窗口，托盘右键菜单可选择「显示窗口」或「退出」。

## 构建与开发

```bash
go build -trimpath -ldflags "-s -w" -o pixel-art.exe .
go test ./internal/...
```

### 重建索引

搜索索引由 `internal/indexer` 从数据集的 `captions.json` / `objects.json` 构建（INSERT OR REPLACE，幂等可重跑）。删除缓存中的 `icons.db` 后重启应用，欢迎页会引导重新部署；也可以在测试中直接调用 `indexer.Run` 构建到指定数据库。

## 许可

- 代码：本项目自有版权
- 数据集：[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/)，由 Angais 发布；精灵图为 AI 生成物，不含模型权重
