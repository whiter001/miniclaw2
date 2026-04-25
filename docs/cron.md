# Cron 串行任务设计与使用

这份文档覆盖 MiniClaw 当前的 cron 任务设计，以及已经落地的串行执行实现。

## 目标

当前仓库里的 Agent、memory、skills 和工具执行都默认共享同一个 workspace。

这带来两个直接约束：

- 同一 workspace 下的定时任务不能并发跑
- 定时任务需要可恢复的状态文件，而不是只靠内存队列

因此，当前实现把 cron 设计成：

- `workspace/cron/*.json` 作为任务定义
- 同一 workspace 下定时任务串行执行
- 用 `workspace/state/cron/runner.lock` 做跨进程互斥
- 用 `workspace/state/cron/tasks/*.json` 记录每个任务状态
- 用 `workspace/HEARTBEAT.md` 生成人类可读的任务概览

## 任务文件格式

任务文件放在：

- `workspace/cron/*.json`

示例：

```json
{
  "id": "daily-summary",
  "description": "Summarize recent daily notes",
  "enabled": true,
  "schedule": "0 9 * * *",
  "prompt": "总结最近一天的重要事项，并更新 memory summary。",
  "timeout_seconds": 600,
  "skip_if_running": true,
  "max_tool_iterations": 40,
  "enable_mcp": false
}
```

字段说明：

- `id`: 任务 ID。缺省时会退化为文件名。
- `description`: 可选说明。
- `enabled`: 是否启用。默认 `true`。
- `schedule`: 调度表达式。
- `prompt`: 要执行的 Agent prompt。
- `timeout_seconds`: 单次任务超时。默认 600 秒。
- `skip_if_running`: 当另一个 cron 任务已占用 workspace 锁时是否跳过。默认 `true`。
- `max_tool_iterations`: 可选，覆盖默认工具迭代上限。
- `enable_mcp`: 可选，覆盖默认 MCP 开关。

## 支持的 schedule

当前实现支持两类：

- 五段 cron：`minute hour day month weekday`
- 特殊形式：`@hourly`、`@daily`、`@weekly`、`@monthly`、`@every 10m`

五段 cron 目前只支持数字、范围、步长和逗号组合，例如：

- `*/5 * * * *`
- `0 9 * * 1-5`
- `0 0 1 * *`

首版没有引入完整的扩展语法，例如月份名称、星期名称或 `?`、`L`、`W`。

## 串行执行语义

### 同一 workspace 下默认串行

`miniclaw cron run` 和 `miniclaw cron serve` 都按任务 ID 顺序逐个执行任务。

这解决的是“定时任务之间互相干扰”的主问题：

- 不会同时写 memory 文件
- 不会同时在同一 workspace 里跑 `exec`
- 不会同时更新 skills / session / summary

### 跨进程互斥

除了单进程内串行，当前实现还加了 workspace 级锁：

- `workspace/state/cron/runner.lock`

这样即使你同时启动两个 `miniclaw cron serve` 进程，也不会让两个任务同时在同一 workspace 里执行。

### `skip_if_running`

如果 workspace 锁已被占用：

- `skip_if_running=true`：当前任务直接跳过
- `skip_if_running=false`：当前任务等待锁释放后再执行

### 不做积压回放

当前实现不会在一次长暂停后把所有错过的时间点全部补跑。

行为是：

- 如果某个任务已经过期，会执行一次
- 执行结束后把 `next_run_at` 推进到未来的下一个时间点

这样可以避免服务恢复后连续补跑大量历史任务，影响正常流量。

## 状态文件

每个任务的状态都会写到：

- `workspace/state/cron/tasks/<task-id>.json`

状态里会记录：

- `running`
- `last_scheduled_at`
- `last_started_at`
- `last_finished_at`
- `last_duration_seconds`
- `last_status`
- `last_error`
- `last_session_file`
- `next_run_at`

概览会写到：

- `workspace/HEARTBEAT.md`

## CLI 命令

列出任务：

```bash
./miniclaw cron list
```

执行一次当前到期任务：

```bash
./miniclaw cron run
```

手工触发某个任务：

```bash
./miniclaw cron trigger --id daily-summary
```

常驻轮询调度：

```bash
./miniclaw cron serve
./miniclaw cron serve --poll 30s
```

## 与其他执行面的关系

当前实现保证的是：

- 同一 workspace 下，cron 任务之间串行

当前实现还没有把下面这些入口统一接入同一把锁：

- 手工 `miniclaw agent`
- QQ / 微信消息驱动的 Agent 执行

如果你想把 cron 与在线消息处理完全隔离，建议使用独立 workspace 来跑 cron。

## 推荐落地方式

如果你的目标是“定时整理 summary / memory / 周报”，推荐：

1. 为 cron 单独准备一个 workspace。
2. 在这个 workspace 下维护 `cron/*.json`。
3. 用 `miniclaw cron serve` 常驻运行。
4. 让需要的任务通过 prompt 读写自己的 memory / summary。

这样可以把定时任务的副作用面限制在独立 workspace 内，避免和在线消息流共享状态。
