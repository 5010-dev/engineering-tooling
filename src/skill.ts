import { createHash, randomUUID } from "node:crypto";
import {
  cp,
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rename,
  rm,
  writeFile,
} from "node:fs/promises";
import { homedir } from "node:os";
import { basename, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

import { packageName, skillMarkerName, skillName } from "./constants.js";
import { getPackageMetadata } from "./metadata.js";

export type AgentHost = "claude-code" | "codex";
export type SkillCheckStatus =
  "current" | "missing" | "modified" | "stale" | "unmanaged";
export type SkillInstallStatus = "installed" | "unchanged" | "updated";

export interface AgentSkillOptions {
  homeDirectory?: string;
  host: AgentHost;
}

export interface InstallAgentSkillOptions extends AgentSkillOptions {
  force?: boolean;
}

export interface SkillCheckResult {
  contentSha256: string | null;
  destination: string;
  detail: string;
  host: AgentHost;
  packageVersion: string | null;
  status: SkillCheckStatus;
}

export interface SkillInstallResult {
  destination: string;
  host: AgentHost;
  status: SkillInstallStatus;
}

interface OwnershipMarker {
  contentHash: string;
  host: AgentHost;
  owner: typeof packageName;
  packageVersion: string;
  schemaVersion: 1;
  skill: typeof skillName;
}

interface MarkerReadResult {
  marker?: OwnershipMarker;
  reason?: string;
}

function hostSkillRoot(host: AgentHost, homeDirectory: string): string {
  return host === "codex"
    ? join(homeDirectory, ".agents", "skills")
    : join(homeDirectory, ".claude", "skills");
}

export function resolveAgentSkillDestination(
  options: AgentSkillOptions,
): string {
  const homeDirectory = resolve(options.homeDirectory ?? homedir());
  return join(hostSkillRoot(options.host, homeDirectory), skillName);
}

function isMissing(error: unknown): boolean {
  return (
    error instanceof Error &&
    "code" in error &&
    (error as NodeJS.ErrnoException).code === "ENOENT"
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isAgentHost(value: unknown): value is AgentHost {
  return value === "claude-code" || value === "codex";
}

async function packagedSkillDirectory(): Promise<string> {
  const candidates = [
    new URL("./skills/golden-path/", import.meta.url),
    new URL("../skills/golden-path/", import.meta.url),
  ];
  for (const candidate of candidates) {
    const directory = fileURLToPath(candidate);
    try {
      const directoryStats = await lstat(directory);
      if (directoryStats.isDirectory()) {
        return directory;
      }
    } catch (error) {
      if (!isMissing(error)) {
        throw error;
      }
    }
  }
  throw new Error("Packaged golden-path Skill content is missing.");
}

function transformSkillForHost(source: string, host: AgentHost): string {
  if (host === "codex") {
    return source;
  }
  if (!source.startsWith("---\n")) {
    throw new Error("Packaged SKILL.md has invalid frontmatter.");
  }
  if (/^disable-model-invocation:/m.test(source)) {
    throw new Error(
      "Packaged SKILL.md unexpectedly contains host-specific metadata.",
    );
  }
  const closing = source.indexOf("\n---\n", 4);
  if (closing < 0) {
    throw new Error("Packaged SKILL.md has unterminated frontmatter.");
  }
  return `${source.slice(0, closing)}\ndisable-model-invocation: true${source.slice(closing)}`;
}

async function readOwnershipMarker(
  destination: string,
): Promise<MarkerReadResult> {
  let source: string;
  try {
    source = await readFile(join(destination, skillMarkerName), "utf8");
  } catch (error) {
    if (isMissing(error)) {
      return { reason: `missing ${skillMarkerName}` };
    }
    throw error;
  }
  let value: unknown;
  try {
    value = JSON.parse(source) as unknown;
  } catch {
    return { reason: `invalid ${skillMarkerName}` };
  }
  if (
    !isRecord(value) ||
    value.schemaVersion !== 1 ||
    value.owner !== packageName ||
    value.skill !== skillName ||
    !isAgentHost(value.host) ||
    typeof value.packageVersion !== "string" ||
    typeof value.contentHash !== "string"
  ) {
    return { reason: `invalid ${skillMarkerName}` };
  }
  return { marker: value as unknown as OwnershipMarker };
}

async function hashDirectory(
  directory: string,
  transformHost?: AgentHost,
): Promise<string> {
  const root = resolve(directory);
  const hash = createHash("sha256");

  async function visit(current: string): Promise<void> {
    const entries = await readdir(current, { withFileTypes: true });
    entries.sort((left, right) => left.name.localeCompare(right.name));
    for (const entry of entries) {
      if (entry.name === skillMarkerName) {
        continue;
      }
      const file = join(current, entry.name);
      if (entry.isDirectory()) {
        await visit(file);
        continue;
      }
      if (!entry.isFile()) {
        throw new Error(
          `Skill content must contain only files and directories: ${file}`,
        );
      }
      const portablePath = relative(root, file).split(sep).join("/");
      let content: string | Buffer = await readFile(file);
      if (transformHost && portablePath === "SKILL.md") {
        content = transformSkillForHost(
          content.toString("utf8"),
          transformHost,
        );
      }
      hash.update(`file:${portablePath}\0`);
      hash.update(content);
      hash.update("\0");
    }
  }

  await visit(root);
  return `sha256:${hash.digest("hex")}`;
}

async function assertDirectory(directory: string): Promise<boolean> {
  try {
    const directoryStats = await lstat(directory);
    if (!directoryStats.isDirectory() || directoryStats.isSymbolicLink()) {
      throw new Error(
        `Skill destination is not a managed directory: ${directory}`,
      );
    }
    return true;
  } catch (error) {
    if (isMissing(error)) {
      return false;
    }
    throw error;
  }
}

async function assertSafeSkillRoot(
  options: AgentSkillOptions,
  create: boolean,
): Promise<string> {
  const homeDirectory = resolve(options.homeDirectory ?? homedir());
  const components =
    options.host === "codex" ? [".agents", "skills"] : [".claude", "skills"];
  let current = homeDirectory;
  for (const component of ["", ...components]) {
    if (component) {
      current = join(current, component);
    }
    try {
      const stats = await lstat(current);
      if (!stats.isDirectory() || stats.isSymbolicLink()) {
        throw new Error(`Skill parent is not a safe directory: ${current}`);
      }
    } catch (error) {
      if (!isMissing(error)) {
        throw error;
      }
      if (!create) {
        return hostSkillRoot(options.host, homeDirectory);
      }
      await mkdir(current);
      const created = await lstat(current);
      if (!created.isDirectory() || created.isSymbolicLink()) {
        throw new Error(`Skill parent is not a safe directory: ${current}`, {
          cause: error,
        });
      }
    }
  }
  return current;
}

export async function checkAgentSkill(
  options: AgentSkillOptions,
): Promise<SkillCheckResult> {
  const destination = resolveAgentSkillDestination(options);
  await assertSafeSkillRoot(options, false);
  if (!(await assertDirectory(destination))) {
    return {
      contentSha256: null,
      destination,
      detail: "skill is not installed",
      host: options.host,
      packageVersion: null,
      status: "missing",
    };
  }
  const metadata = await getPackageMetadata();
  const markerResult = await readOwnershipMarker(destination);
  if (!markerResult.marker || markerResult.marker.owner !== metadata.name) {
    return {
      contentSha256: null,
      destination,
      detail: markerResult.reason ?? "owned by another installer",
      host: options.host,
      packageVersion: null,
      status: "unmanaged",
    };
  }
  const installedHash = await hashDirectory(destination);
  if (installedHash !== markerResult.marker.contentHash) {
    return {
      contentSha256: installedHash,
      destination,
      detail: "managed skill content has local modifications",
      host: options.host,
      packageVersion: markerResult.marker.packageVersion,
      status: "modified",
    };
  }
  const sourceHash = await hashDirectory(
    await packagedSkillDirectory(),
    options.host,
  );
  if (
    markerResult.marker.host !== options.host ||
    markerResult.marker.packageVersion !== metadata.version ||
    markerResult.marker.contentHash !== sourceHash
  ) {
    return {
      contentSha256: installedHash,
      destination,
      detail: `installed skill does not match ${metadata.name}@${metadata.version}`,
      host: options.host,
      packageVersion: markerResult.marker.packageVersion,
      status: "stale",
    };
  }
  return {
    contentSha256: installedHash,
    destination,
    detail: `matches ${metadata.name}@${metadata.version}`,
    host: options.host,
    packageVersion: markerResult.marker.packageVersion,
    status: "current",
  };
}

async function replaceDirectory(
  stagingDirectory: string,
  destination: string,
  destinationExists: boolean,
): Promise<void> {
  if (!destinationExists) {
    await rename(stagingDirectory, destination);
    return;
  }
  const backup = join(
    resolve(destination, ".."),
    `.${basename(destination)}-backup-${randomUUID()}`,
  );
  await rename(destination, backup);
  try {
    await rename(stagingDirectory, destination);
  } catch (error) {
    await rename(backup, destination);
    throw error;
  }
  await rm(backup, { force: true, recursive: true });
}

export async function installAgentSkill(
  options: InstallAgentSkillOptions,
): Promise<SkillInstallResult> {
  const destination = resolveAgentSkillDestination(options);
  const skillRoot = await assertSafeSkillRoot(options, true);
  const destinationExists = await assertDirectory(destination);
  const previousCheck = destinationExists
    ? await checkAgentSkill(options)
    : undefined;
  if (previousCheck?.status === "current") {
    return { destination, host: options.host, status: "unchanged" };
  }
  if (previousCheck?.status === "unmanaged") {
    throw new Error(
      `Refusing to overwrite unmanaged skill at ${destination}: ${previousCheck.detail}.`,
    );
  }
  if (previousCheck?.status === "modified" && !options.force) {
    throw new Error(
      `Refusing to overwrite locally modified skill at ${destination}; rerun with --force to replace managed changes.`,
    );
  }

  const metadata = await getPackageMetadata();
  const sourceDirectory = await packagedSkillDirectory();
  const contentHash = await hashDirectory(sourceDirectory, options.host);
  const stagingDirectory = await mkdtemp(
    join(skillRoot, `.${skillName}-install-`),
  );
  try {
    await cp(sourceDirectory, stagingDirectory, {
      force: false,
      recursive: true,
    });
    if (options.host === "claude-code") {
      const skillFile = join(stagingDirectory, "SKILL.md");
      const source = await readFile(skillFile, "utf8");
      await writeFile(skillFile, transformSkillForHost(source, options.host));
    }
    const marker: OwnershipMarker = {
      contentHash,
      host: options.host,
      owner: packageName,
      packageVersion: metadata.version,
      schemaVersion: 1,
      skill: skillName,
    };
    await writeFile(
      join(stagingDirectory, skillMarkerName),
      `${JSON.stringify(marker, null, 2)}\n`,
      { flag: "wx" },
    );
    await replaceDirectory(stagingDirectory, destination, destinationExists);
  } finally {
    await rm(stagingDirectory, { force: true, recursive: true });
  }
  return {
    destination,
    host: options.host,
    status: destinationExists ? "updated" : "installed",
  };
}
