export type CliCommand = "status" | "agent";

export interface HealthResponse {
	ok: boolean;
	port?: number;
	repoRoot: string;
	cliPath: string;
	binaryMode: "binary" | "go-run";
	frontendRoot: string;
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

export function runCli(payload: CliRunRequest) {
	return requestJson<CliRunResponse>("/api/cli/run", {
		method: "POST",
		body: JSON.stringify(payload),
	});
}