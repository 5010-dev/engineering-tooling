# Stable minor release readiness

This document is release evidence, not normative policy. The organization
Developer Tooling Standard remains the authority for rule meaning and
applicability.

## Release candidate identity

- Standard: `2026.08.1`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.3.0`
- Release lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `a4956db0996516089b0c7da56e23c0fa4c72add1`
- Normative source tree: `c576c91a672bb2c78ba441773dd757e50f69c9a3`
- External-tool cutoff: `2026-08-04T12:54:32Z` (retained selections)
- Candidate source integrity: `2026-08-06` (current release bytes)

This compatible minor release removes duplicate repository-quality execution
from the structural conformance workflow and reduces default human output. It
does not change the v1 request, metadata, exception, result, inventory, or plan
contracts. The immutable `1.2.4`
[`tooling-cutoff-2026-08-04.json`](./tooling-cutoff-2026-08-04.json) retains the
external-tool selections that this release reuses. The separate
[`source-integrity-2026-08-06.json`](./source-integrity-2026-08-06.json) binds
the current release's locks and generated bundle without rewriting dated
historical evidence.

The reusable entrypoint keeps the already released
`.github/workflows/golden-path-quality.yml` path. Exact source pins separate the
old quality implementation from the new checker-only implementation, while the
stable manifest v2 automation identity remains compatible.

## Required evidence before tagging

1. The accepted `2026.08.1` standard has an immutable organization-policy
   source commit and tree. The imported snapshot must be byte-identical, have a
   complete file inventory, and bind the source commit, tree, catalog digest,
   and aggregate digest together.
2. `just ci` succeeds from a clean checkout of the exact candidate source.
3. Checker tests prove that default text omits individual pass and skip detail,
   never hides fail, warn, error, waiver, or expired-exception evidence, and
   retains the complete v1 JSON finding set. Explicit exhaustive text must
   contain every finding.
4. Generator tests prove the caller pins the new reusable workflow by full
   source SHA, passes only runner, working directory, and profiles, and removes
   caller-owned checker version, source commit, verifier version, and platform
   checksums.
5. Pull-request CI invokes the exact reusable workflow against the current
   source candidate. Executable workflow-contract tests run its caller identity
   and exit-mapping scripts, while actionlint validates workflow structure.
   Together they prove `job.workflow_*` handling, checker-only execution,
   report-only exit `1`, failing exits `2` and `3`, and conditional JSON
   retention for non-passing results.
6. The workflow source contains no consumer-toolchain install, `just init`, or
   `just ci` execution. The ordinary repository quality workflow remains the
   single owner of `just ci`.
7. The `1.2.4` polyglot-adoption and shared-native-workspace fixtures remain
   green, along with all supported generated profiles, native release targets,
   upgrade planning, rollback, packaging, schema, and provenance tests.
8. Every release archive is built and executed on its native platform. SBOMs
   bind archive digests; attestations cover all published assets; the release
   manifest binds source, catalog, snapshot, templates, automation, retained
   external-tool cutoff, binaries, schemas, and supported targets. Release
   checksums and attestations bind the separate current source-integrity record.

The stable tag must not be created until these gates have evidence against the
exact source commit. After publication and before advancing the organization
locator, an integration caller pinned to the exact `1.3.0` source SHA must
exercise attested release acquisition and complete independently of quality
CI. A non-passing smoke run must also prove conditional JSON artifact upload.
If either smoke fails, leave the locator on `1.2.4` and publish a correction;
never replace `1.3.0` assets. Final asset digests, release URL, smoke runs, and
locator pins are post-publication evidence and cannot be predeclared.

## Compatibility and support boundary

`1.2.4` remains an immutable supported release. Advancing the organization
locator to `1.3.0` does not require every consumer to open an immediate upgrade
pull request. Consumers may batch compatible `1.x` maintenance on their normal
cadence unless a release is explicitly classified as a security, integrity,
material false-failure, or unusable-workflow replacement.

This change needs no compatibility reader, dual workflow, generated-state
migration, deployment rollout, or runtime-state handling. Exact pins mean a
`1.2.4` consumer continues to execute the `1.2.4` workflow contract, while an
upgraded consumer moves the generated control-plane baseline as one reviewable
unit.

## GitHub Free private baseline

The checker needs no network, GitHub API, credentials, organization secret,
protected branch, ruleset, required check, Environment, Dependency Review, or
private artifact attestation. The public reusable workflow needs only
`contents: read` and the caller-provided runner, working directory, and profile
declaration. It preserves findings while converting only conformance exit `1`
to a report-only job result.

## Cost and evidence boundary

Consumer conformance no longer repeats the repository's quality graph. It runs
independently and produces one bounded summary and annotation set. Passing runs
do not upload duplicate JSON; non-passing runs retain the complete result for
diagnosis. Individual passing and skipped findings are diagnostic detail rather
than default PR evidence. The initial workflow timeout is ten minutes and may
be tightened only after repeated hosted-runner timings establish a safe bound.

The public tooling repository still runs the broader native-platform,
generator, migration, rollback, packaging, and provenance matrix required to
publish one reusable release. Those release-owner checks are not copied into
consumer repositories.

## Rollback

Restore the exact `1.2.4` control-plane baseline from consumer repository
history: request, metadata, generated asset inventory, bootstrap script, binary
identity, and full-commit workflow pin. Product source, native manifests,
locks, quality commands, deployment behavior, runtime state, and durable data
are unchanged and need no rollback or migration.

Never mix a `1.3.0` caller with the `1.2.4` reusable workflow inputs, move a
published tag, or replace release assets.
