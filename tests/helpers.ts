import { execFile } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

import type { AuthorityClient } from "../src/authority.js";

const execFileAsync = promisify(execFile);

export class FakeAuthorityClient implements AuthorityClient {
  readonly reads: string[] = [];
  currentCommit: string;

  constructor(
    readonly commit: string,
    readonly files: Readonly<Record<string, string>>,
  ) {
    this.currentCommit = commit;
  }

  resolveCommit(): Promise<string> {
    return Promise.resolve(this.currentCommit);
  }

  readText(path: string, commit: string): Promise<string> {
    this.reads.push(`${commit}:${path}`);
    if (commit !== this.commit) {
      return Promise.reject(
        new Error(`Unexpected authority commit: ${commit}`),
      );
    }
    const content = this.files[path];
    if (content === undefined) {
      return Promise.reject(new Error(`Missing fake authority path: ${path}`));
    }
    return Promise.resolve(content);
  }
}

export async function temporaryDirectory(prefix: string): Promise<string> {
  return mkdtemp(join(tmpdir(), prefix));
}

export async function createGitRepository(): Promise<string> {
  const root = await temporaryDirectory("golden-path-agent-repository-");
  await execFileAsync("git", ["init", "-b", "main", root]);
  await execFileAsync("git", [
    "-C",
    root,
    "config",
    "user.name",
    "Golden Path Test",
  ]);
  await execFileAsync("git", [
    "-C",
    root,
    "config",
    "user.email",
    "test@example.invalid",
  ]);
  await writeFile(join(root, "README.md"), "fixture\n");
  await execFileAsync("git", ["-C", root, "add", "README.md"]);
  await execFileAsync("git", [
    "-C",
    root,
    "commit",
    "-m",
    "test: initialize fixture",
  ]);
  return root;
}

export async function removeDirectory(directory: string): Promise<void> {
  await rm(directory, { force: true, recursive: true });
}
