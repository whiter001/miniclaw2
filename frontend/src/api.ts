export type CliCommand = "status" | "agent";

import type { DashboardSummary, RunDetail, RunRecord, SkillDetail, SkillRecord, SkillWritePayload, TaskRecord, TaskWritePayload } from "./types";

export interface HealthResponse {
	ok: boolean;
	service?: string;
	host?: string;
	port?: number;
	repoRoot: string;
	cliPath: string;
	binaryMode: "binary" | "go-run";
	frontendRoot: string;
	homeDir?: string;
	workspaceDir?: string;
	configPath?: string;
	mcpConfigPath?: string;
	gatewayChannel?: string;
	llmBaseUrl?: string;
	llmModel?: string;
	hasLlmApiKey?: boolean;
	enableMcp?: boolean;
	enableAutoSkills?: boolean;
	enableSkillScoring?: boolean;
	authRequired?: boolean;
	qqWebhook?: string;
	qqAllowUsers?: string;
	qqAllowGroups?: string;
	weixinAllowUsers?: string;
	weixinConfigured?: boolean;
	maxToolIterations?: number;
	serverTime: string;
}

export interface CliRunRequest {
	command: CliCommand;
	prompt?: string;
	workspace?: string;
	mcp?: boolean;
}

export interface CliRunResponse {
	command: CliCommand;
	commandLine: string;
	stdout: string;
	stderr: string;
	exitCode: number;
	durationMs: number;
	cliPath: string;
	binaryMode: "binary" | "go-run";
	runAt: string;
}

interface ApiErrorPayload {
	error?: string;
}

async function requestJson<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
	const response = await fetch(input, {
		headers: {
			"Content-Type": "application/json",
			...(init?.headers ?? {}),
		},
		...init,
	});
	if (!response.ok) {
		const payload = (await response.json().catch(() => ({}))) as ApiErrorPayload;
		throw new Error(payload.error || `Request failed with status ${response.status}`);
	}
	return (await response.json()) as T;
}

export function fetchHealth() {
	return requestJson<HealthResponse>("/api/health");
}

export function fetchDashboardSummary() {
	return requestJson<DashboardSummary>("/api/dashboard");
}

export function listTasks() {
	return requestJson<TaskRecord[]>("/api/tasks");
}

export function createTask(payload: TaskWritePayload) {
	return requestJson<TaskRecord>("/api/tasks", {
		method: "POST",
		body: JSON.stringify(payload),
	});
}

export function updateTask(taskId: string, payload: TaskWritePayload) {
	return requestJson<TaskRecord>(`/api/tasks/item?id=${encodeURIComponent(taskId)}`, {
		method: "PUT",
		body: JSON.stringify(payload),
	});
}

export function deleteTask(taskId: string) {
	return requestJson<{ ok: boolean }>(`/api/tasks/item?id=${encodeURIComponent(taskId)}`, {
		method: "DELETE",
	});
}

export function runTask(taskId: string) {
	return requestJson<CliRunResponse>(`/api/tasks/run?id=${encodeURIComponent(taskId)}`, {
		method: "POST",
	});
}

export function listRuns() {
	return requestJson<RunRecord[]>("/api/runs");
}

export function getRunDetail(runId: string) {
	return requestJson<RunDetail>(`/api/run?id=${encodeURIComponent(runId)}`);
}

export function listSkills() {
	return requestJson<SkillRecord[]>("/api/skills");
}

export function getSkillDetail(slug: string) {
	return requestJson<SkillDetail>(`/api/skills/item?slug=${encodeURIComponent(slug)}`);
}

export function createSkill(payload: SkillWritePayload) {
	return requestJson<SkillDetail>("/api/skills", {
		method: "POST",
		body: JSON.stringify(payload),
	});
}

export function updateSkill(slug: string, payload: SkillWritePayload) {
	return requestJson<SkillDetail>(`/api/skills/item?slug=${encodeURIComponent(slug)}`, {
		method: "PUT",
		body: JSON.stringify(payload),
	});
}

export function deleteSkill(slug: string) {
	return requestJson<{ ok: boolean }>(`/api/skills/item?slug=${encodeURIComponent(slug)}`, {
		method: "DELETE",
	});
}

export function runCli(payload: CliRunRequest) {
	return requestJson<CliRunResponse>("/api/cli/run", {
		method: "POST",
		body: JSON.stringify(payload),
	});
}