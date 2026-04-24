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
| Native MCP | 已实现 | `web_search`、`understand_image` |
| 外部 MCP / Command Tools | 已实现 | 从 `mcp_config_path` 加载 stdio MCP 或命令工具 |
| Cron / Skills Loader | 未纳入首版 | 目录保留，真实调度与装载逻辑未落地 |

## 配置方式

配置优先级从高到低：

1. 命令行参数
2. 环境变量
3. `~/.config/miniclaw/config`
4. 内置默认值

仓库内的示例文件：

- `.env.example`：环境变量模板，适合 shell 或 systemd `EnvironmentFile`
- `examples/miniclaw.config.example`：配置文件模板
- `examples/mcp.json.example`：stdio MCP + command tools 示例
- `examples/mcp.mmx.json.example`：只启用 `mmx` 的精简 MCP 示例
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

## 服务器端部署

以下流程默认面向 Linux + systemd，并以微信通道为例。

### 使用部署脚本

如果目标机器已经能通过 SSH 访问，可以直接使用仓库里的部署脚本：

```bash
MINICLAW_GATEWAY_CHANNEL=qq ./scripts/deploy_bl.sh
MINICLAW_GATEWAY_CHANNEL=weixin ./scripts/deploy_bl.sh
```

脚本会自动完成这些步骤：

- 构建 Linux 二进制并同步到远端
- 在远端生成或复用 `MINICLAW_REMOTE_ENV_FILE` 指向的 env 文件
- 在远端执行一次 `miniclaw onboard`
- 安装 systemd 服务并按通道生成正确的 `ExecStart`
- 对 `qq` 执行 webhook 回调校验
- 对 `weixin` 执行 bootstrap 校验

首次部署微信时，如果远端还没有保存过账号，可显式开启登录流程：

```bash
MINICLAW_GATEWAY_CHANNEL=weixin MINICLAW_REMOTE_WEIXIN_LOGIN=1 ./scripts/deploy_bl.sh
```

这会在 SSH 会话中执行 `miniclaw gateway login --channel weixin`，打印二维码链接并等待扫码确认。

如果你希望把这套变量固化成一份可重复使用的本地脚本部署配置，直接使用 `examples/deploy.weixin.env.example`：

```bash
cp ./examples/deploy.weixin.env.example ./.deploy.weixin.env
set -a && source ./.deploy.weixin.env && set +a
./scripts/deploy_bl.sh
```

这份模板默认按当前 `deploy_bl.sh` 的远端目录布局填写，并启用首发微信扫码登录。

### QQ 和微信并行部署

当前 `gateway` 进程一次只跑一个通道。要在同一台机器上同时跑 QQ 和微信，需要各自部署成独立 systemd 服务，但可以共用同一份二进制和远端代码目录。

并行部署时，至少把下面四项分开：

- `MINICLAW_REMOTE_SERVICE`
- `MINICLAW_REMOTE_ENV_FILE`
- `MINICLAW_REMOTE_APP_HOME`
- `MINICLAW_REMOTE_WORKSPACE`

仓库已经提供两份并行部署模板：

```bash
cp ./examples/deploy.qq.env.example ./.deploy.qq.env
cp ./examples/deploy.weixin.env.example ./.deploy.weixin.env

set -a && source ./.deploy.qq.env && set +a
./scripts/deploy_bl.sh

set -a && source ./.deploy.weixin.env && set +a
./scripts/deploy_bl.sh
```

默认推荐的并行部署布局：

```text
/bl/project/miniclaw/repo
/etc/miniclaw/miniclaw-qq.env
/etc/miniclaw/miniclaw-weixin.env
/var/lib/miniclaw/qq/
	workspace/
/var/lib/miniclaw/weixin/
	workspace/
```

这两次部署会分别生成 `miniclaw-qq` 和 `miniclaw-weixin` 两个服务。部署完成后可分别检查：

```bash
sudo systemctl status miniclaw-qq
sudo systemctl status miniclaw-weixin
```

常用覆盖变量：

- `MINICLAW_DEPLOY_HOST`
- `MINICLAW_REMOTE_USER`
- `MINICLAW_REMOTE_HOME`
- `MINICLAW_REMOTE_REPO`
- `MINICLAW_REMOTE_ENV_FILE`
- `MINICLAW_REMOTE_APP_HOME`
- `MINICLAW_REMOTE_WORKSPACE`
- `MINICLAW_REMOTE_CONFIG`
- `MINICLAW_REMOTE_MCP_CONFIG`
- `MINICLAW_REMOTE_SERVICE`
- `MINICLAW_REMOTE_WEBHOOK_PORT`

### deploy_bl.sh 默认目录布局

按 `deploy_bl.sh` 当前默认值，远端路径会是：

```text
/bl/project/miniclaw/repo
/etc/miniclaw/miniclaw.env
/root/.config/miniclaw/config
/root/.config/miniclaw/mcp.json
/root/.miniclaw/workspace
```

这些路径分别来自：

- `MINICLAW_REMOTE_REPO=/bl/project/miniclaw/repo`
- `MINICLAW_REMOTE_ENV_FILE=/etc/miniclaw/miniclaw.env`
- `MINICLAW_REMOTE_HOME=/root`
- `MINICLAW_REMOTE_MCP_CONFIG=/root/.config/miniclaw/mcp.json`

如果你希望脚本按下面的手工部署目录工作，至少覆盖这些变量：

```bash
MINICLAW_REMOTE_REPO=/opt/miniclaw \
MINICLAW_REMOTE_HOME=/var/lib/miniclaw \
MINICLAW_REMOTE_MCP_CONFIG=/etc/miniclaw/mcp.json \
MINICLAW_GATEWAY_CHANNEL=weixin \
./scripts/deploy_bl.sh
```

### 手工部署推荐目录布局

下面这套 `/opt + /var/lib + /etc` 布局是手工部署时更清晰的推荐方案，不是 `deploy_bl.sh` 的默认值。

```text
/opt/miniclaw/miniclaw
/etc/miniclaw/miniclaw.env
/etc/miniclaw/mcp.json
/var/lib/miniclaw/
	workspace/
```

### 1. 创建运行用户和目录

```bash
sudo useradd --system --create-home --home-dir /var/lib/miniclaw --shell /usr/sbin/nologin miniclaw
sudo mkdir -p /opt/miniclaw /etc/miniclaw /var/lib/miniclaw/workspace
sudo chown -R miniclaw:miniclaw /var/lib/miniclaw
```

### 2. 拷贝二进制与配置模板

```bash
sudo install -m 0755 ./miniclaw /opt/miniclaw/miniclaw
sudo install -m 0644 ./.env.example /etc/miniclaw/miniclaw.env
sudo install -m 0644 ./examples/mcp.json.example /etc/miniclaw/mcp.json
```

然后编辑 `/etc/miniclaw/miniclaw.env`，至少填这些字段：

- `MINICLAW_API_KEY`
- `MINICLAW_GATEWAY_CHANNEL=weixin`
- `MINICLAW_HOME=/var/lib/miniclaw`
- `MINICLAW_WORKSPACE=/var/lib/miniclaw/workspace`
- `MINICLAW_MCP_CONFIG_PATH=/etc/miniclaw/mcp.json`

如果你要并行部署 QQ 和微信，建议改成两份 env 文件，例如 `/etc/miniclaw/miniclaw-qq.env` 和 `/etc/miniclaw/miniclaw-weixin.env`，并分别给它们设置不同的 `MINICLAW_HOME` 和 `MINICLAW_WORKSPACE`。

### 3. 以服务用户初始化工作区

```bash
sudo -u miniclaw bash -lc 'set -a; source /etc/miniclaw/miniclaw.env; set +a; /opt/miniclaw/miniclaw onboard && /opt/miniclaw/miniclaw status'
```

### 4. 配置微信账号

MiniClaw 的微信通道支持两种方式：

- 直接在环境变量里写 `MINICLAW_WEIXIN_TOKEN`
- 用二维码登录，把 token 持久化到 workspace

推荐二维码登录一次，再让服务长期复用已保存账号：

```bash
sudo -u miniclaw bash -lc 'set -a; source /etc/miniclaw/miniclaw.env; set +a; /opt/miniclaw/miniclaw gateway login --channel weixin'
sudo -u miniclaw bash -lc 'set -a; source /etc/miniclaw/miniclaw.env; set +a; /opt/miniclaw/miniclaw gateway accounts --channel weixin'
```

登录后的账号信息会保存在：

- `MINICLAW_WORKSPACE/state/weixin/accounts/*.json`
- `MINICLAW_WORKSPACE/state/weixin/accounts.json`
- `MINICLAW_WORKSPACE/state/weixin/active_account.txt`

如果有多个微信账号，可在环境变量里设置 `MINICLAW_WEIXIN_ACCOUNT_ID` 指定激活账号。

### 5. 启动前做一次连通性检查

```bash
sudo -u miniclaw bash -lc 'set -a; source /etc/miniclaw/miniclaw.env; set +a; /opt/miniclaw/miniclaw gateway --channel weixin --once'
```

`--once` 只做 bootstrap，不进入常驻长轮询，适合上线前 smoke test。

### 6. 安装 systemd 服务

```bash
sudo cp ./examples/miniclaw.weixin.service.example /etc/systemd/system/miniclaw-weixin.service
sudo systemctl daemon-reload
sudo systemctl enable --now miniclaw-weixin
sudo systemctl status miniclaw-weixin
sudo journalctl -u miniclaw-weixin -f
```

如果你修改了 `/etc/miniclaw/miniclaw.env`，执行：

```bash
sudo systemctl restart miniclaw-weixin
```

## 微信通道说明

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
