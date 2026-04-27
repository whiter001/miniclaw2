import type { TaskRecord, TaskWritePayload } from "./types";

export function formatDateTime(value?: string | null) {
  if (!value) {
    return "-";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

export function ellipsis(value: string | null | undefined, max = 80) {
  const source = (value ?? "").trim();
  if (!source) {
    return "-";
  }
  if (source.length <= max) {
    return source;
  }
  return `${source.slice(0, max - 1)}…`;
}

export function taskToPayload(task: TaskRecord): TaskWritePayload {
  return {
    id: task.id,
    description: task.description,
    schedule: task.schedule,
    prompt: task.prompt,
    enabled: task.enabled,
    skipIfRunning: task.skipIfRunning,
    timeoutSeconds: task.timeoutSeconds,
    maxToolIterations: task.maxToolIterations,
    enableMcp: task.enableMcp,
  };
}
