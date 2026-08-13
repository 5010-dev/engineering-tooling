import {
  allowedAuthorityPaths,
  authorityDocumentPaths,
  authorityRef,
  authorityRepository,
} from "./constants.js";
import { sha256 } from "./hashing.js";
import type { ProcessRunner } from "./process.js";

export interface AuthorityFile {
  path: string;
  sha256: string;
}

export interface AuthoritySnapshot {
  activeCentralTooling: string;
  commit: string;
  contractVersion: string;
  files: AuthorityFile[];
  ref: typeof authorityRef;
  repository: typeof authorityRepository;
  standardVersion: string;
}

export interface AuthorityClient {
  readText(path: string, commit: string): Promise<string>;
  resolveCommit(): Promise<string>;
}

export async function readAuthorityFileManifest(
  client: AuthorityClient,
  commit: string,
  paths: readonly string[],
): Promise<AuthorityFile[]> {
  const selectedPaths = [...new Set(paths)].sort();
  return Promise.all(
    selectedPaths.map(async (path) => ({
      path,
      sha256: sha256(await client.readText(path, commit)),
    })),
  );
}

function matchRequired(source: string, pattern: RegExp, label: string): string {
  const match = source.match(pattern);
  const value = match?.[1]?.trim();
  if (!value) {
    throw new Error(`Current authority source does not declare ${label}.`);
  }
  return value.replaceAll("`", "").replaceAll("*", "");
}

export class GitHubAuthorityClient implements AuthorityClient {
  constructor(private readonly runner: ProcessRunner) {}

  async resolveCommit(): Promise<string> {
    const { stdout } = await this.runner.run("gh", [
      "api",
      "--hostname",
      "github.com",
      `repos/${authorityRepository}/commits/${authorityRef}`,
      "--jq",
      ".sha",
    ]);
    const commit = stdout.trim();
    if (!/^[a-f0-9]{40}$/.test(commit)) {
      throw new Error(`GitHub returned an invalid authority commit: ${commit}`);
    }
    return commit;
  }

  async readText(path: string, commit: string): Promise<string> {
    if (!allowedAuthorityPaths.has(path)) {
      throw new Error(`Authority path is not allowlisted: ${path}`);
    }
    if (!/^[a-f0-9]{40}$/.test(commit)) {
      throw new Error(`Invalid authority commit: ${commit}`);
    }
    const endpoint = `repos/${authorityRepository}/contents/${path}?ref=${commit}`;
    const { stdout } = await this.runner.run("gh", [
      "api",
      "--hostname",
      "github.com",
      "-H",
      "Accept: application/vnd.github.raw+json",
      endpoint,
    ]);
    if (Buffer.byteLength(stdout, "utf8") > 1024 * 1024) {
      throw new Error(
        `Authority file exceeds the 1 MiB support boundary: ${path}`,
      );
    }
    return stdout;
  }
}

export async function readAuthoritySnapshot(
  client: AuthorityClient,
): Promise<AuthoritySnapshot> {
  const commit = await client.resolveCommit();
  const contents = await Promise.all(
    authorityDocumentPaths.map(async (path) => ({
      content: await client.readText(path, commit),
      path,
    })),
  );
  const standard = contents.find(
    (entry) => entry.path === "docs/standards/developer-tooling/README.md",
  )?.content;
  if (!standard) {
    throw new Error("Developer Tooling Standard was not resolved.");
  }

  return {
    activeCentralTooling: matchRequired(
      standard,
      /Active central executable tooling:\s*([^\n]+)/i,
      "the active central tooling identity",
    ).replace(/\.$/, ""),
    commit,
    contractVersion: matchRequired(
      standard,
      /Contract version:\s*([^\n]+)/i,
      "the contract version",
    ),
    files: contents.map(({ content, path }) => ({
      path,
      sha256: sha256(content),
    })),
    ref: authorityRef,
    repository: authorityRepository,
    standardVersion: matchRequired(
      standard,
      /Standard version:\s*([^\n]+)/i,
      "the standard version",
    ),
  };
}
