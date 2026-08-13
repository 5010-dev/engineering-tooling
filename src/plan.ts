import { randomUUID } from "node:crypto";
import {
  lstat,
  readFile,
  realpath,
  rename,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import {
  basename,
  dirname,
  isAbsolute,
  join,
  relative,
  resolve,
  sep,
} from "node:path";

import { z } from "zod";

import type { AuthorityClient } from "./authority.js";
import {
  authorityRef,
  authorityRepository,
  copyOnceSources,
  packageName,
} from "./constants.js";
import { sha256 } from "./hashing.js";
import { getPackageMetadata } from "./metadata.js";
import type { ProcessRunner } from "./process.js";
import {
  readRepositorySnapshot,
  resolveRepositoryRoot,
  type RepositorySnapshot,
} from "./repository.js";

const digestSchema = z.string().regex(/^sha256:[a-f0-9]{64}$/);
const commitSchema = z.string().regex(/^[a-f0-9]{40}$/);
const sourceNameSchema = z.enum([
  "canonical-ci",
  "dependabot",
  "go-just",
  "native-roots",
  "node-just",
  "node-toolchain",
  "python-just",
]);

const replacementSchema = z
  .object({
    expectedOccurrences: z.number().int().positive().max(100),
    match: z.string().min(1).max(256),
    value: z.string().max(16_384),
  })
  .strict();

const requestSchema = z
  .object({
    copies: z
      .array(
        z
          .object({
            destination: z.string().min(1).max(240),
            overwrite: z.boolean(),
            replacements: z.array(replacementSchema).max(32),
            source: sourceNameSchema,
          })
          .strict(),
      )
      .min(1)
      .max(12),
    schemaVersion: z.literal(1),
  })
  .strict();

const snapshotSchema = z
  .object({
    branch: z.string().nullable(),
    head: commitSchema,
    originSha256: digestSchema.nullable(),
    worktreeSha256: digestSchema,
  })
  .strict();

const operationSchema = z
  .object({
    contentSha256: digestSchema,
    destination: z.string().min(1).max(240),
    expectedDestinationSha256: digestSchema.nullable(),
    overwrite: z.boolean(),
    replacements: z.array(replacementSchema).max(32),
    source: sourceNameSchema,
    sourcePath: z.string().min(1),
    sourceSha256: digestSchema,
  })
  .strict();

const observationSchema = z
  .object({
    endedAt: z.string().datetime(),
    startedAt: z.string().datetime(),
  })
  .strict();

const planSchema = z
  .object({
    operations: z.array(operationSchema).min(1).max(12),
    observation: observationSchema,
    package: z
      .object({
        contentSha256: digestSchema,
        name: z.literal(packageName),
        version: z.string().regex(/^\d+\.\d+\.\d+$/),
      })
      .strict(),
    repository: snapshotSchema,
    requestSha256: digestSchema,
    schemaVersion: z.literal(1),
    source: z
      .object({
        commit: commitSchema,
        ref: z.literal(authorityRef),
        repository: z.literal(authorityRepository),
      })
      .strict(),
  })
  .strict();

export type PlanRequest = z.infer<typeof requestSchema>;
export type GoldenPathPlan = z.infer<typeof planSchema>;

export interface ApplyResult {
  approvedPlanSha256: string;
  observation: {
    endedAt: string;
    startedAt: string;
  };
  package: {
    contentSha256: string;
    name: typeof packageName;
    version: string;
  };
  repository: GoldenPathPlan["repository"];
  sourceCommit: string;
  sourceFiles: {
    path: string;
    sha256: string;
  }[];
  written: string[];
}

function describeZodError(error: z.ZodError): string {
  return error.issues
    .map((issue) => `${issue.path.join(".") || "document"}: ${issue.message}`)
    .join("; ");
}

async function readBoundedJson(file: string): Promise<{
  source: string;
  value: unknown;
}> {
  const fileStats = await stat(file);
  if (!fileStats.isFile() || fileStats.size > 256 * 1024) {
    throw new Error(
      `JSON input must be a regular file no larger than 256 KiB: ${file}`,
    );
  }
  const source = await readFile(file, "utf8");
  try {
    return { source, value: JSON.parse(source) as unknown };
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Invalid JSON in ${file}: ${detail}`, { cause: error });
  }
}

function isInside(root: string, candidate: string): boolean {
  const relation = relative(root, candidate);
  return (
    relation === "" || (!relation.startsWith(`..${sep}`) && relation !== "..")
  );
}

async function assertExternalInput(
  root: string,
  file: string,
): Promise<string> {
  const actual = await realpath(resolve(file));
  if (isInside(root, actual)) {
    throw new Error(`Input must remain outside the target repository: ${file}`);
  }
  return actual;
}

async function assertExternalOutput(
  root: string,
  file: string,
): Promise<string> {
  const output = resolve(file);
  const actualParent = await realpath(dirname(output));
  const actual = join(actualParent, basename(output));
  if (isInside(root, actual)) {
    throw new Error(
      `Plan output must remain outside the target repository: ${file}`,
    );
  }
  try {
    await lstat(actual);
    throw new Error(`Refusing to overwrite an existing plan output: ${actual}`);
  } catch (error) {
    if (
      error instanceof Error &&
      "code" in error &&
      (error as NodeJS.ErrnoException).code === "ENOENT"
    ) {
      return actual;
    }
    throw error;
  }
}

function validateDestinationSyntax(destination: string): void {
  const parts = destination.split("/");
  if (
    isAbsolute(destination) ||
    destination.includes("\\") ||
    parts.some((part) => part === "" || part === "." || part === "..") ||
    parts.some((part) => part.toLowerCase() === ".git")
  ) {
    throw new Error(`Unsafe repository-relative destination: ${destination}`);
  }
}

function destinationCollisionKey(destination: string): string {
  return destination.normalize("NFC").toLowerCase();
}

async function assertSafeDestination(
  root: string,
  destination: string,
): Promise<string> {
  validateDestinationSyntax(destination);
  const target = resolve(root, destination);
  if (!isInside(root, target)) {
    throw new Error(`Destination escapes the repository: ${destination}`);
  }
  const parts = destination.split("/");
  let current = root;
  for (const part of parts) {
    current = join(current, part);
    try {
      const currentStats = await lstat(current);
      if (currentStats.isSymbolicLink()) {
        throw new Error(
          `Destination traverses a symbolic link: ${destination}`,
        );
      }
      if (current !== target && !currentStats.isDirectory()) {
        throw new Error(
          `Destination parent is not a directory: ${destination}`,
        );
      }
      if (current === target && !currentStats.isFile()) {
        throw new Error(
          `Existing destination is not a regular file: ${destination}`,
        );
      }
    } catch (error) {
      if (
        error instanceof Error &&
        "code" in error &&
        (error as NodeJS.ErrnoException).code === "ENOENT"
      ) {
        if (current === target) {
          break;
        }
        throw new Error(
          `Destination parent does not exist; create repository-owned directories before planning: ${destination}`,
          { cause: error },
        );
      }
      throw error;
    }
  }
  return target;
}

function countOccurrences(source: string, match: string): number {
  return source.split(match).length - 1;
}

function renderSource(
  source: string,
  replacements: readonly z.infer<typeof replacementSchema>[],
): string {
  let rendered = source;
  for (const replacement of replacements) {
    const observed = countOccurrences(rendered, replacement.match);
    if (observed !== replacement.expectedOccurrences) {
      throw new Error(
        `Replacement count mismatch for ${JSON.stringify(replacement.match)}: expected ${replacement.expectedOccurrences}, observed ${observed}.`,
      );
    }
    rendered = rendered.replaceAll(replacement.match, replacement.value);
  }
  if (/<EXACT_[A-Z0-9_]+>/.test(rendered)) {
    throw new Error(
      "Rendered copy-once source retains unresolved <EXACT_...> tokens.",
    );
  }
  return rendered;
}

async function readDestinationDigest(target: string): Promise<string | null> {
  try {
    return sha256(await readFile(target));
  } catch (error) {
    if (
      error instanceof Error &&
      "code" in error &&
      (error as NodeJS.ErrnoException).code === "ENOENT"
    ) {
      return null;
    }
    throw error;
  }
}

function snapshotForPlan(
  snapshot: RepositorySnapshot,
): GoldenPathPlan["repository"] {
  return {
    branch: snapshot.branch,
    head: snapshot.head,
    originSha256: snapshot.originSha256,
    worktreeSha256: snapshot.worktreeSha256,
  };
}

function assertSameSnapshot(
  expected: GoldenPathPlan["repository"],
  actual: RepositorySnapshot,
): void {
  if (
    expected.head !== actual.head ||
    expected.branch !== actual.branch ||
    expected.originSha256 !== actual.originSha256 ||
    expected.worktreeSha256 !== actual.worktreeSha256
  ) {
    throw new Error(
      "Repository branch, HEAD, origin, or complete worktree state changed after planning; create a new plan.",
    );
  }
}

export async function createPlan(options: {
  authorityClient: AuthorityClient;
  output: string;
  request: string;
  root: string;
  runner: ProcessRunner;
}): Promise<GoldenPathPlan> {
  const startedAt = new Date().toISOString();
  const root = await resolveRepositoryRoot(options.runner, options.root);
  const requestFile = await assertExternalInput(root, options.request);
  const outputFile = await assertExternalOutput(root, options.output);
  const requestDocument = await readBoundedJson(requestFile);
  const parsedRequest = requestSchema.safeParse(requestDocument.value);
  if (!parsedRequest.success) {
    throw new Error(
      `Invalid plan request: ${describeZodError(parsedRequest.error)}`,
    );
  }
  const destinations = new Set<string>();
  for (const copy of parsedRequest.data.copies) {
    validateDestinationSyntax(copy.destination);
    const destinationKey = destinationCollisionKey(copy.destination);
    if (destinations.has(destinationKey)) {
      throw new Error(
        `Duplicate or filesystem-aliased destination in plan request: ${copy.destination}`,
      );
    }
    destinations.add(destinationKey);
  }

  const before = await readRepositorySnapshot(options.runner, root);
  const commit = await options.authorityClient.resolveCommit();
  const operations: GoldenPathPlan["operations"] = [];
  for (const copy of parsedRequest.data.copies) {
    const sourcePath = copyOnceSources[copy.source];
    const source = await options.authorityClient.readText(sourcePath, commit);
    const rendered = renderSource(source, copy.replacements);
    const target = await assertSafeDestination(root, copy.destination);
    const existingDigest = await readDestinationDigest(target);
    if (existingDigest && !copy.overwrite) {
      throw new Error(
        `Destination exists but overwrite was not approved in the request: ${copy.destination}`,
      );
    }
    operations.push({
      contentSha256: sha256(rendered),
      destination: copy.destination,
      expectedDestinationSha256: existingDigest,
      overwrite: copy.overwrite,
      replacements: copy.replacements,
      source: copy.source,
      sourcePath,
      sourceSha256: sha256(source),
    });
  }
  const after = await readRepositorySnapshot(options.runner, root);
  assertSameSnapshot(snapshotForPlan(before), after);
  const metadata = await getPackageMetadata();
  const plan: GoldenPathPlan = {
    operations,
    observation: { endedAt: new Date().toISOString(), startedAt },
    package: {
      contentSha256: metadata.contentSha256,
      name: metadata.name,
      version: metadata.version,
    },
    repository: snapshotForPlan(after),
    requestSha256: sha256(requestDocument.source),
    schemaVersion: 1,
    source: {
      commit,
      ref: authorityRef,
      repository: authorityRepository,
    },
  };
  await writeFile(outputFile, `${JSON.stringify(plan, null, 2)}\n`, {
    encoding: "utf8",
    flag: "wx",
    mode: 0o600,
  });
  return plan;
}

interface PreparedWrite {
  content: string;
  destination: string;
  original: Buffer | null;
  target: string;
}

async function applyPreparedWrites(
  root: string,
  writes: readonly PreparedWrite[],
): Promise<void> {
  const completed: PreparedWrite[] = [];
  try {
    for (const entry of writes) {
      const revalidatedTarget = await assertSafeDestination(
        root,
        entry.destination,
      );
      if (revalidatedTarget !== entry.target) {
        throw new Error(
          `Destination resolution changed before write: ${entry.destination}`,
        );
      }
      const temporary = join(
        dirname(entry.target),
        `.${basename(entry.target)}.golden-path-agent-${randomUUID()}`,
      );
      try {
        await writeFile(temporary, entry.content, { flag: "wx", mode: 0o644 });
        await rename(temporary, entry.target);
      } finally {
        await rm(temporary, { force: true });
      }
      completed.push(entry);
    }
  } catch (error) {
    for (const entry of completed.reverse()) {
      if (entry.original === null) {
        const target = await assertSafeDestination(root, entry.destination);
        await rm(target, { force: true });
      } else {
        const target = await assertSafeDestination(root, entry.destination);
        await writeFile(target, entry.original);
      }
    }
    throw error;
  }
}

export async function applyPlan(options: {
  approvedPlanSha256: string;
  authorityClient: AuthorityClient;
  plan: string;
  root: string;
  runner: ProcessRunner;
}): Promise<ApplyResult> {
  const startedAt = new Date().toISOString();
  const root = await resolveRepositoryRoot(options.runner, options.root);
  const planFile = await assertExternalInput(root, options.plan);
  const planDocument = await readBoundedJson(planFile);
  if (!digestSchema.safeParse(options.approvedPlanSha256).success) {
    throw new Error(
      "Approved plan SHA-256 must use sha256:<64 lowercase hex>.",
    );
  }
  const actualPlanSha256 = sha256(planDocument.source);
  if (actualPlanSha256 !== options.approvedPlanSha256) {
    throw new Error(
      `Plan file does not match the explicitly approved digest: expected ${options.approvedPlanSha256}, observed ${actualPlanSha256}.`,
    );
  }
  const parsedPlan = planSchema.safeParse(planDocument.value);
  if (!parsedPlan.success) {
    throw new Error(`Invalid plan: ${describeZodError(parsedPlan.error)}`);
  }
  const plan = parsedPlan.data;
  const metadata = await getPackageMetadata();
  if (
    plan.package.name !== metadata.name ||
    plan.package.version !== metadata.version ||
    plan.package.contentSha256 !== metadata.contentSha256
  ) {
    throw new Error(
      `Plan requires exact ${plan.package.name}@${plan.package.version} content ${plan.package.contentSha256}; current package is ${metadata.name}@${metadata.version} content ${metadata.contentSha256}.`,
    );
  }
  const currentAuthorityCommit = await options.authorityClient.resolveCommit();
  if (currentAuthorityCommit !== plan.source.commit) {
    throw new Error(
      `Current ${plan.source.repository}/${plan.source.ref} changed after planning: expected ${plan.source.commit}, observed ${currentAuthorityCommit}; create and approve a fresh plan.`,
    );
  }
  const snapshot = await readRepositorySnapshot(options.runner, root);
  assertSameSnapshot(plan.repository, snapshot);

  const destinations = new Set<string>();
  const prepared: PreparedWrite[] = [];
  for (const operation of plan.operations) {
    const destinationKey = destinationCollisionKey(operation.destination);
    if (destinations.has(destinationKey)) {
      throw new Error(
        `Duplicate or filesystem-aliased destination in plan: ${operation.destination}`,
      );
    }
    destinations.add(destinationKey);
    if (copyOnceSources[operation.source] !== operation.sourcePath) {
      throw new Error(
        `Plan source mapping is not current: ${operation.source}`,
      );
    }
    const target = await assertSafeDestination(root, operation.destination);
    const existingDigest = await readDestinationDigest(target);
    if (existingDigest !== operation.expectedDestinationSha256) {
      throw new Error(
        `Destination changed after planning: ${operation.destination}`,
      );
    }
    if (existingDigest && !operation.overwrite) {
      throw new Error(
        `Plan does not authorize overwrite: ${operation.destination}`,
      );
    }

    const source = await options.authorityClient.readText(
      operation.sourcePath,
      plan.source.commit,
    );
    if (sha256(source) !== operation.sourceSha256) {
      throw new Error(
        `Authority source hash changed or was not resolved: ${operation.sourcePath}`,
      );
    }
    const rendered = renderSource(source, operation.replacements);
    if (sha256(rendered) !== operation.contentSha256) {
      throw new Error(
        `Rendered content no longer matches the plan: ${operation.destination}`,
      );
    }
    prepared.push({
      content: rendered,
      destination: operation.destination,
      original: existingDigest ? await readFile(target) : null,
      target,
    });
  }

  const beforeWrite = await readRepositorySnapshot(options.runner, root);
  assertSameSnapshot(plan.repository, beforeWrite);
  await applyPreparedWrites(root, prepared);
  const sourceFiles = [
    ...new Map(
      plan.operations.map((operation) => [
        operation.sourcePath,
        { path: operation.sourcePath, sha256: operation.sourceSha256 },
      ]),
    ).values(),
  ].sort((left, right) => left.path.localeCompare(right.path));
  return {
    approvedPlanSha256: options.approvedPlanSha256,
    observation: { endedAt: new Date().toISOString(), startedAt },
    package: plan.package,
    repository: plan.repository,
    sourceCommit: plan.source.commit,
    sourceFiles,
    written: prepared.map((entry) => entry.destination),
  };
}

export function parsePlanRequest(value: unknown): PlanRequest {
  const result = requestSchema.safeParse(value);
  if (!result.success) {
    throw new Error(`Invalid plan request: ${describeZodError(result.error)}`);
  }
  return result.data;
}
