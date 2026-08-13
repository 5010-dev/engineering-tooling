export const packageName = "@5010-dev/golden-path-agent";
export const skillName = "golden-path";
export const skillMarkerName = ".fiftyten-golden-path-agent.json";

export const authorityRepository = "5010-dev/.github";
export const authorityRef = "main";

export const authorityDocumentPaths = [
  "CONTRIBUTING.md",
  "docs/standards/developer-tooling/README.md",
  "docs/golden-path/README.md",
  "docs/golden-path/reference-examples.md",
] as const;

export const copyOnceSources = {
  "canonical-ci": "docs/golden-path/examples/canonical-ci.yml",
  dependabot: "docs/golden-path/examples/dependabot.yml",
  "go-just": "docs/golden-path/examples/go.just",
  "native-roots": "docs/golden-path/examples/native-roots.yml",
  "node-just": "docs/golden-path/examples/node.just",
  "node-toolchain": "docs/golden-path/examples/toolchain-node.toml",
  "python-just": "docs/golden-path/examples/python.just",
} as const;

export type CopyOnceSource = keyof typeof copyOnceSources;

export const planAuthorityBasePaths = [
  ...authorityDocumentPaths,
  "docs/golden-path/stack-defaults.md",
  "docs/standards/developer-tooling/profiles/README.md",
] as const;

const profileAuthorityPaths = {
  go: "docs/standards/developer-tooling/profiles/go.md",
  "infrastructure-aws-cdk":
    "docs/standards/developer-tooling/profiles/infrastructure.md",
  "infrastructure-hcl":
    "docs/standards/developer-tooling/profiles/infrastructure.md",
  "infrastructure-opentofu":
    "docs/standards/developer-tooling/profiles/infrastructure.md",
  "infrastructure-pulumi":
    "docs/standards/developer-tooling/profiles/infrastructure.md",
  "infrastructure-terraform":
    "docs/standards/developer-tooling/profiles/infrastructure.md",
  "node-typescript":
    "docs/standards/developer-tooling/profiles/node-typescript.md",
  python: "docs/standards/developer-tooling/profiles/python.md",
  rust: "docs/standards/developer-tooling/profiles/rust.md",
  zig: "docs/standards/developer-tooling/profiles/zig.md",
  "zig-toolchain": "docs/standards/developer-tooling/profiles/zig.md",
} as const;

const copyOnceAuthorityPaths: Record<CopyOnceSource, readonly string[]> = {
  "canonical-ci": [
    "docs/standards/developer-tooling/command-contract.md",
    "docs/standards/developer-tooling/conformance.md",
  ],
  dependabot: ["docs/standards/developer-tooling/dependency-management.md"],
  "go-just": [
    "docs/standards/developer-tooling/task-runner.md",
    profileAuthorityPaths.go,
  ],
  "native-roots": [
    "docs/standards/developer-tooling/schemas/golden-path-native-roots-v1.schema.json",
  ],
  "node-just": [
    "docs/standards/developer-tooling/task-runner.md",
    profileAuthorityPaths["node-typescript"],
  ],
  "node-toolchain": [
    "docs/standards/developer-tooling/runtime-support.md",
    "docs/standards/developer-tooling/toolchain-management.md",
    profileAuthorityPaths["node-typescript"],
  ],
  "python-just": [
    "docs/standards/developer-tooling/task-runner.md",
    profileAuthorityPaths.python,
  ],
};

export function selectPlanAuthorityPaths(
  sources: readonly CopyOnceSource[],
  profiles: readonly string[],
): string[] {
  const selected = new Set<string>(planAuthorityBasePaths);
  for (const source of sources) {
    for (const path of copyOnceAuthorityPaths[source]) {
      selected.add(path);
    }
  }
  for (const profile of profiles) {
    const path = profileAuthorityPaths[
      profile as keyof typeof profileAuthorityPaths
    ] as string | undefined;
    if (!path) {
      throw new Error(
        `No normative authority document maps profile: ${profile}`,
      );
    }
    selected.add(path);
  }
  return [...selected].sort();
}

export const allowedAuthorityPaths = new Set<string>([
  ...planAuthorityBasePaths,
  ...Object.values(profileAuthorityPaths),
  ...Object.values(copyOnceAuthorityPaths).flat(),
  ...Object.values(copyOnceSources),
]);
