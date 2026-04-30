import { afterEach, expect, test } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { deleteSkill, deleteTask, getRunDetail, getSkillDetail, listSkills, saveSkill, saveTask } from "./serverData";

const originalWorkspace = process.env.MINICLAW_WORKSPACE;
let workspaces: string[] = [];

function useTempWorkspace() {
  const workspace = mkdtempSync(join(tmpdir(), "miniclaw-frontend-"));
  workspaces.push(workspace);
  process.env.MINICLAW_WORKSPACE = workspace;
  return workspace;
}

afterEach(() => {
  for (const workspace of workspaces) {
    rmSync(workspace, { recursive: true, force: true });
  }
  workspaces = [];
  if (originalWorkspace === undefined) {
    delete process.env.MINICLAW_WORKSPACE;
  } else {
    process.env.MINICLAW_WORKSPACE = originalWorkspace;
  }
});

test("saveSkill and deleteSkill handle a normal slug", () => {
  const workspace = useTempWorkspace();

  saveSkill({ slug: "daily-note", name: "Daily Note", description: "Capture recurring notes." });
  expect(getSkillDetail("daily-note").name).toBe("Daily Note");

  deleteSkill("daily-note");
  expect(existsSync(join(workspace, "skills", "daily-note"))).toBe(false);
});

test("deleteSkill rejects dot traversal slugs", () => {
  const workspace = useTempWorkspace();
  saveSkill({ slug: "safe", name: "Safe", description: "A safe skill." });

  expect(() => deleteSkill("..")).toThrow(/不能是/);
  expect(() => deleteSkill(".")).toThrow(/不能是/);
  expect(existsSync(workspace)).toBe(true);
  expect(existsSync(join(workspace, "skills", "safe", "SKILL.md"))).toBe(true);
});

test("saveSkill rejects an unsafe existing slug before renaming", () => {
  const workspace = useTempWorkspace();
  saveSkill({ slug: "safe", name: "Safe", description: "A safe skill." });

  expect(() => saveSkill({ slug: "renamed", name: "Renamed", description: "Still safe." }, "..")).toThrow(/不能是/);
  expect(existsSync(workspace)).toBe(true);
  expect(existsSync(join(workspace, "skills", "safe", "SKILL.md"))).toBe(true);
  expect(existsSync(join(workspace, "skills", "renamed"))).toBe(false);
});

test("task and run ids reject path traversal", () => {
  const workspace = useTempWorkspace();
  saveTask({
    id: "safe-task",
    description: "Safe task",
    schedule: "0 * * * *",
    prompt: "run safely",
    enabled: true,
    skipIfRunning: true,
    enableMcp: false,
    timeoutSeconds: 60,
    maxToolIterations: 10,
  });

  expect(() => deleteTask("../safe-task")).toThrow(/不能是/);
  expect(() => saveTask({
    id: "renamed-task",
    description: "Renamed task",
    schedule: "0 * * * *",
    prompt: "run safely",
    enabled: true,
    skipIfRunning: true,
    enableMcp: false,
    timeoutSeconds: 60,
    maxToolIterations: 10,
  }, "../safe-task")).toThrow(/不能是/);
  expect(() => getRunDetail("../secret")).toThrow(/不能是/);
  expect(existsSync(join(workspace, "cron", "safe-task.json"))).toBe(true);
  expect(existsSync(join(workspace, "cron", "renamed-task.json"))).toBe(false);
});

test("getRunDetail accepts a normal run id", () => {
  const workspace = useTempWorkspace();
  mkdirSync(join(workspace, "sessions"), { recursive: true });
  writeFileSync(join(workspace, "sessions", "session-20260430_120000.000.jsonl"), '{"kind":"message","role":"user","content":"hello"}\n', "utf8");

  const detail = getRunDetail("session-20260430_120000.000");
  expect(detail.id).toBe("session-20260430_120000.000");
  expect(detail.messages[0].content).toBe("hello");
});

test("listSkills hides candidate and archived skills", () => {
  const workspace = useTempWorkspace();
  saveSkill({ slug: "approved", name: "Approved", description: "Visible skill." });
  mkdirSync(join(workspace, "skills", "_candidates", "candidate"), { recursive: true });
  writeFileSync(join(workspace, "skills", "_candidates", "candidate", "SKILL.md"), "# Candidate\n\nHidden candidate skill.", "utf8");
  mkdirSync(join(workspace, "skills", "_archived", "archived"), { recursive: true });
  writeFileSync(join(workspace, "skills", "_archived", "archived", "SKILL.md"), "# Archived\n\nHidden archived skill.", "utf8");

  expect(listSkills().map(skill => skill.name)).toEqual(["Approved"]);
});
