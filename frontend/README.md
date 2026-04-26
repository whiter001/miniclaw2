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

启动后默认监听 `http://localhost:5020`。

如果需要覆盖端口，可设置：

```bash
MINICLAW_FRONTEND_PORT=5080 bun run dev
```

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
