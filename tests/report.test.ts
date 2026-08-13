import { execFile } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { promisify } from "node:util";

import { afterEach, describe, expect, it } from "vitest";

import { authorityDocumentPaths } from "../src/constants.js";
import { ExecFileProcessRunner } from "../src/process.js";
import { createReport } from "../src/report.js";
import { inspectRepository } from "../src/repository.js";
import {
  createGitRepository,
  FakeAuthorityClient,
  removeDirectory,
} from "./helpers.js";

const temporaryDirectories: string[] = [];
const execFileAsync = promisify(execFile);

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map(removeDirectory));
});

describe("report-only inventory", () => {
  it("observes native surfaces and direct just ci without executing recipes", async () => {
    const root = await createGitRepository();
    temporaryDirectories.push(root);
    await writeFile(
      join(root, "justfile"),
      "init:\n    @true\n\ncheck:\n    @true\n\nci: check\n    @true\n",
    );
    await writeFile(join(root, "package.json"), '{"type":"module"}\n');
    await writeFile(join(root, "pnpm-lock.yaml"), "lockfileVersion: '9.0'\n");
    await mkdir(join(root, ".github", "workflows"), { recursive: true });
    await writeFile(
      join(root, ".github", "workflows", "ci.yml"),
      "name: CI\non: pull_request\njobs:\n  check:\n    runs-on: ubuntu-24.04\n    steps:\n      - run: just ci\n",
    );

    const files = Object.fromEntries(
      authorityDocumentPaths.map((path) => [path, `# ${path}\n`]),
    );
    files["docs/standards/developer-tooling/README.md"] =
      "- Standard version: `2026.08.8`\n- Contract version: `golden-path/v1`\nActive central executable tooling: **none**.\n";
    const report = await createReport({
      authorityClient: new FakeAuthorityClient("c".repeat(40), files),
      homeDirectory: root,
      root,
      runner: new ExecFileProcessRunner(),
    });

    expect(report.classification).toBe("report-only");
    expect(report.repository.nativeSurfaces).toEqual([
      {
        id: null,
        lock: "pnpm-lock.yaml",
        lockPresent: true,
        manifest: "package.json",
        profile: "node-typescript",
        root: ".",
        source: "observed",
      },
    ]);
    expect(report.repository.applicability.classification).toBe("applicable");
    expect(report.repository.observationScope.query).toBe(
      "git ls-files --cached --others --exclude-standard -z --",
    );
    expect(report.observation.startedAt).toMatch(/Z$/);
    expect(report.observation.endedAt).toMatch(/Z$/);
    expect(report.package.contentSha256).toMatch(/^sha256:[a-f0-9]{64}$/);
    expect(report.skills.map((skill) => skill.host)).toEqual([
      "codex",
      "claude-code",
    ]);
    expect(report.repository.directJustCiWorkflows).toEqual([
      ".github/workflows/ci.yml",
    ]);
    expect(report.disclaimer).toContain("does not run repository CI");
  });

  it("never returns credentials from configured origin components", async () => {
    const root = await createGitRepository();
    temporaryDirectories.push(root);
    const origins = [
      {
        expected: "https://github.com/5010-dev/example.git",
        secrets: ["ghp-query-secret", "oauth-fragment"],
        value:
          "https://github.com/5010-dev/example.git?access_token=ghp-query-secret#oauth-fragment",
      },
      {
        expected: "https://github.com/5010-dev/example.git",
        secrets: ["oauth2", "gho-oauth-secret"],
        value:
          "https://oauth2:gho-oauth-secret@github.com/5010-dev/example.git",
      },
      {
        expected: "https://github.com/5010-dev/example.git",
        secrets: ["developer%40example.com", "ghp%5Fencoded%2Fsecret"],
        value:
          "https://developer%40example.com:ghp%5Fencoded%2Fsecret@github.com/5010-dev/example.git",
      },
      {
        expected: "github.com:5010-dev/example.git",
        secrets: ["git@", "ghp-scp-secret", "credential-fragment"],
        value:
          "git@github.com:5010-dev/example.git?token=ghp-scp-secret#credential-fragment",
      },
    ];

    for (const [index, origin] of origins.entries()) {
      await execFileAsync("git", [
        "-C",
        root,
        "remote",
        index === 0 ? "add" : "set-url",
        "origin",
        origin.value,
      ]);
      const inventory = await inspectRepository(
        new ExecFileProcessRunner(),
        root,
      );
      expect(inventory.snapshot.origin).toBe(origin.expected);
      for (const secret of origin.secrets) {
        expect(inventory.snapshot.origin).not.toContain(secret);
      }
      expect(inventory.snapshot.originSha256).toMatch(/^sha256:[a-f0-9]{64}$/);
    }
  });

  it("discovers unambiguous nested Node, Go, and Python native roots", async () => {
    const root = await createGitRepository();
    temporaryDirectories.push(root);
    await mkdir(join(root, "apps", "web"), { recursive: true });
    await mkdir(join(root, "services", "api"), { recursive: true });
    await mkdir(join(root, "tools", "jobs"), { recursive: true });
    await writeFile(join(root, "apps", "web", "package.json"), "{}\n");
    await writeFile(
      join(root, "apps", "web", "pnpm-lock.yaml"),
      "lockfileVersion: '9.0'\n",
    );
    await writeFile(
      join(root, "services", "api", "go.mod"),
      "module example.invalid/api\n",
    );
    await writeFile(join(root, "services", "api", "go.sum"), "");
    await writeFile(
      join(root, "tools", "jobs", "pyproject.toml"),
      "[project]\nname='jobs'\nversion='0.0.0'\n",
    );
    await writeFile(join(root, "tools", "jobs", "uv.lock"), "version = 1\n");

    const inventory = await inspectRepository(
      new ExecFileProcessRunner(),
      root,
    );
    expect(inventory.applicability.classification).toBe("applicable");
    expect(
      inventory.nativeSurfaces.map((surface) => [
        surface.profile,
        surface.root,
      ]),
    ).toEqual([
      ["node-typescript", "apps/web"],
      ["go", "services/api"],
      ["python", "tools/jobs"],
    ]);
  });

  it("keeps repeated native profiles pending until repository-owned roots classify them", async () => {
    const root = await createGitRepository();
    temporaryDirectories.push(root);
    for (const path of ["packages/one", "packages/two"]) {
      await mkdir(join(root, path), { recursive: true });
      await writeFile(join(root, path, "package.json"), "{}\n");
      await writeFile(
        join(root, path, "pnpm-lock.yaml"),
        "lockfileVersion: '9.0'\n",
      );
    }
    expect(
      (await inspectRepository(new ExecFileProcessRunner(), root)).applicability
        .classification,
    ).toBe("pending-classification");

    await mkdir(join(root, ".github"), { recursive: true });
    await writeFile(
      join(root, ".github", "golden-path-native-roots.yaml"),
      "schemaVersion: golden-path-native-roots/v1\nroots:\n  - id: one\n    path: packages/one\n    profiles: [node-typescript]\n  - id: two\n    path: packages/two\n    profiles: [node-typescript]\n",
    );
    const declared = await inspectRepository(new ExecFileProcessRunner(), root);
    expect(declared.applicability.classification).toBe("applicable");
    expect(
      declared.nativeSurfaces.every((surface) => surface.source === "declared"),
    ).toBe(true);
  });
});
