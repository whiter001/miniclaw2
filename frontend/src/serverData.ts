import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { basename, dirname, extname, isAbsolute, join, relative, resolve, sep } from "node:path";

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
const skillSlugPattern = /^[a-zA-Z0-9._-]+$/;
const workspaceIdPattern = /^[a-zA-Z0-9._-]+$/;
const internalSkillDirs = new Set(["_candidates", "_archived"]);

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

export function loadRuntimeConfig(): RuntimeConfig {
  const frontendRoot = resolve(import.meta.dir, "..");
  const repoRoot = resolve(frontendRoot, "..");
  const miniclawBinaryPath = join(repoRoot, "miniclaw");

  const cmd = existsSync(miniclawBinaryPath)
    ? [miniclawBinaryPath, "config", "--json"]
    : ["go", "run", "cmd/miniclaw/main.go", "config", "--json"];

  try {
    const proc = Bun.spawnSync({
      cmd,
      cwd: repoRoot,
      env: process.env,
    });

    if (proc.success) {
      const parsed = JSON.parse(proc.stdout.toString());
      return {
        homeDir: parsed.home_dir,
        workspaceDir: parsed.workspace,
        configPath: parsed.config_path,
        mcpConfigPath: parsed.mcp_config_path,
        apiKey: parsed.api_key,
        baseUrl: parsed.base_url,
        model: parsed.model,
        enableMcp: parsed.enable_mcp,
        gatewayChannel: parsed.gateway_channel,
        qqWebhookHost: parsed.qq_webhook_host,
        qqWebhookPort: parsed.qq_webhook_port,
        qqWebhookPath: parsed.qq_webhook_path,
        qqAllowUsers: parsed.qq_allow_users,
        qqAllowGroups: parsed.qq_allow_groups,
        weixinAllowUsers: parsed.weixin_allow_users,
        weixinToken: parsed.weixin_token,
        enableAutoSkills: parsed.enable_auto_skills,
        enableSkillScoring: parsed.enable_skill_scoring,
        maxToolIterations: parsed.max_tool_iterations,
      };
    }
  } catch (e) {
    console.error("Failed to load config via CLI, using defaults:", e);
  }

  return defaultConfig();
}

export function getBinaryMode(miniclawBinaryPath: string): BinaryMode {
  return existsSync(miniclawBinaryPath) ? "binary" : "go-run";
}

function runCliSync(args: string[]) {
  const frontendRoot = resolve(import.meta.dir, "..");
  const repoRoot = resolve(frontendRoot, "..");
  const miniclawBinaryPath = join(repoRoot, "miniclaw");

  const cmd = existsSync(miniclawBinaryPath)
    ? [miniclawBinaryPath, ...args]
    : ["go", "run", "cmd/miniclaw/main.go", ...args];

  return Bun.spawnSync({
    cmd,
    cwd: repoRoot,
    env: process.env,
  });
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

function normalizeSkillSlug(rawSlug: string, emptyMessage = "Skill slug 不能为空。") {
  return normalizeWorkspaceId(rawSlug, emptyMessage, "Skill slug", skillSlugPattern);
}

function normalizeTaskId(rawId: string, emptyMessage = "任务 ID 不能为空。") {
  return normalizeWorkspaceId(rawId, emptyMessage, "任务 ID", workspaceIdPattern);
}

function normalizeRunId(rawId: string, emptyMessage = "执行记录 ID 不能为空。") {
  return normalizeWorkspaceId(rawId, emptyMessage, "执行记录 ID", workspaceIdPattern);
}

function normalizeWorkspaceId(rawId: string, emptyMessage: string, label: string, pattern: RegExp) {
  const id = rawId.trim();
  if (!id) {
    throw new Error(emptyMessage);
  }
  if (!pattern.test(id) || id === "." || id === "..") {
    throw new Error(`${label} 只能包含字母、数字、点、下划线和中划线，且不能是 . 或 ..。`);
  }
  return id;
}

function resolveChildFile(rootDirectory: string, id: string, extension: string) {
  const root = resolve(rootDirectory);
  const target = resolve(root, `${id}${extension}`);
  const relativePath = relative(root, target);
  if (!relativePath || relativePath === ".." || relativePath.startsWith(`..${sep}`) || isAbsolute(relativePath)) {
    throw new Error("文件路径必须位于目标目录内。");
  }
  return target;
}

function resolveSkillDir(workspaceDir: string, slug: string) {
  const root = resolve(skillsDir(workspaceDir));
  const target = resolve(root, slug);
  const relativePath = relative(root, target);
  if (!relativePath || relativePath === ".." || relativePath.startsWith(`..${sep}`) || isAbsolute(relativePath)) {
    throw new Error("Skill 路径必须位于 skills 目录内。");
  }
  return target;
}

function normalizeDate(value?: string) {
  if (!value) {
    return null;
  }
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? null : parsed.toISOString();
}

function taskStatePath(workspaceDir: string, taskId: string) {
  return resolveChildFile(stateDir(workspaceDir), normalizeTaskId(taskId), ".json");
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
  const id = normalizeTaskId(payload.id);
  const previousId = existingId === undefined ? "" : normalizeTaskId(existingId, "缺少任务 ID。");
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
  if (previousId && previousId !== id && existsSync(resolveChildFile(cronDir(loadRuntimeConfig().workspaceDir), id, ".json"))) {
    throw new Error("同名任务已存在。");
  }
  return { id, previousId };
}

export function saveTask(payload: TaskWritePayload, existingId?: string) {
  const { id, previousId } = validateTaskPayload(payload, existingId);
  
  if (previousId && previousId !== id) {
    deleteTask(previousId);
  }

  const args = [
    "cron", "add",
    "--id", id,
    "--schedule", payload.schedule.trim(),
    "-p", payload.prompt.trim(),
  ];
  
  const proc = runCliSync(args);
  if (!proc.success) {
    throw new Error(`Failed to save task: ${proc.stderr.toString()}`);
  }

  return listTasks().find(task => task.id === id)!;
}

export function deleteTask(taskId: string) {
  const resolvedId = normalizeTaskId(taskId, "缺少任务 ID。");
  const proc = runCliSync(["cron", "delete", "--id", resolvedId]);
  if (!proc.success) {
    throw new Error(`Failed to delete task: ${proc.stderr.toString()}`);
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
  const resolvedRunId = normalizeRunId(runId, "缺少执行记录 ID。");
  const sessionFile = resolveChildFile(sessionsDir(workspaceDir), resolvedRunId, ".jsonl");
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
      if (internalSkillDirs.has(entry.name)) {
        continue;
      }
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
  const requestedSlug = normalizeSkillSlug(slug, "缺少 Skill slug。");
  const skill = listSkills().find(item => item.slug === requestedSlug);
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
  const slug = normalizeSkillSlug(payload.slug);
  const previousSlug = existingSlug === undefined ? "" : normalizeSkillSlug(existingSlug, "缺少 Skill slug。");
  if (!payload.name.trim()) {
    throw new Error("Skill 名称不能为空。");
  }
  if (!payload.content?.trim() && !payload.description?.trim()) {
    throw new Error("Skill 内容不能为空。");
  }
  if (previousSlug && previousSlug !== slug && listSkills().some(item => item.slug === slug)) {
    throw new Error("同名 Skill 已存在。");
  }
  return { slug, previousSlug };
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
  const { slug, previousSlug } = validateSkillPayload(payload, existingSlug);
  
  if (previousSlug && previousSlug !== slug) {
    deleteSkill(previousSlug);
  }

  const proc = runCliSync([
    "skill", "create",
    "--name", slug,
    "-p", payload.content.trim(),
  ]);

  if (!proc.success) {
    throw new Error(`Failed to save skill: ${proc.stderr.toString()}`);
  }

  return getSkillDetail(slug);
}

export function deleteSkill(slug: string) {
  const resolvedSlug = normalizeSkillSlug(slug, "缺少 Skill slug。");
  const proc = runCliSync(["skill", "delete", "--name", resolvedSlug]);
  if (!proc.success) {
    throw new Error(`Failed to delete skill: ${proc.stderr.toString()}`);
  }
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