import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { lstat, readFile, readlink, realpath } from "node:fs/promises";
import { dirname, join, relative, resolve, sep } from "node:path";

import { minimatch } from "minimatch";
import { parse as parseYaml } from "yaml";

import { sha256 } from "./hashing.js";
import { ProcessFailure, type ProcessRunner } from "./process.js";

export interface NativeSurface {
  id: string | null;
  lock: string | null;
  lockPresent: boolean;
  manifest: string;
  profile: string;
  root: string;
  source: "declared" | "observed";
}

export type RepositoryApplicability =
  "applicable" | "not-applicable" | "pending-classification";

export interface RepositorySnapshot {
  branch: string | null;
  dirty: boolean;
  head: string;
  origin: string | null;
  originSha256: string | null;
  worktreeSha256: string;
}

export interface RepositoryInventory {
  applicability: {
    classification: RepositoryApplicability;
    detail: string;
    source: "declared-native-roots" | "observed-manifests";
  };
  directJustCiWorkflows: string[];
  files: {
    justfile: string | null;
    miseLock: boolean;
    miseToml: boolean;
  };
  justRecipes: string[];
  nativeRootsDeclaration: string | null;
  nativeSurfaces: NativeSurface[];
  observationScope: {
    ignoredFilesIncluded: false;
    query: string;
    regularFilesOnly: true;
  };
  releaseUnitsFile: string | null;
  root: string;
  snapshot: RepositorySnapshot;
  workflowFiles: string[];
}

interface SurfaceDefinition {
  lock: string | null;
  manifest: string;
  profile: string;
}

const surfaceDefinitions: readonly SurfaceDefinition[] = [
  {
    lock: "pnpm-lock.yaml",
    manifest: "package.json",
    profile: "node-typescript",
  },
  { lock: "go.sum", manifest: "go.mod", profile: "go" },
  { lock: "uv.lock", manifest: "pyproject.toml", profile: "python" },
  { lock: "Cargo.lock", manifest: "Cargo.toml", profile: "rust" },
  { lock: "build.zig.zon", manifest: "build.zig", profile: "zig" },
  {
    lock: ".terraform.lock.hcl",
    manifest: "main.tf",
    profile: "infrastructure-hcl",
  },
  { lock: null, manifest: "Pulumi.yaml", profile: "infrastructure-pulumi" },
  { lock: null, manifest: "cdk.json", profile: "infrastructure-aws-cdk" },
];

const declaredProfiles = new Set([
  "node-typescript",
  "python",
  "go",
  "rust",
  "zig",
  "zig-toolchain",
  "infrastructure-aws-cdk",
  "infrastructure-terraform",
  "infrastructure-opentofu",
  "infrastructure-pulumi",
]);

interface DeclaredRoot {
  id: string;
  path: string;
  profiles: string[];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

async function isRegularFile(file: string): Promise<boolean> {
  try {
    return (await lstat(file)).isFile();
  } catch (error) {
    if (
      error instanceof Error &&
      "code" in error &&
      (error as NodeJS.ErrnoException).code === "ENOENT"
    ) {
      return false;
    }
    throw error;
  }
}

async function optionalRun(
  runner: ProcessRunner,
  command: string,
  args: readonly string[],
  cwd: string,
): Promise<string | null> {
  try {
    return (await runner.run(command, args, { cwd })).stdout.trim();
  } catch (error) {
    if (error instanceof ProcessFailure) {
      return null;
    }
    throw error;
  }
}

async function listRepositoryFiles(
  runner: ProcessRunner,
  root: string,
): Promise<string[]> {
  const { stdout } = await runner.run(
    "git",
    [
      "-C",
      root,
      "ls-files",
      "--cached",
      "--others",
      "--exclude-standard",
      "-z",
      "--",
    ],
    { cwd: root },
  );
  const files = stdout.split("\0").filter(Boolean).sort();
  for (const file of files) {
    if (file.startsWith("/") || file.split("/").includes("..")) {
      throw new Error(`Git returned an unsafe repository path: ${file}`);
    }
  }
  const regular: string[] = [];
  for (const file of files) {
    if (await isRegularFile(join(root, file))) {
      regular.push(file);
    }
  }
  return regular;
}

function parseDeclaredRoots(source: string): DeclaredRoot[] {
  const value: unknown = parseYaml(source);
  if (
    !isRecord(value) ||
    value.schemaVersion !== "golden-path-native-roots/v1" ||
    !Array.isArray(value.roots) ||
    value.roots.length === 0
  ) {
    throw new Error("native-root declaration has an invalid v1 document");
  }
  const roots: DeclaredRoot[] = [];
  const ids = new Set<string>();
  for (const entry of value.roots) {
    if (
      !isRecord(entry) ||
      typeof entry.id !== "string" ||
      !/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(entry.id) ||
      typeof entry.path !== "string" ||
      !/^(?:\.|[a-z0-9][a-z0-9._-]*(?:\/[a-z0-9][a-z0-9._-]*)*)$/.test(
        entry.path,
      ) ||
      !Array.isArray(entry.profiles)
    ) {
      throw new Error("native-root declaration contains an invalid root");
    }
    const rawProfiles: unknown[] = entry.profiles;
    const profiles = rawProfiles.filter(
      (profile): profile is string => typeof profile === "string",
    );
    if (
      profiles.length === 0 ||
      profiles.length !== rawProfiles.length ||
      !profiles.every((profile) => declaredProfiles.has(profile)) ||
      new Set(profiles).size !== profiles.length ||
      ids.has(entry.id)
    ) {
      throw new Error("native-root declaration contains an invalid root");
    }
    ids.add(entry.id);
    roots.push({
      id: entry.id,
      path: entry.path,
      profiles: [...profiles].sort(),
    });
  }
  return roots;
}

function inRoot(root: string, name: string): string {
  return root === "." ? name : `${root}/${name}`;
}

function declaredManifest(
  root: string,
  profile: string,
  files: ReadonlySet<string>,
): { lock: string | null; manifest: string } | null {
  const exact: Record<string, { lock: string | null; manifest: string }> = {
    "node-typescript": { lock: "pnpm-lock.yaml", manifest: "package.json" },
    python: { lock: "uv.lock", manifest: "pyproject.toml" },
    go: { lock: "go.sum", manifest: "go.mod" },
    rust: { lock: "Cargo.lock", manifest: "Cargo.toml" },
    zig: { lock: "build.zig.zon", manifest: "build.zig" },
    "zig-toolchain": { lock: "mise.lock", manifest: "mise.toml" },
    "infrastructure-aws-cdk": { lock: null, manifest: "cdk.json" },
    "infrastructure-pulumi": { lock: null, manifest: "Pulumi.yaml" },
  };
  const definition = exact[profile];
  if (definition) {
    const manifest = inRoot(root, definition.manifest);
    return files.has(manifest)
      ? {
          lock: definition.lock ? inRoot(root, definition.lock) : null,
          manifest,
        }
      : null;
  }
  if (
    profile === "infrastructure-terraform" ||
    profile === "infrastructure-opentofu"
  ) {
    const prefix = root === "." ? "" : `${root}/`;
    const manifest = [...files].find(
      (file) =>
        file.startsWith(prefix) &&
        !file.slice(prefix.length).includes("/") &&
        file.endsWith(".tf"),
    );
    return manifest
      ? { lock: inRoot(root, ".terraform.lock.hcl"), manifest }
      : null;
  }
  return null;
}

function isInsideRepositoryPath(parent: string, candidate: string): boolean {
  return (
    parent === "." || candidate === parent || candidate.startsWith(`${parent}/`)
  );
}

interface PnpmWorkspaceConfiguration {
  patterns: string[];
  sharedWorkspaceLockfile: boolean;
}

interface PnpmManifestOwnership {
  manifest: string;
  root: string;
}

interface PnpmWorkspaceAnalysis {
  dependencyRoots: Map<string, string>;
  manifestOwnership: Map<string, PnpmManifestOwnership>;
}

function parsePnpmWorkspaceConfiguration(
  source: string,
): PnpmWorkspaceConfiguration | null {
  let value: unknown;
  try {
    value = parseYaml(source);
  } catch {
    return null;
  }
  if (!isRecord(value) || !Array.isArray(value.packages)) {
    return null;
  }
  const patterns = value.packages.filter(
    (pattern): pattern is string =>
      typeof pattern === "string" && pattern.length > 0,
  );
  if (
    patterns.length !== value.packages.length ||
    patterns.length === 0 ||
    (value.sharedWorkspaceLockfile !== undefined &&
      typeof value.sharedWorkspaceLockfile !== "boolean")
  ) {
    return null;
  }
  return {
    patterns,
    sharedWorkspaceLockfile: value.sharedWorkspaceLockfile !== false,
  };
}

function matchesPnpmWorkspace(
  member: string,
  patterns: readonly string[],
): boolean {
  const included = patterns
    .filter((pattern) => !pattern.startsWith("!"))
    .some((pattern) => matchesPnpmPattern(member, pattern));
  return included && !isExcludedFromPnpmWorkspace(member, patterns);
}

function matchesPnpmPattern(member: string, pattern: string): boolean {
  return (
    minimatch(member, pattern, { nonegate: true }) ||
    (pattern.endsWith("/**") &&
      minimatch(member, pattern.slice(0, -3), { nonegate: true }))
  );
}

function isExcludedFromPnpmWorkspace(
  member: string,
  patterns: readonly string[],
): boolean {
  return patterns
    .filter((pattern) => pattern.startsWith("!") && pattern.length > 1)
    .some((pattern) => matchesPnpmPattern(member, pattern.slice(1)));
}

async function inspectPnpmWorkspaces(
  repositoryRoot: string,
  files: readonly string[],
): Promise<PnpmWorkspaceAnalysis> {
  const packageManifests = files.filter(
    (file) => file === "package.json" || file.endsWith("/package.json"),
  );
  const manifestRoots = packageManifests.map((manifest) => ({
    manifest,
    root: dirname(manifest) === "." ? "." : dirname(manifest),
  }));
  const workspaces = files
    .filter(
      (file) =>
        file === "pnpm-workspace.yaml" || file.endsWith("/pnpm-workspace.yaml"),
    )
    .map((workspace) => ({
      root: dirname(workspace) === "." ? "." : dirname(workspace),
      workspace,
    }))
    .sort((left, right) => right.root.length - left.root.length);
  const dependencyRoots = new Map<string, string>();
  const manifestOwnership = new Map<string, PnpmManifestOwnership>();
  for (const workspace of workspaces) {
    const rootManifest = inRoot(workspace.root, "package.json");
    const rootLock = inRoot(workspace.root, "pnpm-lock.yaml");
    if (!files.includes(rootLock)) {
      continue;
    }
    const configuration = parsePnpmWorkspaceConfiguration(
      await readFile(join(repositoryRoot, workspace.workspace), "utf8"),
    );
    if (!configuration || !configuration.sharedWorkspaceLockfile) {
      continue;
    }
    const members = manifestRoots
      .filter(
        (entry) =>
          entry.root !== workspace.root &&
          isInsideRepositoryPath(workspace.root, entry.root),
      )
      .filter((entry) => {
        const member =
          workspace.root === "."
            ? entry.root
            : entry.root.slice(workspace.root.length + 1);
        return matchesPnpmWorkspace(member, configuration.patterns);
      })
      .sort((left, right) => right.root.length - left.root.length);
    const workspaceManifest = files.includes(rootManifest)
      ? rootManifest
      : [...members].sort((left, right) =>
          left.manifest.localeCompare(right.manifest),
        )[0]?.manifest;
    if (!workspaceManifest) {
      continue;
    }
    if (!dependencyRoots.has(workspace.root)) {
      dependencyRoots.set(workspace.root, workspaceManifest);
    }
    if (files.includes(rootManifest) && !manifestOwnership.has(rootManifest)) {
      manifestOwnership.set(rootManifest, {
        manifest: workspaceManifest,
        root: workspace.root,
      });
    }
    for (const member of members) {
      const hasMemberLock = files.includes(
        inRoot(member.root, "pnpm-lock.yaml"),
      );
      const ownership: PnpmManifestOwnership = hasMemberLock
        ? { manifest: member.manifest, root: member.root }
        : { manifest: workspaceManifest, root: workspace.root };
      if (hasMemberLock && !dependencyRoots.has(member.root)) {
        dependencyRoots.set(member.root, member.manifest);
      }
      for (const entry of manifestRoots) {
        const relativeEntry =
          workspace.root === "."
            ? entry.root
            : entry.root.slice(workspace.root.length + 1);
        if (
          !manifestOwnership.has(entry.manifest) &&
          isInsideRepositoryPath(member.root, entry.root) &&
          !isExcludedFromPnpmWorkspace(relativeEntry, configuration.patterns)
        ) {
          manifestOwnership.set(entry.manifest, ownership);
        }
      }
    }
  }
  return { dependencyRoots, manifestOwnership };
}

async function discoverNativeSurfaces(
  repositoryRoot: string,
  files: readonly string[],
): Promise<NativeSurface[]> {
  const allFiles = new Set(files);
  const surfaces: NativeSurface[] = [];
  const pnpmWorkspaces = await inspectPnpmWorkspaces(repositoryRoot, files);
  const emittedPnpmWorkspaces = new Set<string>();
  for (const definition of surfaceDefinitions) {
    for (const manifest of files.filter(
      (file) =>
        file === definition.manifest ||
        file.endsWith(`/${definition.manifest}`),
    )) {
      const workspaceOwnership =
        definition.profile === "node-typescript"
          ? pnpmWorkspaces.manifestOwnership.get(manifest)
          : undefined;
      if (
        workspaceOwnership &&
        emittedPnpmWorkspaces.has(workspaceOwnership.root)
      ) {
        continue;
      }
      const root =
        workspaceOwnership?.root ??
        (dirname(manifest) === "." ? "." : dirname(manifest));
      if (workspaceOwnership) {
        emittedPnpmWorkspaces.add(workspaceOwnership.root);
      }
      const lock = definition.lock ? inRoot(root, definition.lock) : null;
      surfaces.push({
        id: null,
        lock,
        lockPresent: lock === null || allFiles.has(lock),
        manifest: workspaceOwnership?.manifest ?? manifest,
        profile: definition.profile,
        root,
        source: "observed",
      });
    }
  }
  for (const [root, manifest] of pnpmWorkspaces.dependencyRoots) {
    if (emittedPnpmWorkspaces.has(root)) {
      continue;
    }
    const lock = inRoot(root, "pnpm-lock.yaml");
    surfaces.push({
      id: null,
      lock,
      lockPresent: allFiles.has(lock),
      manifest,
      profile: "node-typescript",
      root,
      source: "observed",
    });
  }
  return surfaces.sort((left, right) =>
    `${left.root}:${left.profile}`.localeCompare(
      `${right.root}:${right.profile}`,
    ),
  );
}

function isInside(root: string, candidate: string): boolean {
  const relation = relative(root, candidate);
  return (
    relation === "" || (!relation.startsWith(`..${sep}`) && relation !== "..")
  );
}

function redactOrigin(origin: string | null): string | null {
  if (origin === null) {
    return null;
  }
  try {
    const parsed = new URL(origin);
    parsed.username = "";
    parsed.password = "";
    parsed.search = "";
    parsed.hash = "";
    return parsed.toString();
  } catch {
    const scp = origin.match(/^(?:[^@/:]+@)?([^:]+):(.+)$/);
    if (scp?.[1] && scp[2]) {
      return `${scp[1]}:${scp[2].split(/[?#]/, 1)[0]}`;
    }
    if (origin.startsWith("/") || origin.startsWith("./")) {
      return origin;
    }
    return "<configured>";
  }
}

async function hashRegularFile(file: string): Promise<string> {
  const hash = createHash("sha256");
  await new Promise<void>((resolvePromise, reject) => {
    const stream = createReadStream(file);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("error", reject);
    stream.on("end", resolvePromise);
  });
  return `sha256:${hash.digest("hex")}`;
}

async function hashUntrackedFiles(
  runner: ProcessRunner,
  root: string,
): Promise<string> {
  const { stdout } = await runner.run(
    "git",
    ["-C", root, "ls-files", "--others", "--exclude-standard", "-z"],
    { cwd: root },
  );
  const names = stdout.split("\0").filter(Boolean).sort();
  const hash = createHash("sha256");
  for (const name of names) {
    const file = resolve(root, name);
    if (!isInside(root, file)) {
      throw new Error(`Git returned an unsafe untracked path: ${name}`);
    }
    const before = await lstat(file);
    let contentHash: string;
    if (before.isSymbolicLink()) {
      contentHash = sha256(await readlink(file));
    } else if (before.isFile()) {
      contentHash = await hashRegularFile(file);
    } else {
      throw new Error(`Unsupported untracked worktree entry: ${name}`);
    }
    const after = await lstat(file);
    if (
      before.dev !== after.dev ||
      before.ino !== after.ino ||
      before.mode !== after.mode ||
      before.size !== after.size ||
      before.mtimeMs !== after.mtimeMs
    ) {
      throw new Error(`Untracked file changed while hashing: ${name}`);
    }
    hash.update(`${name}\0${before.mode}\0${contentHash}\0`);
  }
  return `sha256:${hash.digest("hex")}`;
}

function workflowInvokesJustCi(value: unknown): boolean {
  if (!isRecord(value) || !isRecord(value.jobs)) {
    return false;
  }
  for (const job of Object.values(value.jobs)) {
    if (!isRecord(job) || !Array.isArray(job.steps)) {
      continue;
    }
    for (const step of job.steps) {
      if (!isRecord(step) || typeof step.run !== "string") {
        continue;
      }
      if (/(^|\n)\s*(?:mise exec --\s+)?just ci(?:\s|$)/m.test(step.run)) {
        return true;
      }
    }
  }
  return false;
}

async function inspectWorkflows(
  root: string,
  repositoryFiles: readonly string[],
): Promise<{
  directJustCiWorkflows: string[];
  workflowFiles: string[];
}> {
  const workflowFiles = repositoryFiles.filter((file) =>
    /^\.github\/workflows\/[^/]+\.ya?ml$/i.test(file),
  );
  const directJustCiWorkflows: string[] = [];
  for (const workflow of workflowFiles) {
    const source = await readFile(join(root, workflow), "utf8");
    let parsed: unknown;
    try {
      parsed = parseYaml(source);
    } catch {
      continue;
    }
    if (workflowInvokesJustCi(parsed)) {
      directJustCiWorkflows.push(workflow);
    }
  }
  return { directJustCiWorkflows, workflowFiles };
}

export async function resolveRepositoryRoot(
  runner: ProcessRunner,
  candidate: string,
): Promise<string> {
  const requested = resolve(candidate);
  const { stdout } = await runner.run(
    "git",
    ["-C", requested, "rev-parse", "--show-toplevel"],
    { cwd: requested },
  );
  return realpath(stdout.trim());
}

export async function readRepositorySnapshot(
  runner: ProcessRunner,
  root: string,
): Promise<RepositorySnapshot> {
  const [
    { stdout: headOutput },
    { stdout: statusOutput },
    { stdout: diffOutput },
    branch,
    rawOrigin,
    untrackedSha256,
  ] = await Promise.all([
    runner.run("git", ["-C", root, "rev-parse", "HEAD"], { cwd: root }),
    runner.run(
      "git",
      ["-C", root, "status", "--porcelain=v1", "-z", "--untracked-files=all"],
      { cwd: root },
    ),
    runner.run(
      "git",
      [
        "-C",
        root,
        "diff",
        "--binary",
        "--no-ext-diff",
        "--no-textconv",
        "HEAD",
        "--",
      ],
      { cwd: root },
    ),
    optionalRun(
      runner,
      "git",
      ["-C", root, "symbolic-ref", "--short", "-q", "HEAD"],
      root,
    ),
    optionalRun(
      runner,
      "git",
      ["-C", root, "remote", "get-url", "origin"],
      root,
    ),
    hashUntrackedFiles(runner, root),
  ]);
  const head = headOutput.trim();
  if (!/^[a-f0-9]{40}$/.test(head)) {
    throw new Error(`Repository returned an invalid HEAD: ${head}`);
  }
  const worktreeHash = createHash("sha256");
  worktreeHash.update(`status\0${statusOutput}\0`);
  worktreeHash.update(`diff\0${diffOutput}\0`);
  worktreeHash.update(`untracked\0${untrackedSha256}\0`);
  return {
    branch,
    dirty: statusOutput.length > 0,
    head,
    origin: redactOrigin(rawOrigin),
    originSha256: rawOrigin === null ? null : sha256(rawOrigin),
    worktreeSha256: `sha256:${worktreeHash.digest("hex")}`,
  };
}

export async function inspectRepository(
  runner: ProcessRunner,
  candidate: string,
): Promise<RepositoryInventory> {
  const root = await resolveRepositoryRoot(runner, candidate);
  const [snapshot, repositoryFiles] = await Promise.all([
    readRepositorySnapshot(runner, root),
    listRepositoryFiles(runner, root),
  ]);
  const repositoryFileSet = new Set(repositoryFiles);
  const justfile = (await isRegularFile(join(root, "justfile")))
    ? "justfile"
    : (await isRegularFile(join(root, "Justfile")))
      ? "Justfile"
      : null;

  const nativeRootsDeclaration = repositoryFileSet.has(
    ".github/golden-path-native-roots.yaml",
  )
    ? ".github/golden-path-native-roots.yaml"
    : null;
  let nativeSurfaces = await discoverNativeSurfaces(root, repositoryFiles);
  let applicability: RepositoryInventory["applicability"];
  if (nativeRootsDeclaration) {
    try {
      const declarations = parseDeclaredRoots(
        await readFile(join(root, nativeRootsDeclaration), "utf8"),
      );
      const missing: string[] = [];
      nativeSurfaces = declarations.flatMap((declaration) =>
        declaration.profiles.map((profile) => {
          const observed = declaredManifest(
            declaration.path,
            profile,
            repositoryFileSet,
          );
          if (!observed) {
            missing.push(`${declaration.id}:${profile}`);
          }
          const lock = observed?.lock ?? null;
          return {
            id: declaration.id,
            lock,
            lockPresent: lock === null || repositoryFileSet.has(lock),
            manifest:
              observed?.manifest ?? inRoot(declaration.path, "<not-observed>"),
            profile,
            root: declaration.path,
            source: "declared" as const,
          };
        }),
      );
      applicability = missing.length
        ? {
            classification: "pending-classification",
            detail: `Declared native profiles lack an observed manifest: ${missing.join(", ")}.`,
            source: "declared-native-roots",
          }
        : {
            classification: "applicable",
            detail:
              "Repository-owned native-root declarations match observed manifests.",
            source: "declared-native-roots",
          };
    } catch (error) {
      applicability = {
        classification: "pending-classification",
        detail: error instanceof Error ? error.message : String(error),
        source: "declared-native-roots",
      };
    }
  } else if (nativeSurfaces.length === 0) {
    applicability = {
      classification: "not-applicable",
      detail:
        "No supported native manifest was observed in the bounded repository file inventory.",
      source: "observed-manifests",
    };
  } else {
    const rootsByProfile = new Map<string, Set<string>>();
    for (const surface of nativeSurfaces) {
      const roots = rootsByProfile.get(surface.profile) ?? new Set<string>();
      roots.add(surface.root);
      rootsByProfile.set(surface.profile, roots);
    }
    const ambiguousProfiles = [...rootsByProfile]
      .filter(([, roots]) => roots.size > 1)
      .map(([profile]) => profile);
    if (rootsByProfile.has("infrastructure-hcl")) {
      ambiguousProfiles.push("infrastructure-hcl");
    }
    applicability = ambiguousProfiles.length
      ? {
          classification: "pending-classification",
          detail: `Multiple roots were observed for ${ambiguousProfiles.join(", ")}; add a repository-owned native-root declaration before making adoption claims.`,
          source: "observed-manifests",
        }
      : {
          classification: "applicable",
          detail:
            "Supported native manifests identify unambiguous repository-owned roots.",
          source: "observed-manifests",
        };
  }

  let justRecipes: string[] = [];
  if (justfile) {
    const summary = await optionalRun(
      runner,
      "just",
      [
        "--justfile",
        join(root, justfile),
        "--working-directory",
        root,
        "--summary",
      ],
      root,
    );
    if (summary) {
      justRecipes = summary.split(/\s+/).filter(Boolean).sort();
    }
  }

  const workflows = await inspectWorkflows(root, repositoryFiles);
  return {
    ...workflows,
    applicability,
    files: {
      justfile,
      miseLock: await isRegularFile(join(root, "mise.lock")),
      miseToml: await isRegularFile(join(root, "mise.toml")),
    },
    justRecipes,
    nativeRootsDeclaration,
    nativeSurfaces,
    observationScope: {
      ignoredFilesIncluded: false,
      query: "git ls-files --cached --others --exclude-standard -z --",
      regularFilesOnly: true,
    },
    releaseUnitsFile: repositoryFileSet.has(".github/release-units.json")
      ? ".github/release-units.json"
      : null,
    root: ".",
    snapshot,
  };
}
