import { serve } from "bun";
import { existsSync } from "node:fs";
import { join, resolve } from "node:path";

import index from "./index.html";

type BinaryMode = "binary" | "go-run";
type CliCommand = "status" | "agent";

interface CliRunRequest {
  command?: CliCommand;
  prompt?: string;
  workspace?: string;
  mcp?: boolean;
}

class RequestError extends Error {
  status: number;

  constructor(message: string, status = 400) {
    super(message);
    this.status = status;
  }
}

const frontendRoot = resolve(import.meta.dir, "..");
const repoRoot = resolve(frontendRoot, "..");
const miniclawBinaryPath = join(repoRoot, "miniclaw");
const requestTimeoutMs = 180_000;

function resolveFrontendPort() {
  const raw = process.env.MINICLAW_FRONTEND_PORT?.trim() || process.env.PORT?.trim() || "5020";
  const parsed = Number.parseInt(raw, 10);
  if (Number.isNaN(parsed) || parsed <= 0 || parsed > 65535) {
    return 5020;
  }
  return parsed;
}

const frontendPort = resolveFrontendPort();

function formatArg(value: string) {
  if (value === "") {
    return '""';
  }
  if (/^[a-zA-Z0-9_./:-]+$/.test(value)) {
    return value;
  }
  return JSON.stringify(value);
}

function resolveCliInvocation(args: string[]) {
  if (existsSync(miniclawBinaryPath)) {
    return {
      binaryMode: "binary" as BinaryMode,
      cliPath: "./miniclaw",
      cmd: [miniclawBinaryPath, ...args],
    };
  }
  return {
    binaryMode: "go-run" as BinaryMode,
    cliPath: "go run cmd/miniclaw/main.go",
    cmd: ["go", "run", "./cmd/miniclaw/main.go", ...args],
  };
}

function buildCliArgs(payload: CliRunRequest) {
  switch (payload.command) {
  case "status": {
    const args = ["status"];
    if (payload.workspace?.trim()) {
      args.push("--workspace", payload.workspace.trim());
    }
    return args;
  }
  case "agent": {
    const prompt = payload.prompt?.trim();
    if (!prompt) {
      throw new RequestError("prompt is required for the agent command.");
    }
    const args = ["agent", "-p", prompt];
    if (payload.workspace?.trim()) {
      args.push("--workspace", payload.workspace.trim());
    }
    if (payload.mcp) {
      args.push("--mcp");
    }
    return args;
  }
  default:
    throw new RequestError("unsupported command", 400);
  }
}

async function parseJson<T>(request: Request) {
  try {
    return (await request.json()) as T;
  } catch {
    throw new RequestError("request body must be valid JSON", 400);
  }
}

function jsonError(error: unknown) {
  if (error instanceof RequestError) {
    return Response.json({ error: error.message }, { status: error.status });
  }
  console.error(error);
  return Response.json({ error: error instanceof Error ? error.message : String(error) }, { status: 500 });
}

async function runCli(payload: CliRunRequest) {
  const args = buildCliArgs(payload);
  const invocation = resolveCliInvocation(args);
  const startedAt = Date.now();
  const proc = Bun.spawn({
    cmd: invocation.cmd,
    cwd: repoRoot,
    stdout: "pipe",
    stderr: "pipe",
    env: {
      ...process.env,
      NO_COLOR: "1",
    },
  });

  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    proc.kill();
  }, requestTimeoutMs);

  const [stdout, stderr, exitCode] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  clearTimeout(timer);

  if (timedOut) {
    throw new RequestError("MiniClaw execution timed out after 180 seconds.", 504);
  }

  return {
    command: payload.command as CliCommand,
    commandLine: `${invocation.cliPath} ${args.map(formatArg).join(" ")}`,
    stdout,
    stderr,
    exitCode,
    durationMs: Date.now() - startedAt,
    cliPath: invocation.cliPath,
    binaryMode: invocation.binaryMode,
    runAt: new Date().toISOString(),
  };
}

const server = serve({
  port: frontendPort,
  routes: {
    "/api/health": async () => {
      const binaryMode: BinaryMode = existsSync(miniclawBinaryPath) ? "binary" : "go-run";
      return Response.json({
        ok: true,
        port: frontendPort,
        repoRoot,
        frontendRoot,
        cliPath: binaryMode === "binary" ? "./miniclaw" : "go run cmd/miniclaw/main.go",
        binaryMode,
        serverTime: new Date().toISOString(),
      });
    },

    "/api/status": async () => {
      try {
        return Response.json(await runCli({ command: "status" }));
      } catch (error) {
        return jsonError(error);
      }
    },

    "/api/agent/run": {
      async POST(request) {
        try {
          const payload = await parseJson<CliRunRequest>(request);
          return Response.json(await runCli({ ...payload, command: "agent" }));
        } catch (error) {
          return jsonError(error);
        }
      },
    },

    "/api/cli/run": {
      async POST(request) {
        try {
          const payload = await parseJson<CliRunRequest>(request);
          return Response.json(await runCli(payload));
        } catch (error) {
          return jsonError(error);
        }
      },
    },

    "/*": index,
  },

  development: process.env.NODE_ENV !== "production" && {
    // Enable browser hot reloading in development
    hmr: true,

    // Echo console logs from the browser to the server
    console: true,
  },
});

console.log(`🚀 Server running at ${server.url}`);
