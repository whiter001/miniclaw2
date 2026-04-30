import { afterEach, expect, test } from "bun:test";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { deleteSkill, getSkillDetail, saveSkill } from "./serverData";

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
