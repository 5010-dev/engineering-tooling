import { describe, expect, it } from "vitest";

import {
  GitHubAuthorityClient,
  readAuthoritySnapshot,
} from "../src/authority.js";
import { authorityDocumentPaths } from "../src/constants.js";
import type { ProcessRunner, RunResult } from "../src/process.js";
import { FakeAuthorityClient } from "./helpers.js";

const commit = "a".repeat(40);

function authorityFiles(): Record<string, string> {
  return Object.fromEntries(
    authorityDocumentPaths.map((path) => [path, `# ${path}\n`]),
  );
}

describe("authority snapshot", () => {
  it("pins all authority reads to github.com", async () => {
    const calls: string[][] = [];
    const runner: ProcessRunner = {
      run(_command: string, args: readonly string[]): Promise<RunResult> {
        calls.push([...args]);
        return Promise.resolve({
          stderr: "",
          stdout: args.includes("--jq") ? `${commit}\n` : "authority\n",
        });
      },
    };
    const client = new GitHubAuthorityClient(runner);

    expect(await client.resolveCommit()).toBe(commit);
    expect(await client.readText(authorityDocumentPaths[0], commit)).toBe(
      "authority\n",
    );
    expect(calls).toHaveLength(2);
    for (const args of calls) {
      expect(args.slice(0, 4)).toEqual([
        "api",
        "--hostname",
        "github.com",
        expect.any(String),
      ]);
    }
  });

  it("derives identity from exact current source instead of a packaged snapshot", async () => {
    const files = authorityFiles();
    files["docs/standards/developer-tooling/README.md"] =
      `# Developer Tooling Standard

- Standard version: \`2026.08.8\`
- Contract version: \`golden-path/v1\`

Active central executable tooling: **none**.
`;
    const snapshot = await readAuthoritySnapshot(
      new FakeAuthorityClient(commit, files),
    );

    expect(snapshot.commit).toBe(commit);
    expect(snapshot.standardVersion).toBe("2026.08.8");
    expect(snapshot.contractVersion).toBe("golden-path/v1");
    expect(snapshot.activeCentralTooling).toBe("none");
    expect(snapshot.files).toHaveLength(authorityDocumentPaths.length);
    expect(
      snapshot.files.every((file) => file.sha256.startsWith("sha256:")),
    ).toBe(true);
  });

  it("fails closed when the normative identity cannot be read", async () => {
    const files = authorityFiles();
    files["docs/standards/developer-tooling/README.md"] =
      "# missing metadata\n";

    await expect(
      readAuthoritySnapshot(new FakeAuthorityClient(commit, files)),
    ).rejects.toThrow("does not declare");
  });
});
