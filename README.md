# MiniClaw (Go)

用 Go 标准库重写的 `MiniClaw`，对齐参考仓库 `d:\work\github\miniclaw` 当前已实现的核心功能：

- `onboard` / `status` / `agent` / `gateway` / `memory`
- MiniMax Anthropic-compatible `messages` 调用
- 多轮工具循环：`list_dir`、`read_file`、`write_file`、`exec`、`grep_search`
- workspace / session / memory 持久化
- QQ gateway / webhook / 白名单 / 去重 / 处理中占位回复
- 内建原生 MCP：`web_search`、`understand_image`
- 可选 stdio MCP 与命令型外部工具扩展（读取 `mcp_config_path` 指向的 JSON 配置）

实现约束：

- **不安装第三方 Go 依赖**
- 默认只用 Go 标准库
- 核心功能不依赖 `curl`、`rg`、`grep`、`uvx`、`npx`
- 如果你配置了额外 stdio MCP 服务，那些服务本身仍然需要你自己提供可执行文件

## 当前状态

已完成：

- Go 项目骨架与可编译单二进制入口
- 配置加载、环境变量覆盖、workspace 初始化
- session JSONL 记录
- 三层记忆系统与 `memory` 子命令
- 本地工具执行与安全边界
- MiniMax Agent loop
- QQ bootstrap / webhook / 事件处理
- 原生 MCP manager + 内建 MCP 工具
- 基础单元测试

暂未纳入首版范围：

- `cron` 调度器的真实执行能力
- `skills` 目录的实际技能装载逻辑

这两项在参考仓库里也属于预留目录，不是当前已落地的主功能。

## 构建与测试

```powershell
go test ./...
go build -o miniclaw.exe ./cmd/miniclaw
```

## 快速开始

1. 构建二进制
2. 运行 `miniclaw onboard`
3. 编辑 `~/.config/miniclaw/config`
4. 配置 `api_key` 和 QQ 相关字段
5. 运行 `miniclaw status` 检查状态

常用命令：

```powershell
miniclaw onboard
miniclaw status
miniclaw agent -p "hello"
miniclaw memory show
miniclaw gateway --once
```

## 配置文件

示例配置见：

- `examples/miniclaw.config.example`
- `examples/mcp.json.example`
- `examples/mcp.mmx.json.example`

默认路径：

- 配置：`~/.config/miniclaw/config`
- MCP 配置：`~/.config/miniclaw/mcp.json`
- 工作区：`~/.miniclaw/workspace`

## Workspace 结构

`miniclaw onboard` 会初始化这些目录和文件：

```text
workspace/
	sessions/
	memory/
		MEMORY.md
		SUMMARY.md
	state/
	cron/
	skills/
	AGENTS.md
	USER.md
	HEARTBEAT.md
```

## 记忆系统

支持的命令：

- `miniclaw memory show`
- `miniclaw memory set -p "..."`
- `miniclaw memory append -p "..."`
- `miniclaw memory today -p "..."`
- `miniclaw memory summarize [days]`
- `miniclaw memory compact`
- `miniclaw memory prune [days]`
- `miniclaw memory clear`

## MCP

### 内建原生 MCP

启用 `enable_mcp=true` 或命令行加 `--mcp` 后，会默认暴露两个内建工具：

- `web_search`：通过标准库 HTTP 发起网页搜索并整理结果
- `understand_image`：读取本地/远程图片，并调用 MiniMax 描述图片

### 可选 stdio MCP

如果 `mcp_config_path` 指向的 JSON 存在，程序会尝试加载其中声明的 `type: "stdio"` MCP 服务。示例见 `examples/mcp.json.example`。

这条能力是**可选扩展**：

- 不配置也不影响核心功能
- 配置了就会尝试启动外部 MCP 进程

### 命令型外部工具

`mcp_config_path` 现在还支持声明 `type: "command"` 的外部工具。它们不是 MCP server，而是普通命令行程序，MiniClaw 会：

- 在 agent 执行时把它们注册成工具
- 用当前 workspace 作为工作目录启动命令
- 支持在 `args` / `env` 中使用模板变量，例如：
  - `{{query}}`
  - `{{prompt}}`
  - `{{image_source}}`
  - `{{MINICLAW_API_KEY}}`
  - `{{MINICLAW_REGION}}`
  - `{{MINICLAW_WORKSPACE}}`

适合接入像 `https://github.com/MiniMax-AI/cli` 这样的普通外部 CLI。

对于 `mmx`，MiniClaw 会优先补齐 agent/CI 友好的默认参数，例如 `--non-interactive`、`--quiet`，并默认继承**当前 MiniClaw 生效配置**：

- API Key：优先使用 `MINICLAW_API_KEY`，否则使用 `config` 里的 `api_key`
- Region：根据当前 MiniClaw 的 `base_url` / `api_url` 自动推导（`minimaxi.com` → `cn`，否则 `global`）

随后再补齐 `--api-key` / `--region`，避免命令卡在交互提示里，也避免误用 `mmx` 自己的本地持久化配置。

### MiniMax CLI (`mmx`) 集成

当前示例配置已经内置三条 `mmx` 外部工具示例：

- `mmx_search`
- `mmx_image`
- `mmx_vision_describe`
- `mmx_quota`

如果你只想启用 `mmx`，推荐直接使用 `examples/mcp.mmx.json.example`；它不会顺带尝试启动其他 stdio MCP 进程。

准备方式：

```powershell
npm install -g mmx-cli
mmx auth login --api-key sk-xxxxx
```

示例默认使用 `--api-key` / `--region` / `--non-interactive` / `--quiet`，这样 `mmx` 在 agent 场景下不会弹交互提示，也不会因为读取环境变量而顺手把 key 落到 `~/.mmx/config.json`。

说明：

- `mmx search query --q ... --output json`
- `mmx image generate --prompt ... --output json`
- `mmx vision describe --image ... --prompt ... --output json`
- `mmx quota show --output json`

都可以直接通过 `examples/mcp.json.example` 里的 `type: "command"` 配置暴露给 agent。

其中 `mmx_image` 走的是上游官方 `mmx image generate` 命令，默认只要求 `prompt`，适合让 agent 直接返回生成图片的 URL 结果。

如果你想固定宽高比、批量张数或输出目录，也可以在这个示例基础上自行复制一份工具定义，并在 `args` 里追加固定参数，例如 `--aspect-ratio 16:9`、`--n 3`、`--out-dir ./minimax-output`。

## QQ Gateway

支持：

- access token 获取
- bot profile 查询
- webhook 验证
- 单聊 / 群聊事件解析
- 白名单控制
- 消息去重
- 处理中占位回复
- Agent 执行后回传结果

启动方式：

```powershell
miniclaw gateway --once
miniclaw gateway
```

`--once` 只做 bootstrap，不启动本地 webhook 服务。

## 开发说明

当前实现优先保证：

- 与参考仓库的命令和配置尽量兼容
- 以标准库为核心，减少环境摩擦
- 在 Windows 下可直接开发和验证

仓库根目录下还自动放置了一个 `.env` 占位文件，方便本地填入 MiniMax / QQ 相关变量，但真正运行仍以配置文件和环境变量覆盖规则为准。
