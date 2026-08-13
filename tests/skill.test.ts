import { mkdir, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { join } from "node:path";

import { afterEach, describe, expect, it } from "vitest";

import {
  checkAgentSkill,
  installAgentSkill,
  resolveAgentSkillDestination,
} from "../src/skill.js";
import { removeDirectory, temporaryDirectory } from "./helpers.js";

const temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map(removeDirectory));
});

async function temporaryHome(): Promise<string> {
  const directory = await temporaryDirectory("golden-path-agent-home-");
  temporaryDirectories.push(directory);
  return directory;
}

describe("Skill installer", () => {
  it("installs idempotently in both official user locations", async () => {
    const homeDirectory = await temporaryHome();

    for (const host of ["codex", "claude-code"] as const) {
      const installed = await installAgentSkill({ homeDirectory, host });
      expect(installed.status).toBe("installed");
      expect(installed.destination).toBe(
        resolveAgentSkillDestination({ homeDirectory, host }),
      );
      expect((await checkAgentSkill({ homeDirectory, host })).status).toBe(
        "current",
      );
      expect((await installAgentSkill({ homeDirectory, host })).status).toBe(
        "unchanged",
      );

      const skill = await readFile(
        join(installed.destination, "SKILL.md"),
        "utf8",
      );
      expect(skill).toContain("repository-owned");
      if (host === "claude-code") {
        expect(skill).toContain("disable-model-invocation: true");
      } else {
        expect(skill).not.toContain("disable-model-invocation: true");
        expect(
          await readFile(
            join(installed.destination, "agents", "openai.yaml"),
            "utf8",
          ),
        ).toContain("allow_implicit_invocation: false");
      }
    }
  });

  it("protects unmanaged and locally modified directories", async () => {
    const homeDirectory = await temporaryHome();
    const host = "codex" as const;
    const destination = resolveAgentSkillDestination({ homeDirectory, host });
    await mkdir(destination, { recursive: true });
    await writeFile(join(destination, "SKILL.md"), "unmanaged\n");

    await expect(
      installAgentSkill({ force: true, homeDirectory, host }),
    ).rejects.toThrow("unmanaged skill");

    await rm(destination, { force: true, recursive: true });
    await installAgentSkill({ homeDirectory, host });
    await writeFile(join(destination, "SKILL.md"), "locally modified\n");
    expect((await checkAgentSkill({ homeDirectory, host })).status).toBe(
      "modified",
    );
    await expect(installAgentSkill({ homeDirectory, host })).rejects.toThrow(
      "locally modified",
    );
    expect(
      (await installAgentSkill({ force: true, homeDirectory, host })).status,
    ).toBe("updated");
  });

  it("rejects symlinked parents for both installation and checks", async () => {
    const homeDirectory = await temporaryHome();
    const redirected = await temporaryHome();
    await symlink(redirected, join(homeDirectory, ".agents"));

    await expect(
      installAgentSkill({ homeDirectory, host: "codex" }),
    ).rejects.toThrow("not a safe directory");
    await expect(
      checkAgentSkill({ homeDirectory, host: "codex" }),
    ).rejects.toThrow("not a safe directory");
  });
});
