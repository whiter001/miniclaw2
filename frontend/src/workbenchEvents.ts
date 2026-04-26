export type PromptHistorySource = "agent" | "command";

export interface PromptHistoryItem {
	title: string;
	prompt: string;
	source: PromptHistorySource;
	runAt: string;
	exitCode: number;
	durationMs: number;
	commandLine: string;
}

export interface HistoryUpdatedDetail {
	history: PromptHistoryItem[];
}

export interface PromptRequestedDetail {
	prompt: string;
}

export const historyStorageKey = "miniclaw-agent-history";
export const promptRequestStorageKey = "miniclaw-agent-prompt-request";
export const historyUpdatedEvent = "miniclaw:history-updated";
export const promptRequestedEvent = "miniclaw:prompt-requested";

function isPromptHistorySource(value: unknown): value is PromptHistorySource {
	return value === "agent" || value === "command";
}

function normalizePromptHistoryItem(value: unknown): PromptHistoryItem | null {
	if (!value || typeof value !== "object") {
		return null;
	}
	const candidate = value as Record<string, unknown>;
	const prompt = typeof candidate.prompt === "string" ? candidate.prompt.trim() : "";
	const title = typeof candidate.title === "string" && candidate.title.trim() ? candidate.title.trim() : prompt;
	const runAt = typeof candidate.runAt === "string" ? candidate.runAt : "";
	const commandLine = typeof candidate.commandLine === "string" ? candidate.commandLine : "";
	const exitCode = typeof candidate.exitCode === "number" ? candidate.exitCode : Number(candidate.exitCode);
	const durationMs = typeof candidate.durationMs === "number" ? candidate.durationMs : Number(candidate.durationMs);
	if (!prompt || !title || !runAt || !commandLine || !Number.isFinite(exitCode) || !Number.isFinite(durationMs)) {
		return null;
	}
	return {
		title,
		prompt,
		source: isPromptHistorySource(candidate.source) ? candidate.source : "agent",
		runAt,
		exitCode,
		durationMs,
		commandLine,
	};
}

export function readPromptHistory(): PromptHistoryItem[] {
	if (typeof window === "undefined") {
		return [];
	}
	const raw = window.localStorage.getItem(historyStorageKey);
	if (!raw) {
		return [];
	}
	try {
		const parsed = JSON.parse(raw) as unknown;
		if (!Array.isArray(parsed)) {
			window.localStorage.removeItem(historyStorageKey);
			return [];
		}
		return parsed
			.map(normalizePromptHistoryItem)
			.filter((item): item is PromptHistoryItem => item !== null);
	} catch {
		window.localStorage.removeItem(historyStorageKey);
		return [];
	}
}

export function emitHistoryUpdated(history: PromptHistoryItem[]) {
	window.dispatchEvent(new CustomEvent<HistoryUpdatedDetail>(historyUpdatedEvent, { detail: { history } }));
}

export function writePromptHistory(history: PromptHistoryItem[]) {
	if (typeof window === "undefined") {
		return history;
	}
	window.localStorage.setItem(historyStorageKey, JSON.stringify(history));
	emitHistoryUpdated(history);
	return history;
	}

export function prependPromptHistory(item: PromptHistoryItem, limit = 12) {
	const nextHistory = [item, ...readPromptHistory()].slice(0, limit);
	return writePromptHistory(nextHistory);
}

export function readRequestedPrompt() {
	if (typeof window === "undefined") {
		return null;
	}
	return window.sessionStorage.getItem(promptRequestStorageKey);
}

export function clearRequestedPrompt() {
	if (typeof window === "undefined") {
		return;
	}
	window.sessionStorage.removeItem(promptRequestStorageKey);
}

export function consumeRequestedPrompt() {
	const prompt = readRequestedPrompt();
	clearRequestedPrompt();
	return prompt;
}

export function emitPromptRequested(prompt: string) {
	window.sessionStorage.setItem(promptRequestStorageKey, prompt);
	window.dispatchEvent(new CustomEvent<PromptRequestedDetail>(promptRequestedEvent, { detail: { prompt } }));
}