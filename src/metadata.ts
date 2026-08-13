import { createHash } from "node:crypto";
import { lstat, readFile, readdir } from "node:fs/promises";
import { basename, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

import { packageName } from "./constants.js";

export interface PackageMetadata {
  contentScope: "development-source" | "installed-package";
  contentSha256: string;
  manifestSha256: string;
  name: typeof packageName;
  version: string;
}

let metadataPromise: Promise<PackageMetadata> | undefined;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

async function collectFiles(root: string, entry: string): Promise<string[]> {
  const absolute = join(root, entry);
  const stats = await lstat(absolute);
  if (stats.isSymbolicLink()) {
    throw new Error(`Package content must not contain symlinks: ${entry}`);
  }
  if (stats.isFile()) {
    return [entry];
  }
  if (!stats.isDirectory()) {
    throw new Error(`Unsupported package content entry: ${entry}`);
  }
  const files: string[] = [];
  for (const child of (await readdir(absolute)).sort()) {
    files.push(...(await collectFiles(root, join(entry, child))));
  }
  return files;
}

async function contentDigest(
  root: string,
  entries: readonly string[],
): Promise<string> {
  const files = (
    await Promise.all(entries.map((entry) => collectFiles(root, entry)))
  )
    .flat()
    .sort();
  const hash = createHash("sha256");
  for (const file of files) {
    const portable = relative(root, resolve(root, file)).split(sep).join("/");
    hash.update(`file:${portable}\0`);
    hash.update(await readFile(join(root, file)));
    hash.update("\0");
  }
  return `sha256:${hash.digest("hex")}`;
}

export function getPackageMetadata(): Promise<PackageMetadata> {
  metadataPromise ??= (async () => {
    const moduleDirectory = fileURLToPath(new URL("./", import.meta.url));
    const packageRoot = resolve(moduleDirectory, "..");
    const source = await readFile(join(packageRoot, "package.json"), "utf8");
    const value: unknown = JSON.parse(source);
    if (
      !isRecord(value) ||
      value.name !== packageName ||
      typeof value.version !== "string" ||
      !/^\d+\.\d+\.\d+$/.test(value.version) ||
      !Array.isArray(value.files) ||
      !value.files.every((entry) => typeof entry === "string")
    ) {
      throw new Error("Invalid golden-path-agent package metadata.");
    }
    const packageFiles: string[] = value.files;
    const installed = basename(moduleDirectory) === "lib";
    const entries = installed
      ? ["package.json", ...packageFiles]
      : ["package.json", "README.md", "src", "skills/golden-path"];
    return {
      contentScope: installed ? "installed-package" : "development-source",
      contentSha256: await contentDigest(packageRoot, entries),
      manifestSha256: `sha256:${createHash("sha256").update(source).digest("hex")}`,
      name: packageName,
      version: value.version,
    };
  })();
  return metadataPromise;
}
