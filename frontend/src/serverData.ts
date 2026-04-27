import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { basename, dirname, extname, join, resolve } from "node:path";

import type { DashboardSummary, RunDetail, RunMessage, RunRecord, SkillDetail, SkillRecord, SkillWritePayload, TaskRecord, TaskWritePayload } from "./types";

type BinaryMode = "binary" | "go-run";

interface RuntimeConfig {
  homeDir: string;
  workspaceDir: string;
  configPath: string;
  mcpConfigPath: string;
  apiKey: string;
  baseUrl: string;
  model: string;
  enableMcp: boolean;
  gatewayChannel: string;
  qqWebhookHost: string;
  qqWebhookPort: number;
  qqWebhookPath: string;
  qqAllowUsers: string;
  qqAllowGroups: string;
  weixinAllowUsers: string;
  weixinToken: string;
  enableAutoSkills: boolean;
  enableSkillScoring: boolean;
  maxToolIterations: number;
}

interface TaskFilePayload {
  id?: string;
  description?: string;
  schedule?: string;
  prompt?: string;
  enabled?: boolean;
  skip_if_running?: boolean;
  timeout_seconds?: number;
  max_tool_iterations?: number;
  enable_mcp?: boolean;
}

interface TaskStatePayload {
  task_id?: string;
  running?: boolean;
  last_scheduled_at?: string;
  last_started_at?: string;
  last_finished_at?: string;
  last_duration_seconds?: number;
  last_status?: string;
  last_error?: string;
  last_session_file?: string;
  next_run_at?: string;
}

interface TaskStateInfo {
  taskId: string;
  lastStatus: string;
  lastError: string;
  lastStartedAt: string | null;
  lastFinishedAt: string | null;
  sessionFile: string;
}

const defaultTaskTimeoutSeconds = 10 * 60;

function defaultConfig(): RuntimeConfig {
  const userHome = homedir();
  const homeDir = join(userHome, ".miniclaw");
  return {
    homeDir,
    workspaceDir: join(homeDir, "workspace"),
    configPath: join(userHome, ".config", "miniclaw", "config"),
    mcpConfigPath: join(userHome, ".config", "miniclaw", "mcp.json"),
    apiKey: "",
    baseUrl: "https://api.minimaxi.com/anthropic",
    model: "MiniMax-M2.7",
    enableMcp: false,
    gatewayChannel: "qq",
    qqWebhookHost: "127.0.0.1",
    qqWebhookPort: 8080,
    qqWebhookPath: "/webhook/qq",
    qqAllowUsers: "",
    qqAllowGroups: "",
    weixinAllowUsers: "",
    weixinToken: "",
    enableAutoSkills: true,
    enableSkillScoring: true,
    maxToolIterations: 100,
  };
}

function expandHomePath(value: string) {
  if (!value) {
    return value;
  }
  if (value === "~") {
    return homedir();
  }
  if (value.startsWith("~/")) {
    return join(homedir(), value.slice(2));
  }
  return value;
}

function applyConfigValue(config: RuntimeConfig, key: string, value: string) {
  switch (key) {
    case "home_dir":
      config.homeDir = expandHomePath(value);
      return;
    case "workspace":
      config.workspaceDir = expandHomePath(value);
      return;
    case "mcp_config_path":
      config.mcpConfigPath = expandHomePath(value);
      return;
    case "api_key":
      config.apiKey = value;
      return;
    case "base_url":
    case "api_url":
      config.baseUrl = value;
      return;
    case "model":
      config.model = value;
      return;
    case "enable_mcp":
      config.enableMcp = value === "true";
      return;
    case "gateway_channel":
      config.gatewayChannel = value || "qq";
      return;
    case "qq_webhook_host":
      config.qqWebhookHost = value || config.qqWebhookHost;
      return;
    case "qq_webhook_port": {
      const parsed = Number.parseInt(value, 10);
      if (Number.isFinite(parsed) && parsed > 0) {
        config.qqWebhookPort = parsed;
      }
      return;
    }
    case "qq_webhook_path":
      config.qqWebhookPath = value || config.qqWebhookPath;
      return;
    case "qq_allow_users":
      config.qqAllowUsers = value;
      return;
    case "qq_allow_groups":
      config.qqAllowGroups = value;
      return;
    case "weixin_allow_users":
      config.weixinAllowUsers = value;
      return;
    case "weixin_token":
      config.weixinToken = value;
      return;
    case "enable_auto_skills":
      config.enableAutoSkills = value !== "false";
      return;
    case "enable_skill_scoring":
      config.enableSkillScoring = value !== "false";
      return;
    case "max_tool_iterations": {
      const parsed = Number.parseInt(value, 10);
      if (Number.isFinite(parsed) && parsed > 0) {
        config.maxToolIterations = parsed;
      }
      return;
    }
  }
}

function parseConfigFile(configPath: string, config: RuntimeConfig) {
  if (!existsSync(configPath)) {
    return config;
  }
  const content = readFileSync(configPath, "utf8");
  for (const line of content.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) {
      continue;
    }
    const separator = trimmed.indexOf("=");
    if (separator <= 0) {
      continue;
    }
    const key = trimmed.slice(0, separator).trim();
    const value = trimmed.slice(separator + 1).trim();
    applyConfigValue(config, key, value);
  }
  return config;
}

function applyEnvOverrides(config: RuntimeConfig) {
  if (process.env.MINICLAW_HOME?.trim()) {
    config.homeDir = expandHomePath(process.env.MINICLAW_HOME.trim());
  }
  if (process.env.MINICLAW_WORKSPACE?.trim()) {
    config.workspaceDir = expandHomePath(process.env.MINICLAW_WORKSPACE.trim());
  }
  if (process.env.MINICLAW_MCP_CONFIG_PATH?.trim()) {
    config.mcpConfigPath = expandHomePath(process.env.MINICLAW_MCP_CONFIG_PATH.trim());
  }
  if (process.env.MINICLAW_API_KEY !== undefined) {
    config.apiKey = process.env.MINICLAW_API_KEY;
  }
  if (process.env.ANTHROPIC_BASE_URL?.trim()) {
    config.baseUrl = process.env.ANTHROPIC_BASE_URL.trim();
  }
  if (process.env.MINICLAW_API_URL?.trim()) {
    config.baseUrl = process.env.MINICLAW_API_URL.trim();
  }
  if (process.env.MINICLAW_MODEL?.trim()) {
    config.model = process.env.MINICLAW_MODEL.trim();
  }
  if (process.env.MINICLAW_ENABLE_MCP?.trim()) {
    config.enableMcp = process.env.MINICLAW_ENABLE_MCP.trim() === "true";
  }
  if (process.env.MINICLAW_GATEWAY_CHANNEL?.trim()) {
    config.gatewayChannel = process.env.MINICLAW_GATEWAY_CHANNEL.trim();
  }
  if (process.env.MINICLAW_QQ_WEBHOOK_HOST?.trim()) {
    config.qqWebhookHost = process.env.MINICLAW_QQ_WEBHOOK_HOST.trim();
  }
  if (process.env.MINICLAW_QQ_WEBHOOK_PORT?.trim()) {
    const parsed = Number.parseInt(process.env.MINICLAW_QQ_WEBHOOK_PORT.trim(), 10);
    if (Number.isFinite(parsed) && parsed > 0) {
      config.qqWebhookPort = parsed;
    }
  }
  if (process.env.MINICLAW_QQ_WEBHOOK_PATH?.trim()) {
    config.qqWebhookPath = process.env.MINICLAW_QQ_WEBHOOK_PATH.trim();
  }
  if (process.env.MINICLAW_QQ_ALLOW_USERS !== undefined) {
    config.qqAllowUsers = process.env.MINICLAW_QQ_ALLOW_USERS;
  }
  if (process.env.MINICLAW_QQ_ALLOW_GROUPS !== undefined) {
    config.qqAllowGroups = process.env.MINICLAW_QQ_ALLOW_GROUPS;
  }
  if (process.env.MINICLAW_WEIXIN_ALLOW_USERS !== undefined) {
    config.weixinAllowUsers = process.env.MINICLAW_WEIXIN_ALLOW_USERS;
  }
  if (process.env.MINICLAW_WEIXIN_TOKEN !== undefined) {
    config.weixinToken = process.env.MINICLAW_WEIXIN_TOKEN;
  }
  if (process.env.MINICLAW_ENABLE_AUTO_SKILLS?.trim()) {
    config.enableAutoSkills = process.env.MINICLAW_ENABLE_AUTO_SKILLS.trim() !== "false";
  }
  if (process.env.MINICLAW_ENABLE_SKILL_SCORING?.trim()) {
    config.enableSkillScoring = process.env.MINICLAW_ENABLE_SKILL_SCORING.trim() !== "false";
  }
  if (process.env.MINICLAW_MAX_TOOL_ITERATIONS?.trim()) {
    const parsed = Number.parseInt(process.env.MINICLAW_MAX_TOOL_ITERATIONS.trim(), 10);
    if (Number.isFinite(parsed) && parsed > 0) {
      config.maxToolIterations = parsed;
    }
  }
  return config;
}

export function loadRuntimeConfig() {
  const config = applyEnvOverrides(parseConfigFile(defaultConfig().configPath, defaultConfig()));
  config.homeDir = expandHomePath(config.homeDir);
  config.workspaceDir = expandHomePath(config.workspaceDir);
  config.configPath = expandHomePath(config.configPath);
  config.mcpConfigPath = expandHomePath(config.mcpConfigPath);
  return config;
}

export function getBinaryMode(miniclawBinaryPath: string): BinaryMode {
  return existsSync(miniclawBinaryPath) ? "binary" : "go-run";
}

function safeReadJson<T>(filePath: string): T | null {
  if (!existsSync(filePath)) {
    return null;
  }
  return JSON.parse(readFileSync(filePath, "utf8")) as T;
}

function cronDir(workspaceDir: string) {
  return join(workspaceDir, "cron");
}

function stateDir(workspaceDir: string) {
  return join(workspaceDir, "state", "cron", "tasks");
}

function sessionsDir(workspaceDir: string) {
  return join(workspaceDir, "sessions");
}

function skillsDir(workspaceDir: string) {
  return join(workspaceDir, "skills");
}

function normalizeDate(value?: string) {
  if (!value) {
    return null;
  }
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? null : parsed.toISOString();
}

function taskStatePath(workspaceDir: string, taskId: string) {
  return join(stateDir(workspaceDir), `${taskId}.json`);
}

function readTaskState(workspaceDir: string, taskId: string) {
  return safeReadJson<TaskStatePayload>(taskStatePath(workspaceDir, taskId));
}

function normalizeTaskRecord(filePath: string, state: TaskStatePayload | null): TaskRecord {
  const payload = safeReadJson<TaskFilePayload>(filePath) ?? {};
  const resolvedId = (payload.id ?? "").trim() || basename(filePath, extname(filePath));
  const nextRunAt = normalizeDate(state?.next_run_at);
  const running = Boolean(state?.running);
  return {
    id: resolvedId,
    description: (payload.description ?? "").trim(),
    schedule: (payload.schedule ?? "").trim(),
    prompt: (payload.prompt ?? "").trim(),
    enabled: payload.enabled ?? true,
    skipIfRunning: payload.skip_if_running ?? true,
    timeoutSeconds: payload.timeout_seconds && payload.timeout_seconds > 0 ? payload.timeout_seconds : defaultTaskTimeoutSeconds,
    maxToolIterations: payload.max_tool_iterations ?? 0,
    enableMcp: payload.enable_mcp ?? null,
    filePath,
    due: Boolean(payload.enabled ?? true) && !running && Boolean(nextRunAt && new Date(nextRunAt).getTime() <= Date.now()),
    running,
    nextRunAt,
    lastScheduledAt: normalizeDate(state?.last_scheduled_at),
    lastStartedAt: normalizeDate(state?.last_started_at),
    lastFinishedAt: normalizeDate(state?.last_finished_at),
    lastDurationSeconds: state?.last_duration_seconds ?? 0,
    lastStatus: (state?.last_status ?? "").trim(),
    lastError: (state?.last_error ?? "").trim(),
    lastSessionFile: (state?.last_session_file ?? "").trim(),
  };
}

export function listTasks(): TaskRecord[] {
  const { workspaceDir } = loadRuntimeConfig();
  const dir = cronDir(workspaceDir);
  if (!existsSync(dir)) {
    return [];
  }
  return readdirSync(dir)
    .filter(name => name.toLowerCase().endsWith(".json"))
    .map(name => {
      const filePath = join(dir, name);
      const payload = safeReadJson<TaskFilePayload>(filePath) ?? {};
      const taskId = (payload.id ?? "").trim() || basename(name, ".json");
      return normalizeTaskRecord(filePath, readTaskState(workspaceDir, taskId));
    })
    .sort((left, right) => left.id.localeCompare(right.id, "zh-CN"));
}

function validateTaskPayload(payload: TaskWritePayload, existingId?: string) {
  const id = payload.id.trim();
  if (!id) {
    throw new Error("任务 ID 不能为空。");
  }
  if (!/^[a-zA-Z0-9._-]+$/.test(id)) {
    throw new Error("任务 ID 只能包含字母、数字、点、下划线和中划线。");
  }
  if (!payload.schedule.trim()) {
    throw new Error("Cron 表达式不能为空。");
  }
  if (!payload.prompt.trim()) {
    throw new Error("任务提示词不能为空。");
  }
  if (payload.timeoutSeconds !== undefined && payload.timeoutSeconds <= 0) {
    throw new Error("超时时间必须大于 0。");
  }
  if (payload.maxToolIterations !== undefined && payload.maxToolIterations < 0) {
    throw new Error("最大工具迭代次数不能小于 0。");
  }
  if (existingId && existingId !== id && existsSync(join(cronDir(loadRuntimeConfig().workspaceDir), `${id}.json`))) {
    throw new Error("同名任务已存在。");
  }
}

export function saveTask(payload: TaskWritePayload, existingId?: string) {
  validateTaskPayload(payload, existingId);
  const { workspaceDir } = loadRuntimeConfig();
  mkdirSync(cronDir(workspaceDir), { recursive: true });
  const nextPayload: TaskFilePayload = {
    id: payload.id.trim(),
    description: payload.description?.trim() || undefined,
    schedule: payload.schedule.trim(),
    prompt: payload.prompt.trim(),
    enabled: payload.enabled ?? true,
    skip_if_running: payload.skipIfRunning ?? true,
    timeout_seconds: payload.timeoutSeconds && payload.timeoutSeconds > 0 ? payload.timeoutSeconds : defaultTaskTimeoutSeconds,
    max_tool_iterations: payload.maxToolIterations ?? 0,
  };
  if (payload.enableMcp !== null && payload.enableMcp !== undefined) {
    nextPayload.enable_mcp = payload.enableMcp;
  }

  const targetFilePath = join(cronDir(workspaceDir), `${payload.id.trim()}.json`);
  writeFileSync(targetFilePath, `${JSON.stringify(nextPayload, null, 2)}\n`, "utf8");

  if (existingId && existingId !== payload.id.trim()) {
    const previousFilePath = join(cronDir(workspaceDir), `${existingId}.json`);
    if (existsSync(previousFilePath)) {
      unlinkSync(previousFilePath);
    }
    const previousStatePath = taskStatePath(workspaceDir, existingId);
    if (existsSync(previousStatePath)) {
      unlinkSync(previousStatePath);
    }
  }

  return listTasks().find(task => task.id === payload.id.trim()) ?? normalizeTaskRecord(targetFilePath, readTaskState(workspaceDir, payload.id.trim()));
}

export function deleteTask(taskId: string) {
  const resolvedId = taskId.trim();
  if (!resolvedId) {
    throw new Error("缺少任务 ID。");
  }
  const { workspaceDir } = loadRuntimeConfig();
  const taskFilePath = join(cronDir(workspaceDir), `${resolvedId}.json`);
  if (existsSync(taskFilePath)) {
    unlinkSync(taskFilePath);
  }
  const stateFilePath = taskStatePath(workspaceDir, resolvedId);
  if (existsSync(stateFilePath)) {
    unlinkSync(stateFilePath);
  }
}

function readSessionMessages(sessionFile: string): RunMessage[] {
  if (!existsSync(sessionFile)) {
    return [];
  }
  return readFileSync(sessionFile, "utf8")
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(Boolean)
    .map(line => JSON.parse(line) as RunMessage);
}

function buildTaskStateBySession(workspaceDir: string) {
  const dir = stateDir(workspaceDir);
  const mapping = new Map<string, TaskStateInfo>();
  if (!existsSync(dir)) {
    return mapping;
  }
  for (const name of readdirSync(dir)) {
    if (!name.toLowerCase().endsWith(".json")) {
      continue;
    }
    const payload = safeReadJson<TaskStatePayload>(join(dir, name));
    const sessionFile = payload?.last_session_file?.trim();
    if (!payload || !sessionFile) {
      continue;
    }
    mapping.set(resolve(sessionFile), {
      taskId: payload.task_id?.trim() || basename(name, ".json"),
      lastStatus: (payload.last_status ?? "").trim(),
      lastError: (payload.last_error ?? "").trim(),
      lastStartedAt: normalizeDate(payload.last_started_at),
      lastFinishedAt: normalizeDate(payload.last_finished_at),
      sessionFile,
    });
  }
  return mapping;
}

function trimSummary(value: string, limit = 120) {
  const compact = value.replace(/\s+/g, " ").trim();
  if (!compact) {
    return "";
  }
  if (compact.length <= limit) {
    return compact;
  }
  return `${compact.slice(0, limit - 1)}…`;
}

function buildRunRecord(sessionFile: string, messages: RunMessage[], stateInfo?: TaskStateInfo): RunRecord {
  const id = basename(sessionFile, ".jsonl");
  const firstMessage = messages[0];
  const firstUserMessage = messages.find(message => message.role === "user" && message.kind === "message");
  const assistantMessages = messages.filter(message => message.role === "assistant" && message.kind === "message");
  const lastAssistantMessage = assistantMessages.at(-1);
  const errorMessages = messages.filter(message => message.isError);
  const createdAt = normalizeDate(firstMessage?.ts) ?? statSync(sessionFile).mtime.toISOString();
  const prompt = firstUserMessage?.content?.trim() || "";
  const fallbackTitle = prompt ? trimSummary(prompt, 36) : id;
  const status = stateInfo?.lastStatus === "success"
    ? "success"
    : stateInfo?.lastStatus === "failed"
      ? "failed"
      : errorMessages.length > 0
        ? "failed"
        : lastAssistantMessage
          ? "success"
          : "unknown";
  return {
    id,
    title: stateInfo?.taskId || fallbackTitle,
    prompt,
    source: stateInfo ? "cron" : "agent",
    status,
    createdAt,
    finishedAt: stateInfo?.lastFinishedAt ?? normalizeDate(messages.at(-1)?.ts),
    taskId: stateInfo?.taskId ?? null,
    sessionFile,
    summary: trimSummary(stateInfo?.lastError || lastAssistantMessage?.content || errorMessages.at(-1)?.content || prompt || "无摘要"),
    messageCount: messages.length,
    toolCallCount: messages.filter(message => message.kind === "tool").length,
    errorCount: errorMessages.length,
  };
}

export function listRuns() {
  const { workspaceDir } = loadRuntimeConfig();
  const dir = sessionsDir(workspaceDir);
  if (!existsSync(dir)) {
    return [];
  }
  const taskStateBySession = buildTaskStateBySession(workspaceDir);
  return readdirSync(dir)
    .filter(name => name.toLowerCase().endsWith(".jsonl"))
    .map(name => join(dir, name))
    .map(sessionFile => {
      const resolvedPath = resolve(sessionFile);
      return buildRunRecord(resolvedPath, readSessionMessages(resolvedPath), taskStateBySession.get(resolvedPath));
    })
    .sort((left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime());
}

export function getRunDetail(runId: string): RunDetail {
  const { workspaceDir } = loadRuntimeConfig();
  const sessionFile = join(sessionsDir(workspaceDir), `${runId}.jsonl`);
  if (!existsSync(sessionFile)) {
    throw new Error("执行记录不存在。");
  }
  const taskStateBySession = buildTaskStateBySession(workspaceDir);
  const messages = readSessionMessages(sessionFile);
  return {
    ...buildRunRecord(sessionFile, messages, taskStateBySession.get(resolve(sessionFile))),
    messages,
  };
}

function walkSkillFiles(directory: string, acc: string[] = []) {
  if (!existsSync(directory)) {
    return acc;
  }
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const entryPath = join(directory, entry.name);
    if (entry.isDirectory()) {
      walkSkillFiles(entryPath, acc);
      continue;
    }
    if (entry.isFile() && entry.name.toLowerCase() === "skill.md") {
      acc.push(entryPath);
    }
  }
  return acc;
}

function parseSkillDocument(document: string, fallbackSlug: string) {
  const lines = document.split(/\r?\n/);
  let index = 0;
  while (index < lines.length && !lines[index]?.trim()) {
    index += 1;
  }
  let name = fallbackSlug;
  const headingLine = lines[index]?.trim();
  if (headingLine?.startsWith("# ")) {
    name = headingLine.slice(2).trim() || fallbackSlug;
    index += 1;
  }
  while (index < lines.length && !lines[index]?.trim()) {
    index += 1;
  }
  const descriptionLines: string[] = [];
  while (index < lines.length) {
    const line = lines[index]?.trim();
    if (!line) {
      break;
    }
    descriptionLines.push(line);
    index += 1;
  }
  while (index < lines.length && !lines[index]?.trim()) {
    index += 1;
  }
  const content = lines.slice(index).join("\n").trim();
  return {
    name,
    description: descriptionLines.join(" ").trim(),
    content,
  };
}

function buildSkillRecord(skillFilePath: string): SkillRecord {
  const slug = basename(dirname(skillFilePath));
  const document = readFileSync(skillFilePath, "utf8");
  const parsed = parseSkillDocument(document, slug);
  return {
    id: slug,
    slug,
    name: parsed.name,
    description: parsed.description,
    skillFilePath,
    updatedAt: statSync(skillFilePath).mtime.toISOString(),
  };
}

export function listSkills() {
  const { workspaceDir } = loadRuntimeConfig();
  return walkSkillFiles(skillsDir(workspaceDir))
    .map(buildSkillRecord)
    .sort((left, right) => left.slug.localeCompare(right.slug, "zh-CN"));
}

export function getSkillDetail(slug: string): SkillDetail {
  const skill = listSkills().find(item => item.slug === slug.trim());
  if (!skill) {
    throw new Error("Skill 不存在。");
  }
  const document = readFileSync(skill.skillFilePath, "utf8");
  const parsed = parseSkillDocument(document, skill.slug);
  return {
    ...skill,
    name: parsed.name,
    description: parsed.description,
    content: parsed.content,
  };
}

function validateSkillPayload(payload: SkillWritePayload, existingSlug?: string) {
  const slug = payload.slug.trim();
  if (!slug) {
    throw new Error("Skill slug 不能为空。");
  }
  if (!/^[a-zA-Z0-9._-]+$/.test(slug)) {
    throw new Error("Skill slug 只能包含字母、数字、点、下划线和中划线。");
  }
  if (!payload.name.trim()) {
    throw new Error("Skill 名称不能为空。");
  }
  if (!payload.content?.trim() && !payload.description?.trim()) {
    throw new Error("Skill 内容不能为空。");
  }
  if (existingSlug && existingSlug !== slug && listSkills().some(item => item.slug === slug)) {
    throw new Error("同名 Skill 已存在。");
  }
}

function renderSkillDocument(payload: SkillWritePayload) {
  const parts = [`# ${payload.name.trim()}`];
  const description = payload.description?.trim();
  if (description) {
    parts.push(description);
  }
  const content = payload.content?.trim();
  if (content) {
    parts.push(content);
  }
  return `${parts.join("\n\n").trim()}\n`;
}

export function saveSkill(payload: SkillWritePayload, existingSlug?: string) {
  validateSkillPayload(payload, existingSlug);
  const { workspaceDir } = loadRuntimeConfig();
  const slug = payload.slug.trim();
  const targetDir = join(skillsDir(workspaceDir), slug);
  mkdirSync(targetDir, { recursive: true });
  const targetFilePath = join(targetDir, "SKILL.md");
  writeFileSync(targetFilePath, renderSkillDocument(payload), "utf8");
  if (existingSlug && existingSlug !== slug) {
    const previousDir = join(skillsDir(workspaceDir), existingSlug);
    if (existsSync(previousDir)) {
      rmSync(previousDir, { recursive: true, force: true });
    }
  }
  return getSkillDetail(slug);
}

export function deleteSkill(slug: string) {
  const resolvedSlug = slug.trim();
  if (!resolvedSlug) {
    throw new Error("缺少 Skill slug。");
  }
  const { workspaceDir } = loadRuntimeConfig();
  rmSync(join(skillsDir(workspaceDir), resolvedSlug), { recursive: true, force: true });
}

function isToday(value: string | null) {
  if (!value) {
    return false;
  }
  const date = new Date(value);
  const now = new Date();
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate();
}

export function getDashboardSummary(): DashboardSummary {
  const tasks = listTasks();
  const runs = listRuns();
  const runningTask = tasks.find(task => task.running);
  return {
    totalTasks: tasks.length,
    enabledTasks: tasks.filter(task => task.enabled).length,
    dueTasks: tasks.filter(task => task.due).length,
    runningTasks: tasks.filter(task => task.running).length,
    totalSkills: listSkills().length,
    successToday: runs.filter(run => run.status === "success" && isToday(run.createdAt)).length,
    failedToday: runs.filter(run => run.status === "failed" && isToday(run.createdAt)).length,
    currentRunId: runningTask?.id ?? null,
    pendingQueueCount: tasks.filter(task => task.due || task.running).length,
    recentRuns: runs.slice(0, 8),
  };
}