# MiniClaw Frontend

`frontend/` 是一个用 Bun 初始化的 React 子项目，负责提供 MiniClaw 的本地 Web 控制台。

当前能力：

- Ant Design 界面
- React Router 路由（/agent、/dashboard、/commands）
- Bun full-stack server
- 通过 HTTP API 执行本地 `./miniclaw status`
- 通过 HTTP API 执行本地 `./miniclaw agent -p "..."`

## 开发

在仓库根目录执行：

```bash
cd frontend
bun install
bun run dev
```

启动后默认监听 `http://127.0.0.1:5020`，并在终端输出一个带 `?token=...` 的授权地址。首次打开这个地址后，服务会设置一个 `HttpOnly; SameSite=Strict` 的本地会话 cookie，后续同一浏览器会话可以直接访问控制台。

如果需要覆盖端口，可设置：

```bash
MINICLAW_FRONTEND_PORT=5080 bun run dev
```

如果确实需要让其他机器访问前端服务，可以显式设置监听地址和固定访问 token：

```bash
MINICLAW_FRONTEND_HOST=0.0.0.0 MINICLAW_FRONTEND_TOKEN='change-me' bun run dev
```

前端 API 可通过 `X-MiniClaw-Frontend-Token` 请求头显式授权；浏览器 cookie 授权的写入类请求还会校验同源 `Origin` / `Referer`，用于降低跨站请求伪造风险。不要在不可信网络中暴露这个控制台。

## 构建

```bash
cd frontend
bun run build
```

## 运行方式

Bun server 会优先调用仓库根目录下的 `./miniclaw` 二进制；如果二进制不存在，则回退到：

```bash
go run cmd/miniclaw/main.go
```

因此开发时建议先在仓库根目录构建一次：

```bash
./build.sh
```

## 主要接口

- `GET /api/health`
- `GET /api/status`
- `POST /api/agent/run`
- `POST /api/cli/run`
