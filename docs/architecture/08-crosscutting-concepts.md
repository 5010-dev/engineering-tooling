# 8. Cross-cutting concepts

State: **As-built**

Authority paths are allowlisted and read from `github.com` with `gh` at a 40-character commit, bounded to 1 MiB each. Plan authority selection includes the core Golden Path documents, the normative documents for each requested copy-once source, and every profile document selected from repository facts. Apply derives that path set again, re-reads every raw source, and emits the repository/ref/commit plus path/digest manifest. JSON inputs are strict, bounded to 256 KiB, and must live outside the target repository. Repository destinations must be relative, stay outside case-folded `.git`, contain no traversal, use existing repository-owned parents, cross no symlink, and remain unique under a Unicode NFC-normalized lowercase collision key. Apply requires the digest of the exact user-reviewed plan, requires current authority main to remain the planned commit, and rechecks each destination content and mode immediately before writing. Overwrites and rollback preserve the planned mode; new files use `0644`.

The CLI removes URL user information, query, and fragment components before emitting a configured origin, persists only a digest of the raw origin, and delegates authentication to `gh` or the registry workflow. Skill installation rejects symlinked home and host parents and uses ownership markers only inside user Skill directories. The pnpm bootstrap treats an override as a cache parent and atomically replaces only its package-owned executable leaf. Copy-once repository files receive no marker and are immediately repository-owned.

Report language keeps `report-only`, `policy-required`, and `platform-enforced` distinct. The package never infers the latter two.
