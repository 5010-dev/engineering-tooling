# 8. Cross-cutting concepts

State: **As-built**

Authority paths are allowlisted and read from `github.com` with `gh` at a 40-character commit, bounded to 1 MiB each. JSON inputs are strict, bounded to 256 KiB, and must live outside the target repository. Repository destinations must be relative, stay outside case-folded `.git`, contain no traversal, use existing repository-owned parents, cross no symlink, and remain unique under a Unicode NFC-normalized lowercase collision key. Apply requires the digest of the exact user-reviewed plan, requires current authority main to remain the planned commit, and rechecks each destination immediately before writing.

The CLI emits no credentials, persists only a digest of the configured origin, and delegates authentication to `gh` or the registry workflow. Skill installation rejects symlinked home and host parents and uses ownership markers only inside user Skill directories. The pnpm bootstrap treats an override as a cache parent and atomically replaces only its package-owned executable leaf. Copy-once repository files receive no marker and are immediately repository-owned.

Report language keeps `report-only`, `policy-required`, and `platform-enforced` distinct. The package never infers the latter two.
