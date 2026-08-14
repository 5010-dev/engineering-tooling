import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const root = resolve(import.meta.dirname, "..");
const persistentOutput = process.env.GOLDEN_PATH_AGENT_PACKAGE_OUTPUT;
const output = persistentOutput
  ? resolve(persistentOutput)
  : await mkdtemp(join(tmpdir(), "golden-path-agent-package-check-"));
if (persistentOutput) {
  await mkdir(output);
}
const consumer = join(output, "consumer");
const pnpm = process.env.npm_execpath;

if (!pnpm) {
  throw new Error(
    "package-check must run through the repository-owned pnpm command.",
  );
}

async function runPnpm(args, cwd = root) {
  return execFileAsync(process.execPath, [pnpm, ...args], {
    cwd,
    encoding: "utf8",
    maxBuffer: 5 * 1024 * 1024,
  });
}

const packed = await runPnpm(["pack", "--json", "--pack-destination", output]);
const manifest = JSON.parse(packed.stdout);
if (
  manifest.name !== "@5010-dev/golden-path-agent" ||
  manifest.version !== "1.0.1" ||
  typeof manifest.filename !== "string" ||
  !Array.isArray(manifest.files)
) {
  throw new Error("pnpm pack returned an unexpected package manifest.");
}
const tarball = await readFile(manifest.filename);
const packageEvidence = {
  filename: manifest.filename,
  integrity: `sha512-${createHash("sha512").update(tarball).digest("base64")}`,
  name: manifest.name,
  sha256: createHash("sha256").update(tarball).digest("hex"),
  version: manifest.version,
};

const packagedPaths = new Set(manifest.files.map((entry) => entry.path));
const generatedEntries = [
  "authority",
  "cli",
  "constants",
  "hashing",
  "index",
  "metadata",
  "plan",
  "process",
  "report",
  "repository",
  "skill",
];
const expectedPaths = new Set(["package.json", "README.md"]);
for (const entry of generatedEntries) {
  for (const suffix of [".d.ts", ".d.ts.map", ".js", ".js.map"]) {
    expectedPaths.add(`lib/${entry}${suffix}`);
  }
}
for (const skillPath of [
  "SKILL.md",
  "agents/openai.yaml",
  "references/authority-boundaries.md",
  "references/copy-once-workflow.md",
]) {
  expectedPaths.add(`lib/skills/golden-path/${skillPath}`);
}
const missingPaths = [...expectedPaths].filter(
  (path) => !packagedPaths.has(path),
);
const unexpectedPaths = [...packagedPaths].filter(
  (path) => !expectedPaths.has(path),
);
if (missingPaths.length > 0 || unexpectedPaths.length > 0) {
  throw new Error(
    `Package tarball allowlist mismatch; missing=${JSON.stringify(missingPaths)}, unexpected=${JSON.stringify(unexpectedPaths)}.`,
  );
}

const { stdout: packagedReadme } = await execFileAsync("tar", [
  "-xOf",
  manifest.filename,
  "package/README.md",
]);
for (const requiredFragment of [
  "@5010-dev/golden-path-agent@1.0.1",
  "read:packages",
  "${NODE_AUTH_TOKEN}",
  "https://linear.new?team=ENG",
  "pnpm setup",
  "pnpm add --global",
]) {
  if (!packagedReadme.includes(requiredFragment)) {
    throw new Error(
      `Packed README is missing required onboarding content: ${requiredFragment}`,
    );
  }
}
if (
  packagedReadme.includes(
    "General support uses this repository's GitHub Issues",
  )
) {
  throw new Error(
    "Packed README retains the retired GitHub Issues support route.",
  );
}

await runPnpm(["exec", "publint", manifest.filename, "--strict"]);
await runPnpm([
  "exec",
  "attw",
  manifest.filename,
  "--profile",
  "esm-only",
  "--no-definitely-typed",
  "--quiet",
]);

await mkdir(consumer, { recursive: true });
await writeFile(
  join(consumer, "package.json"),
  `${JSON.stringify(
    {
      name: "golden-path-agent-package-smoke",
      private: true,
      type: "module",
      dependencies: {
        "@5010-dev/golden-path-agent": `file:${manifest.filename}`,
      },
    },
    null,
    2,
  )}\n`,
);
await runPnpm(["install", "--ignore-scripts"], consumer);
await writeFile(
  join(consumer, "smoke.mjs"),
  `import { parsePlanRequest, resolveAgentSkillDestination } from "@5010-dev/golden-path-agent";

const request = parsePlanRequest({
  schemaVersion: 1,
  copies: [{
    source: "canonical-ci",
    destination: ".github/workflows/ci.yml",
    overwrite: false,
    replacements: [],
  }],
});
if (request.copies.length !== 1) process.exit(1);
if (!resolveAgentSkillDestination({ host: "codex", homeDirectory: "/tmp/home" }).endsWith("/.agents/skills/golden-path")) process.exit(1);
`,
);
await execFileAsync(process.execPath, [join(consumer, "smoke.mjs")], {
  cwd: consumer,
  encoding: "utf8",
});

const installedBin = join(
  consumer,
  "node_modules",
  ".bin",
  "golden-path-agent",
);
const { stdout: versionOutput } = await execFileAsync(installedBin, [
  "--version",
]);
if (versionOutput.trim() !== manifest.version) {
  throw new Error("Packed CLI returned an unexpected version.");
}

const skillHome = join(output, "skill-home");
await execFileAsync(installedBin, [
  "skill",
  "install",
  "--host",
  "all",
  "--home",
  skillHome,
]);
await execFileAsync(installedBin, [
  "skill",
  "check",
  "--host",
  "all",
  "--home",
  skillHome,
]);
const codexSkill = await readFile(
  join(skillHome, ".agents", "skills", "golden-path", "SKILL.md"),
  "utf8",
);
const codexPolicy = await readFile(
  join(skillHome, ".agents", "skills", "golden-path", "agents", "openai.yaml"),
  "utf8",
);
const claudeSkill = await readFile(
  join(skillHome, ".claude", "skills", "golden-path", "SKILL.md"),
  "utf8",
);
if (
  codexSkill.includes("disable-model-invocation: true") ||
  !codexPolicy.includes("allow_implicit_invocation: false") ||
  !claudeSkill.includes("disable-model-invocation: true")
) {
  throw new Error("Packed Skill explicit-invocation policy is not preserved.");
}

const installedManifest = JSON.parse(
  await readFile(
    join(
      consumer,
      "node_modules",
      "@5010-dev",
      "golden-path-agent",
      "package.json",
    ),
    "utf8",
  ),
);
if (
  installedManifest.name !== manifest.name ||
  installedManifest.version !== manifest.version
) {
  throw new Error("Fresh consumer resolved an unexpected package identity.");
}

console.log(
  `package-check: OK (${manifest.name}@${manifest.version}, sha256:${packageEvidence.sha256})`,
);
if (persistentOutput) {
  await writeFile(
    join(output, "package-evidence.json"),
    `${JSON.stringify(packageEvidence, null, 2)}\n`,
    { flag: "wx" },
  );
} else {
  await rm(output, { force: true, recursive: true });
}
