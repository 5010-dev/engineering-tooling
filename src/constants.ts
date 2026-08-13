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

export const allowedAuthorityPaths = new Set<string>([
  ...authorityDocumentPaths,
  ...Object.values(copyOnceSources),
]);
