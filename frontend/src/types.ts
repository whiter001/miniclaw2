export interface TaskRecord {
  id: string;
  description: string;
  schedule: string;
  prompt: string;
  enabled: boolean;
  skipIfRunning: boolean;
  timeoutSeconds: number;
  maxToolIterations: number;
  enableMcp: boolean | null;
  filePath: string;
  due: boolean;
  running: boolean;
  nextRunAt: string | null;
  lastScheduledAt: string | null;
  lastStartedAt: string | null;
  lastFinishedAt: string | null;
  lastDurationSeconds: number;
  lastStatus: string;
  lastError: string;
  lastSessionFile: string;
}

export interface TaskWritePayload {
  id: string;
  description?: string;
  schedule: string;
  prompt: string;
  enabled?: boolean;
  skipIfRunning?: boolean;
  timeoutSeconds?: number;
  maxToolIterations?: number;
  enableMcp?: boolean | null;
}

export interface RunMessage {
  ts: string;
  kind: string;
  role?: string;
  content: string;
  toolName?: string;
  toolId?: string;
  isError?: boolean;
}

export interface RunRecord {
  id: string;
  title: string;
  prompt: string;
  source: "cron" | "agent";
  status: "success" | "failed" | "running" | "unknown";
  createdAt: string;
  finishedAt: string | null;
  taskId: string | null;
  sessionFile: string;
  summary: string;
  messageCount: number;
  toolCallCount: number;
  errorCount: number;
}

export interface RunDetail extends RunRecord {
  messages: RunMessage[];
}

export interface SkillRecord {
  id: string;
  slug: string;
  name: string;
  description: string;
  skillFilePath: string;
  updatedAt: string;
}

export interface SkillDetail extends SkillRecord {
  content: string;
}

export interface SkillWritePayload {
  slug: string;
  name: string;
  description?: string;
  content?: string;
}

export interface DashboardSummary {
  totalTasks: number;
  enabledTasks: number;
  dueTasks: number;
  runningTasks: number;
  totalSkills: number;
  successToday: number;
  failedToday: number;
  currentRunId: string | null;
  pendingQueueCount: number;
  recentRuns: RunRecord[];
}