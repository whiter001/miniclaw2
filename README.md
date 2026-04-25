# MiniClaw (Go)

MiniClaw 的 Go 单二进制实现，优先保证可编译、可部署、可在服务器上长期运行。

项目约束：

- 只依赖 Go 标准库
- 不要求额外安装 `curl`、`rg`、`grep`、`uvx`、`npx`
- 核心能力以单二进制 + 本地文件状态为中心
- 外部 MCP 或命令型工具是可选扩展，不影响主流程

## 能力概览

| 模块 | 状态 | 说明 |
| --- | --- | --- |
| Agent | 已实现 | MiniMax Anthropic-compatible `messages` 调用，多轮工具循环 |
| Gateway / QQ | 已实现 | webhook、鉴权、事件解析、回复回传 |
| Gateway / Weixin | 已实现 | 长轮询、二维码登录、账号持久化、媒体回复 |
| Workspace / Memory | 已实现 | session、memory、state 落盘 |
| Skills / Auto Skills | 已实现 | workspace skills、autoskill、评分回写、QQ / 微信显式 skill 管理 |
| Native MCP | 已实现 | `web_search`、`understand_image` |
| 外部 MCP / Command Tools | 已实现 | 从 `mcp_config_path` 加载 stdio MCP 或命令工具 |
| Cron | 未纳入首版 | `cron/` 目录保留，真实调度逻辑仍未落地 |

## 配置方式

配置优先级从高到低：

1. 命令行参数
2. 环境变量
3. `~/.config/miniclaw/config`
4. 内置默认值

仓库内的示例文件：

- `.env.example`：环境变量模板，适合 shell 或 systemd `EnvironmentFile`
- `docs/local-weixin.md`：本地直接运行微信通道的说明
- `docs/deployment.md`：部署说明汇总，包含 systemd、并行部署和 Podman Alpine
- `docs/skills.md`：skills、autoskill、评分和 QQ / 微信 skill 命令说明
- `Containerfile.alpine`：基于 Alpine 的 Podman 镜像定义
- `examples/miniclaw.config.example`：配置文件模板
- `examples/mcp.json.example`：stdio MCP + command tools 示例
- `examples/mcp.mmx.json.example`：只启用 `mmx` 的精简 MCP 示例
- `examples/deploy.podman.alpine.qq.env.example`：Podman Alpine QQ 部署模板
- `examples/deploy.podman.alpine.weixin.env.example`：Podman Alpine 微信部署模板
- `examples/miniclaw.weixin.service.example`：Linux systemd 微信服务模板

默认路径：

- 配置文件：`~/.config/miniclaw/config`
- MCP 配置：`~/.config/miniclaw/mcp.json`
- 工作目录：`~/.miniclaw/workspace`

说明：

- 程序本身不会自动加载 `.env` 文件
- 本地调试可用 `set -a && source ./.env && set +a`
- 服务器部署推荐用 systemd `EnvironmentFile=/etc/miniclaw/miniclaw.env`

## 快速开始

### 1. 构建与测试

```bash
go test ./...
./build.sh
```

构建 Linux 服务器版本：

```bash
GOOS=linux GOARCH=amd64 ./build.sh
```

如果目标机器是 ARM64，把 `GOARCH=amd64` 换成 `GOARCH=arm64`。

### 2. 初始化

```bash
./miniclaw onboard
./miniclaw status
```

### 3. 准备配置

本地 shell 方式：

```bash
cp .env.example .env
set -a && source ./.env && set +a
./miniclaw status
```

如果你更偏好配置文件，也可以直接参考 `examples/miniclaw.config.example` 填写 `~/.config/miniclaw/config`。

### 4. 验证 Agent

```bash
./miniclaw agent -p "hello"
```

如果你要在本机直接跑微信通道，优先看 [docs/local-weixin.md](docs/local-weixin.md)。这份文档会避开当前默认指向远端 `bl` 的部署脚本。

## 服务器端部署

完整部署说明已经迁移到 [docs/deployment.md](docs/deployment.md)。

这份文档现在集中维护以下内容：

- SSH + systemd 脚本部署
- QQ 和微信并行部署
- 手工 systemd 部署
- Podman Alpine 容器部署

常用入口：

- 脚本部署：`MINICLAW_GATEWAY_CHANNEL=weixin ./scripts/deploy_bl.sh`（默认走 Podman Alpine）
- 远端 SSH + systemd：`MINICLAW_DEPLOY_MODE=remote-systemd MINICLAW_GATEWAY_CHANNEL=weixin ./scripts/deploy_bl.sh`
- 显式容器部署：`./scripts/deploy_podman_alpine.sh`

如果你要查部署变量、目录布局、首发微信扫码、并行部署约束或 Podman 示例，直接看 [docs/deployment.md](docs/deployment.md)。

## 微信通道说明

如果你的目标是在本机直接把微信通道跑起来，而不是部署到远端服务器，先看 [docs/local-weixin.md](docs/local-weixin.md)。

当前微信实现对齐 `@tencent-weixin/openclaw-weixin` 的 backend API 协议，不依赖 Node 插件宿主。

已支持：

- `ilink/bot/getupdates` 长轮询收消息
- `ilink/bot/sendmessage` 文本回复
- `ilink/bot/getuploadurl` + CDN 上传媒体
- 二维码登录、账号保存、账号切换、账号删除
- 用户白名单
- 消息去重、会话记录、处理中占位回复

当前限制：

- 未实现 typing 状态
- 多账号路由策略仍是基础版本
- 非文本入站消息解析还不完整

媒体回复约定：

- 如果 Agent 最终回复包含 `MEDIA:/absolute/path/to/file`
- 或包含 `MEDIA:https://...`
- 程序会先发文字，再上传并发送媒体消息

常用命令：

```bash
./miniclaw gateway login --channel weixin
./miniclaw gateway accounts --channel weixin
./miniclaw gateway accounts --channel weixin --use bot-1
./miniclaw gateway logout --channel weixin --account bot-1
./miniclaw gateway --channel weixin --once
./miniclaw gateway --channel weixin
```

## QQ 通道说明

QQ 通道默认走 webhook 模式，适合公网入口明确、可配置回调地址的场景。

支持：

- access token 获取
- bot profile 查询
- webhook 验证
- 单聊 / 群聊事件解析
- 白名单控制
- 消息去重
- 处理中占位回复
- Agent 执行后回传结果

启动：

```bash
./miniclaw gateway --channel qq --once
./miniclaw gateway --channel qq
```

## MCP 与外部工具

### 内建原生 MCP

启用 `enable_mcp=true` 或命令行加 `--mcp` 后，会暴露：

- `web_search`
- `understand_image`

### 可选 stdio MCP

如果 `mcp_config_path` 指向的 JSON 文件存在，程序会加载其中声明的 `type: "stdio"` 服务。示例见 `examples/mcp.json.example`。

### 命令型外部工具

`mcp_config_path` 同时支持 `type: "command"` 的普通命令行工具。适合接入 `mmx` 这类 CLI。

模板变量支持：

- `{{query}}`
- `{{prompt}}`
- `{{image_source}}`
- `{{MINICLAW_API_KEY}}`
- `{{MINICLAW_REGION}}`
- `{{MINICLAW_WORKSPACE}}`

对于 `mmx`，MiniClaw 会自动补上更适合 agent/CI 的默认参数，例如 `--non-interactive` 和 `--quiet`。

如果你只想启用 `mmx`，直接使用 `examples/mcp.mmx.json.example` 即可。

## Skills

MiniClaw 当前已经支持：

- `workspace/skills/` 下的手工 skill 装载
- 基于成功 session 自动沉淀 autoskill
- skill 评分与排序
- 在 QQ / 微信聊天里通过 `/skill` 命令做显式管理

详细说明见 [docs/skills.md](docs/skills.md)。

## Workspace 结构

`miniclaw onboard` 会初始化：

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

支持的子命令：

- `miniclaw memory show`
- `miniclaw memory set -p "..."`
- `miniclaw memory append -p "..."`
- `miniclaw memory today -p "..."`
- `miniclaw memory summarize [days]`
- `miniclaw memory compact`
- `miniclaw memory prune [days]`
- `miniclaw memory clear`

这个实现依赖兼容 `openclaw-weixin` backend API 的服务端接口。仓库当前已包含 Go 侧二维码登录、账号持久化和基础媒体上传，但不包含独立的微信托管后端。

## 开发说明

当前实现优先保证：

- 与参考仓库的命令和配置尽量兼容
- 以标准库为核心，减少环境摩擦
- 在 Windows 下可直接开发和验证

仓库根目录下还自动放置了一个 `.env` 占位文件，方便本地填入 MiniMax / QQ 相关变量，但真正运行仍以配置文件和环境变量覆盖规则为准。
