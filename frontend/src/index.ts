import { serve } from "bun";
import { join, resolve } from "node:path";

import index from "./index.html";
import { deleteSkill, deleteTask, getBinaryMode, getDashboardSummary, getRunDetail, getSkillDetail, listRuns, listSkills, listTasks, loadRuntimeConfig, saveSkill, saveTask } from "./serverData";
import type { SkillWritePayload, TaskWritePayload } from "./types";

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
  if (getBinaryMode(miniclawBinaryPath) === "binary") {
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

async function runCronTrigger(taskId: string) {
  const args = ["cron", "trigger", "--id", taskId];
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
    command: "agent" as CliCommand,
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
      const runtimeConfig = loadRuntimeConfig();
      const binaryMode: BinaryMode = getBinaryMode(miniclawBinaryPath);
      return Response.json({
        ok: true,
        service: "MiniClaw Frontend",
        port: frontendPort,
        repoRoot,
        frontendRoot,
        homeDir: runtimeConfig.homeDir,
        workspaceDir: runtimeConfig.workspaceDir,
        configPath: runtimeConfig.configPath,
        mcpConfigPath: runtimeConfig.mcpConfigPath,
        cliPath: binaryMode === "binary" ? "./miniclaw" : "go run cmd/miniclaw/main.go",
        binaryMode,
        gatewayChannel: runtimeConfig.gatewayChannel,
        llmBaseUrl: runtimeConfig.baseUrl,
        llmModel: runtimeConfig.model,
        hasLlmApiKey: Boolean(runtimeConfig.apiKey),
        enableMcp: runtimeConfig.enableMcp,
        enableAutoSkills: runtimeConfig.enableAutoSkills,
        enableSkillScoring: runtimeConfig.enableSkillScoring,
        qqWebhook: `http://${runtimeConfig.qqWebhookHost}:${runtimeConfig.qqWebhookPort}${runtimeConfig.qqWebhookPath}`,
        qqAllowUsers: runtimeConfig.qqAllowUsers,
        qqAllowGroups: runtimeConfig.qqAllowGroups,
        weixinAllowUsers: runtimeConfig.weixinAllowUsers,
        weixinConfigured: Boolean(runtimeConfig.weixinToken),
        maxToolIterations: runtimeConfig.maxToolIterations,
        serverTime: new Date().toISOString(),
      });
    },

    "/api/dashboard": async () => Response.json(getDashboardSummary()),

    "/api/tasks": {
      async GET() {
        return Response.json(listTasks());
      },
      async POST(request) {
        try {
          const payload = await parseJson<TaskWritePayload>(request);
          return Response.json(saveTask(payload));
        } catch (error) {
          return jsonError(error);
        }
      },
    },

    "/api/tasks/item": {
      async PUT(request) {
        try {
          const taskId = new URL(request.url).searchParams.get("id")?.trim();
          if (!taskId) {
            throw new RequestError("task id is required");
          }
          const payload = await parseJson<TaskWritePayload>(request);
          return Response.json(saveTask(payload, taskId));
        } catch (error) {
          return jsonError(error);
        }
      },
      async DELETE(request) {
        try {
          const taskId = new URL(request.url).searchParams.get("id")?.trim();
          if (!taskId) {
            throw new RequestError("task id is required");
          }
          deleteTask(taskId);
          return Response.json({ ok: true });
        } catch (error) {
          return jsonError(error);
        }
      },
    },

    "/api/tasks/run": {
      async POST(request) {
        try {
          const taskId = new URL(request.url).searchParams.get("id")?.trim();
          if (!taskId) {
            throw new RequestError("task id is required");
          }
          return Response.json(await runCronTrigger(taskId));
        } catch (error) {
          return jsonError(error);
        }
      },
    },

    "/api/runs": async () => Response.json(listRuns()),

    "/api/run": async request => {
      try {
        const runId = new URL(request.url).searchParams.get("id")?.trim();
        if (!runId) {
          throw new RequestError("run id is required");
        }
        return Response.json(getRunDetail(runId));
      } catch (error) {
        return jsonError(error);
      }
    },

    "/api/skills": {
      async GET() {
        return Response.json(listSkills());
      },
      async POST(request) {
        try {
          const payload = await parseJson<SkillWritePayload>(request);
          return Response.json(saveSkill(payload));
        } catch (error) {
          return jsonError(error);
        }
      },
    },

    "/api/skills/item": {
      async GET(request) {
        try {
          const slug = new URL(request.url).searchParams.get("slug")?.trim();
          if (!slug) {
            throw new RequestError("skill slug is required");
          }
          return Response.json(getSkillDetail(slug));
        } catch (error) {
          return jsonError(error);
        }
      },
      async PUT(request) {
        try {
          const slug = new URL(request.url).searchParams.get("slug")?.trim();
          if (!slug) {
            throw new RequestError("skill slug is required");
          }
          const payload = await parseJson<SkillWritePayload>(request);
          return Response.json(saveSkill(payload, slug));
        } catch (error) {
          return jsonError(error);
        }
      },
      async DELETE(request) {
        try {
          const slug = new URL(request.url).searchParams.get("slug")?.trim();
          if (!slug) {
            throw new RequestError("skill slug is required");
          }
          deleteSkill(slug);
          return Response.json({ ok: true });
        } catch (error) {
          return jsonError(error);
        }
      },
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
