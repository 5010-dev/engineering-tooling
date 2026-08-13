import { readFile } from "node:fs/promises";

import { describe, expect, it } from "vitest";

describe("repository-owned pnpm bootstrap", () => {
  it("never recursively removes a caller-provided cache root", async () => {
    const source = await readFile(
      new URL("../scripts/pnpm", import.meta.url),
      "utf8",
    );

    expect(source).toContain(
      'cache_directory="$cache_parent/golden-path-agent/pnpm/$version"',
    );
    expect(source).not.toContain('rm -rf "$cache_directory"');
    expect(source).toContain(
      'mv "$staging/unpacked/package" "$cache_directory/package"',
    );
    expect(source).toContain("pnpm cache entry is incomplete or modified");
  });
});
