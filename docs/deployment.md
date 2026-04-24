# 部署指南

以下流程覆盖当前仓库支持的几种部署方式：

- SSH + systemd 脚本部署
- QQ 和微信并行部署
- 手工 systemd 部署
- Podman Alpine 容器部署

默认示例以 Linux 环境为主，并以微信通道作为入门场景。

## 使用部署脚本

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

## QQ 和微信并行部署

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

## deploy_bl.sh 默认目录布局

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

## 手工部署推荐目录布局

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

## Podman Alpine 部署命令

如果你更偏向容器部署，仓库现在提供 `Containerfile.alpine` 和 `scripts/deploy_podman_alpine.sh`。

这个脚本会自动完成以下步骤：

- 构建 Linux 二进制
- 构建 Alpine 基础镜像
- 在本地生成或复用 `MINICLAW_PODMAN_ENV_FILE` 指向的 env 文件
- 以同一组挂载执行一次 `miniclaw onboard`
- 对微信按需触发二维码登录
- 启动长期运行的 Podman 容器
- 对 `qq` 执行 callback 校验，对 `weixin` 执行 bootstrap 校验

QQ 示例：

```bash
cp ./examples/deploy.podman.alpine.qq.env.example ./.deploy.podman.qq.env
set -a && source ./.deploy.podman.qq.env && set +a
./scripts/deploy_podman_alpine.sh
```

微信示例：

```bash
cp ./examples/deploy.podman.alpine.weixin.env.example ./.deploy.podman.weixin.env
set -a && source ./.deploy.podman.weixin.env && set +a
./scripts/deploy_podman_alpine.sh
```

脚本默认把容器持久化目录放在仓库下的 `.podman/<container>/`，并把应用 env 文件生成到 `MINICLAW_PODMAN_ENV_FILE`。首次运行后，记得编辑这份 env 文件，填入 `MINICLAW_API_KEY`、QQ 或微信通道所需的凭证。

Podman 并行部署时，至少把下面几项分开：

- `MINICLAW_PODMAN_CONTAINER`
- `MINICLAW_PODMAN_STATE_ROOT`
- `MINICLAW_PODMAN_ENV_FILE`
- `MINICLAW_PODMAN_HOME`
- `MINICLAW_PODMAN_QQ_PORT`（仅 QQ 需要对外暴露端口时）

如果你希望挂载自定义 MCP 配置，而不是镜像内置的 `examples/mcp.mmx.json.example`，可以设置：

```bash
MINICLAW_PODMAN_MCP_CONFIG=/absolute/path/to/mcp.json
```
