import { execFile } from "node:child_process";
import { mkdir, readFile, symlink, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { promisify } from "node:util";

import { afterEach, describe, expect, it } from "vitest";

import { copyOnceSources } from "../src/constants.js";
import { sha256 } from "../src/hashing.js";
import { applyPlan, createPlan } from "../src/plan.js";
import { ExecFileProcessRunner } from "../src/process.js";
import {
  createGitRepository,
  FakeAuthorityClient,
  removeDirectory,
  temporaryDirectory,
} from "./helpers.js";

const commit = "b".repeat(40);
const execFileAsync = promisify(execFile);
const temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map(removeDirectory));
});

async function fixture(): Promise<{
  authorityClient: FakeAuthorityClient;
  external: string;
  root: string;
  runner: ExecFileProcessRunner;
}> {
  const root = await createGitRepository();
  const external = await temporaryDirectory("golden-path-agent-external-");
  temporaryDirectories.push(root, external);
  return {
    authorityClient: new FakeAuthorityClient(commit, {
      [copyOnceSources["node-toolchain"]]:
        `min_version = "<EXACT_SUPPORTED_MISE_VERSION>"\n\n[tools]\nnode = "<EXACT_SUPPORTED_NODE_PATCH>"\njust = "<EXACT_JUST_VERSION>"\n`,
    }),
    external,
    root,
    runner: new ExecFileProcessRunner(),
  };
}

async function writeRequest(
  external: string,
  destination = "mise.toml",
): Promise<string> {
  const request = join(external, "request.json");
  await writeFile(
    request,
    `${JSON.stringify(
      {
        copies: [
          {
            destination,
            overwrite: false,
            replacements: [
              {
                expectedOccurrences: 1,
                match: "<EXACT_SUPPORTED_MISE_VERSION>",
                value: "2026.7.18",
              },
              {
                expectedOccurrences: 1,
                match: "<EXACT_SUPPORTED_NODE_PATCH>",
                value: "24.18.1",
              },
              {
                expectedOccurrences: 1,
                match: "<EXACT_JUST_VERSION>",
                value: "1.57.0",
              },
            ],
            source: "node-toolchain",
          },
        ],
        schemaVersion: 1,
      },
      null,
      2,
    )}\n`,
  );
  return request;
}

async function writeAliasedRequest(
  external: string,
  destinations: readonly [string, string],
  filename = "aliased-request.json",
): Promise<string> {
  const request = join(external, filename);
  await writeFile(
    request,
    `${JSON.stringify(
      {
        copies: destinations.map((destination) => ({
          destination,
          overwrite: false,
          replacements: [
            {
              expectedOccurrences: 1,
              match: "<EXACT_SUPPORTED_MISE_VERSION>",
              value: "2026.7.18",
            },
            {
              expectedOccurrences: 1,
              match: "<EXACT_SUPPORTED_NODE_PATCH>",
              value: "24.18.1",
            },
            {
              expectedOccurrences: 1,
              match: "<EXACT_JUST_VERSION>",
              value: "1.57.0",
            },
          ],
          source: "node-toolchain",
        })),
        schemaVersion: 1,
      },
      null,
      2,
    )}\n`,
  );
  return request;
}

describe("copy-once plan and apply", () => {
  it("plans without repository writes and applies only the unchanged plan", async () => {
    const context = await fixture();
    const request = await writeRequest(context.external);
    const output = join(context.external, "plan.json");

    const plan = await createPlan({ ...context, output, request });
    expect(plan.source.commit).toBe(commit);
    expect(plan.package.contentSha256).toMatch(/^sha256:[a-f0-9]{64}$/);
    expect(plan.observation.startedAt).toMatch(/Z$/);
    await expect(
      readFile(join(context.root, "mise.toml")),
    ).rejects.toMatchObject({
      code: "ENOENT",
    });

    const result = await applyPlan({
      ...context,
      approvedPlanSha256: sha256(await readFile(output)),
      plan: output,
    });
    expect(result.written).toEqual(["mise.toml"]);
    expect(result.approvedPlanSha256).toBe(sha256(await readFile(output)));
    expect(result.sourceFiles).toEqual([
      {
        path: copyOnceSources["node-toolchain"],
        sha256: plan.operations[0]?.sourceSha256,
      },
    ]);
    expect(result.observation.endedAt).toMatch(/Z$/);
    expect(await readFile(join(context.root, "mise.toml"), "utf8")).toContain(
      'node = "24.18.1"',
    );
  });

  it("rejects repository drift after planning", async () => {
    const context = await fixture();
    const request = await writeRequest(context.external);
    const output = join(context.external, "plan.json");
    await createPlan({ ...context, output, request });
    await writeFile(join(context.root, "new-file.txt"), "drift\n");

    await expect(
      applyPlan({
        ...context,
        approvedPlanSha256: sha256(await readFile(output)),
        plan: output,
      }),
    ).rejects.toThrow("changed after planning");
    await expect(
      readFile(join(context.root, "mise.toml")),
    ).rejects.toMatchObject({
      code: "ENOENT",
    });
  });

  it("rejects an approved plan when current authority main advances", async () => {
    const context = await fixture();
    const request = await writeRequest(context.external);
    const output = join(context.external, "plan.json");
    await createPlan({ ...context, output, request });
    context.authorityClient.currentCommit = "d".repeat(40);

    await expect(
      applyPlan({
        ...context,
        approvedPlanSha256: sha256(await readFile(output)),
        plan: output,
      }),
    ).rejects.toThrow("changed after planning");
    await expect(
      readFile(join(context.root, "mise.toml")),
    ).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("rejects symlink traversal and in-repository plan files", async () => {
    const context = await fixture();
    const request = await writeRequest(context.external, "linked/mise.toml");
    const output = join(context.external, "plan.json");
    const target = join(context.external, "target");
    await mkdir(target);
    await symlink(target, join(context.root, "linked"));

    await expect(createPlan({ ...context, output, request })).rejects.toThrow(
      "symbolic link",
    );

    const internalRequest = join(context.root, "request.json");
    await writeFile(internalRequest, await readFile(request));
    await expect(
      createPlan({
        ...context,
        output: join(context.external, "second-plan.json"),
        request: internalRequest,
      }),
    ).rejects.toThrow("outside the target repository");
  });

  it("fails before writing when the exact source is unavailable", async () => {
    const context = await fixture();
    const request = await writeRequest(context.external);
    const output = join(context.external, "plan.json");
    const unavailable = new FakeAuthorityClient(commit, {});

    await expect(
      createPlan({ ...context, authorityClient: unavailable, output, request }),
    ).rejects.toThrow("Missing fake authority path");
    await expect(readFile(output)).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("rejects case-folded Git metadata destinations", async () => {
    const context = await fixture();
    const request = await writeRequest(context.external, ".GIT/config");

    await expect(
      createPlan({
        ...context,
        output: join(context.external, "plan.json"),
        request,
      }),
    ).rejects.toThrow("Unsafe repository-relative destination");
  });

  it("rejects case and Unicode-normalized destination aliases while planning", async () => {
    const caseContext = await fixture();
    const caseRequest = await writeAliasedRequest(caseContext.external, [
      "alias.yml",
      "ALIAS.yml",
    ]);
    await expect(
      createPlan({
        ...caseContext,
        output: join(caseContext.external, "case-plan.json"),
        request: caseRequest,
      }),
    ).rejects.toThrow("filesystem-aliased destination");

    const unicodeContext = await fixture();
    const composed = "caf\u00e9.yml";
    const decomposed = "cafe\u0301.yml";
    expect(composed).not.toBe(decomposed);
    expect(composed.normalize("NFC")).toBe(decomposed.normalize("NFC"));
    const unicodeRequest = await writeAliasedRequest(
      unicodeContext.external,
      [composed, decomposed],
      "unicode-aliased-request.json",
    );
    await expect(
      createPlan({
        ...unicodeContext,
        output: join(unicodeContext.external, "unicode-plan.json"),
        request: unicodeRequest,
      }),
    ).rejects.toThrow("filesystem-aliased destination");
  });

  it("rejects a reviewed plan containing filesystem-aliased operations", async () => {
    const context = await fixture();
    const request = await writeRequest(context.external, "alias.yml");
    const output = join(context.external, "plan.json");
    const plan = await createPlan({ ...context, output, request });
    const operation = plan.operations[0];
    expect(operation).toBeDefined();
    if (!operation) {
      throw new Error("Expected one planned operation.");
    }
    plan.operations.push({ ...operation, destination: "ALIAS.yml" });
    await writeFile(output, `${JSON.stringify(plan, null, 2)}\n`);

    await expect(
      applyPlan({
        ...context,
        approvedPlanSha256: sha256(await readFile(output)),
        plan: output,
      }),
    ).rejects.toThrow("filesystem-aliased destination");
    await expect(
      readFile(join(context.root, "alias.yml")),
    ).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("binds the approved plan bytes, dirty content, and branch", async () => {
    const context = await fixture();
    await writeFile(join(context.root, "dirty.txt"), "before\n");
    const request = await writeRequest(context.external);
    const contentPlan = join(context.external, "content-plan.json");
    await createPlan({ ...context, output: contentPlan, request });
    const approvedContentPlan = sha256(await readFile(contentPlan));

    await writeFile(join(context.root, "dirty.txt"), "after!\n");
    await expect(
      applyPlan({
        ...context,
        approvedPlanSha256: approvedContentPlan,
        plan: contentPlan,
      }),
    ).rejects.toThrow("complete worktree state changed");

    await writeFile(join(context.root, "dirty.txt"), "before\n");
    const branchPlan = join(context.external, "branch-plan.json");
    await createPlan({ ...context, output: branchPlan, request });
    const approvedBranchPlan = sha256(await readFile(branchPlan));
    await execFileAsync("git", ["-C", context.root, "switch", "-c", "dev"]);
    await expect(
      applyPlan({
        ...context,
        approvedPlanSha256: approvedBranchPlan,
        plan: branchPlan,
      }),
    ).rejects.toThrow("branch, HEAD");

    await execFileAsync("git", ["-C", context.root, "switch", "main"]);
    await writeFile(branchPlan, `${await readFile(branchPlan, "utf8")} \n`);
    await expect(
      applyPlan({
        ...context,
        approvedPlanSha256: approvedBranchPlan,
        plan: branchPlan,
      }),
    ).rejects.toThrow("explicitly approved digest");
  });
});
