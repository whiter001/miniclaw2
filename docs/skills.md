# Skills 说明

MiniClaw 现在支持三类 skill 能力：

- 手工维护的 workspace skills
- 运行过程中自动沉淀的 autoskill
- 通过 QQ / 微信聊天直接触发的显式 skill 管理命令

## 目录结构

`miniclaw onboard` 会初始化 `workspace/skills/` 目录。

每个 skill 使用一个独立子目录：

```text
workspace/
    skills/
        testing/
            SKILL.md
            skill.json
        _candidates/
            autoskill-some-workflow/
                SKILL.md
                skill.json
```

约定：

- `SKILL.md` 是给 agent 看的技能说明正文
- `skill.json` 是结构化元数据，保存评分、关键词、推荐工具、示例、命中统计等
- `_candidates/` 保存低置信度 autoskill 候选，默认不会自动注入 agent prompt

## 运行时加载

每轮 agent 执行前，MiniClaw 会扫描 `workspace/skills/` 下的 `SKILL.md`，但会跳过 `_candidates/` 和 `_archived/` 这类内部目录。

当前选择逻辑：

- 根据用户当前 query 做字段加权匹配，优先考虑 skill 名称、描述、关键词、推荐工具和 autoskill 的结构化指导字段
- 结合 skill 历史分数和成功率做排序
- 只把前几个相关 skill 注入本轮 system prompt，而不是全量加载
- autoskill 注入时优先保留 `When To Use`、`Decision Hints`、`Procedure`、`Watchouts`、`Final Outcome` 等核心 section，跳过最近捕获历史和指标明细

相关配置：

- `skill_selection_limit` 或环境变量 `MINICLAW_SKILL_SELECTION_LIMIT`

## Autoskill

如果一轮任务成功完成，并且成功工具调用数量达到阈值，MiniClaw 会从 session 日志里自动提炼经验，创建或更新 autoskill。

autoskill 分为两层：

- `approved`：写入 `workspace/skills/<name>/`，会参与后续自动检索和注入
- `candidate`：写入 `workspace/skills/_candidates/<name>/`，保留供检查，但不会自动加载

恢复型运行、带少量失败但最终完成的运行通常先进入 `candidate`；后续同族任务如果出现更干净的成功轨迹，会自动提升到 `approved`。

autoskill 会自动记录：

- tier 和质量分
- 质量原因与警告
- 决策提示、执行步骤、注意事项和最终结果摘要
- 关键词
- 推荐工具
- 最近成功示例的脱敏摘要
- 被选中次数
- 成功 / 失败次数
- 当前分数

为避免把一次性上下文长期留在元数据中，`skill.json` 不保存原始 prompt / response；示例会先替换绝对路径、时间戳、运行 ID 等环境特定值，再写入 `request_summary` / `outcome_summary`。

autoskill 文档会随着后续成功执行持续优化。生成内容会分成 `Decision Hints`、`Procedure`、`Watchouts`、`Final Outcome` 等 section，帮助下一轮判断什么时候该用、怎么执行、需要验证什么，以及哪些历史环境细节不能复用。

相关配置：

- `enable_auto_skills` 或环境变量 `MINICLAW_ENABLE_AUTO_SKILLS`
- `auto_skill_min_tool_calls` 或环境变量 `MINICLAW_AUTO_SKILL_MIN_TOOL_CALLS`
- `auto_skill_max_examples` 或环境变量 `MINICLAW_AUTO_SKILL_MAX_EXAMPLES`

## Skill 评分

每次 skill 被命中后，MiniClaw 会根据该轮结果回写评分元数据。

当前评分会综合考虑：

- capture 次数
- 选中次数
- 成功次数
- 失败次数
- 关键词覆盖
- 推荐工具稳定性

相关配置：

- `enable_skill_scoring` 或环境变量 `MINICLAW_ENABLE_SKILL_SCORING`

## 聊天命令管理

QQ 和微信消息都会走统一的 gateway runtime，因此都支持显式的 `/skill` 命令。

支持的命令：

```text
/skill list
/skill show <name>
/skill add <name>
<skill content>

/skill update <name>
<replacement content>

/skill optimize <name>
/skill delete <name>
```

行为说明：

- `list`：列出当前 skills，包含分数和是否为 autoskill
- `show`：查看 skill 正文和元数据摘要
- `add`：创建手工 skill；如果正文不是 Markdown 标题开头，会自动补一个标题
- `update`：整体替换 skill 正文，并刷新元数据
- `optimize`：重新提取关键词、工具和分数；如果目标是 autoskill，还会重写 autoskill 文档
- `delete`：删除对应 skill 目录

示例：

```text
/skill add testing
Use read_file before editing and validate with go test.
```

```text
/skill update testing
# Testing

Use read_file before editing.
Use write_file only after confirming the target file.
Finish with go test for the touched package.
```

```text
/skill optimize testing
```

## 手工 skill 与 autoskill 的关系

两者都会参与检索和排序，但来源不同：

- 手工 skill 适合沉淀稳定流程、团队约定和高质量最佳实践
- autoskill 适合把近期成功经验快速落盘，再逐步演化

如果一个手工 skill 已经足够成熟，建议优先维护手工 skill；autoskill 更适合作为经验捕获层。

## 建议

- 手工 skill 尽量保持短小、明确、可执行
- 每个 skill 聚焦一个任务模式，不要把太多流程混进同一个 skill
- 如果某个 autoskill 已经稳定，可手工整理成正式 skill
- 对高风险场景，优先使用显式 `/skill` 命令，而不是依赖模型自行决定是否改 skill 文件
