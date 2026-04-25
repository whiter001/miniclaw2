# 本地运行微信版 MiniClaw

这份文档只覆盖本机直接运行微信通道，不涉及远端 `bl` 服务器部署。

当前仓库里：

- `./scripts/deploy_bl.sh` 默认会部署到远端 `bl`
- `examples/deploy.podman.alpine.weixin.env.example` 也是远端模板

如果你的目标是在 macOS 或 Linux 本机把微信版 MiniClaw 跑起来，推荐直接运行本地二进制。

## 适用场景

适合下面这类需求：

- 本机调试微信通道
- 本机做二维码登录并保存账号
- 本机验证 Agent、MCP、workspace 和记忆流程

不适合下面这类需求：

- 部署到远端服务器长期运行
- 直接复用当前远端 Podman 模板

这两类场景请看 [docs/deployment.md](docs/deployment.md)。

## 前置条件

本地运行至少需要：

- 可用的 Go 环境
- 可访问 MiniMax API
- 可访问微信通道后端接口
- 一份可用的 `MINICLAW_API_KEY`

如果只是本地直跑，不需要先准备 systemd、Podman machine 或远端 SSH。

## 默认本地路径

程序默认会把本地状态写到用户目录，不是 `/var/lib` 或 `/etc`：

- 配置文件：`~/.config/miniclaw/config`
- MCP 配置：`~/.config/miniclaw/mcp.json`
- home：`~/.miniclaw`
- workspace：`~/.miniclaw/workspace`

第一次执行 `./miniclaw onboard` 时，如果配置文件不存在，程序会自动创建默认配置。

## 快速开始

### 1. 构建二进制

```bash
./build.sh
```

### 2. 初始化本地目录

```bash
./miniclaw onboard
./miniclaw status
```

你应该能在 `status` 输出里看到：

- `config: ~/.config/miniclaw/config`
- `workspace: ~/.miniclaw/workspace`
- `gateway channel:` 当前默认值

### 3. 编辑本地配置

打开 `~/.config/miniclaw/config`，至少填写下面几项：

```ini
api_key=你的 MiniMax Key
gateway_channel=weixin
```

如果你已经有微信 token，也可以直接填写：

```ini
weixin_token=你的 token
```

更完整的参考模板见 [examples/miniclaw.config.example](examples/miniclaw.config.example)。

一个适合本地微信通道的最小配置示例：

```ini
home_dir=~/.miniclaw
workspace=~/.miniclaw/workspace
mcp_config_path=~/.config/miniclaw/mcp.json

api_key=你的 MiniMax Key
base_url=https://api.minimaxi.com/anthropic
model=MiniMax-M2.7
gateway_channel=weixin

weixin_api_base=https://ilinkai.weixin.qq.com
weixin_cdn_base=https://novac2c.cdn.weixin.qq.com/c2c
weixin_allow_users=
weixin_processing_text=收到，处理中，请稍候。
```

### 4. 再看一次状态

```bash
./miniclaw status
```

这时至少应满足：

- `api configured: true`
- `gateway channel: weixin`

### 5. 首次登录微信账号

推荐先走二维码登录，再让本地服务长期复用已保存账号：

```bash
./miniclaw gateway login --channel weixin
./miniclaw gateway accounts --channel weixin
```

默认情况下，`gateway login` 会在本机尝试自动打开微信二维码页面。

如果你不想自动拉起浏览器，可以执行：

```bash
./miniclaw gateway login --channel weixin --no-open
```

登录后的账号信息会保存在：

- `~/.miniclaw/workspace/state/weixin/accounts/*.json`
- `~/.miniclaw/workspace/state/weixin/accounts.json`
- `~/.miniclaw/workspace/state/weixin/active_account.txt`

如果你有多个微信账号，可切换当前活动账号：

```bash
./miniclaw gateway accounts --channel weixin --use bot-1
```

或者在配置文件里设置：

```ini
weixin_account_id=bot-1
```

### 6. 先做一次 smoke test

```bash
./miniclaw gateway --channel weixin --once
```

`--once` 只做 bootstrap，不会进入常驻长轮询。第一次本地启动前，先跑这个命令更容易判断问题是在配置、登录还是运行阶段。

### 7. 正式启动

```bash
./miniclaw gateway --channel weixin
```

这时进程会进入常驻长轮询模式，开始接收和回复微信消息。

## 环境变量方式

如果你不想直接编辑 `~/.config/miniclaw/config`，也可以用环境变量在当前 shell 里运行：

```bash
export MINICLAW_HOME="$HOME/.miniclaw"
export MINICLAW_WORKSPACE="$HOME/.miniclaw/workspace"
export MINICLAW_MCP_CONFIG_PATH="$HOME/.config/miniclaw/mcp.json"
export MINICLAW_API_KEY="你的 MiniMax Key"
export MINICLAW_GATEWAY_CHANNEL=weixin

./miniclaw onboard
./miniclaw gateway login --channel weixin
./miniclaw gateway --channel weixin
```

不建议直接把仓库根目录下的 [.env.example](.env.example) 原样 `source` 到本地 shell，因为它默认写的是服务部署路径：

- `MINICLAW_HOME=/var/lib/miniclaw`
- `MINICLAW_WORKSPACE=/var/lib/miniclaw/workspace`
- `MINICLAW_MCP_CONFIG_PATH=/etc/miniclaw/mcp.json`

这更适合服务部署，不是本地桌面调试的默认路径。

## 常用命令

```bash
./miniclaw onboard
./miniclaw status
./miniclaw gateway login --channel weixin
./miniclaw gateway accounts --channel weixin
./miniclaw gateway accounts --channel weixin --use bot-1
./miniclaw gateway logout --channel weixin --account bot-1
./miniclaw gateway --channel weixin --once
./miniclaw gateway --channel weixin
```

## 常见问题

### `api configured: false`

说明 `api_key` 没有生效。优先检查：

- `~/.config/miniclaw/config` 里是否填了 `api_key`
- 当前 shell 是否覆盖了 `MINICLAW_API_KEY`

### `weixin configured: false`

说明当前既没有 `weixin_token`，也没有已保存的微信账号。执行：

```bash
./miniclaw gateway login --channel weixin
./miniclaw gateway accounts --channel weixin
```

### 提示先执行 `miniclaw onboard`

说明 workspace 目录还没初始化，先执行：

```bash
./miniclaw onboard
```

### 多账号切换后没生效

先确认当前活动账号：

```bash
./miniclaw gateway accounts --channel weixin
```

如果需要显式指定，执行：

```bash
./miniclaw gateway accounts --channel weixin --use bot-1
```

或者把 `weixin_account_id=bot-1` 写进配置文件。

## 与远端部署的区别

本地微信直跑和远端部署最大的区别是入口不同：

- 本地直跑：直接执行 `./miniclaw gateway --channel weixin`
- 远端部署：使用 `./scripts/deploy_bl.sh` 或 [docs/deployment.md](docs/deployment.md) 里的 systemd / Podman 流程

如果你只是想先在本机把微信通道跑通，不要从远端 Podman 模板开始。
