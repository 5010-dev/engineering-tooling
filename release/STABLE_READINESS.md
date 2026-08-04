# Stable minor release readiness

This document is release evidence, not normative policy. The organization
Developer Tooling Standard remains the authority for rule meaning and
applicability.

## Release candidate identity

- Standard: `2026.08`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.2.0`
- Release lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `604a40886d5a1cba3b304471e6e072b72cec7601`
- External-tool cutoff: `2026-08-04T12:54:32Z`

The minor increment is required because the released generator request gains an
optional explicit component-capability input. Legacy requests retain their
canonical bytes and digest, and no contract epoch changes. Runtime selections,
external tools, generated dependency locks, workflow actions, and the checker
rule catalog remain unchanged. Release-specific integrity evidence is bound by
[`tooling-cutoff-2026-08-04.json`](./tooling-cutoff-2026-08-04.json).

## Required evidence before tagging

1. `just ci` succeeds from a clean checkout of the candidate source.
2. The bundled `2026.08` snapshot remains byte-identical to organization-policy
   commit `604a40886d5a1cba3b304471e6e072b72cec7601`; its source tree, complete file
   inventory, catalog digest, and aggregate digest verify together.
3. The published request schema and generator semantics reject unknown,
   duplicate, and empty capabilities while accepting the normative metadata
   capability catalog.
4. Tests prove aggregate and component metadata contain the deterministic union
   of baseline and explicit capabilities, and relevant capability-scoped rules
   no longer skip for undeclared applicability.
5. A legacy request retains its exact canonical request bytes and SHA-256
   digest, and upgrading it to explicit capabilities is conflict-free,
   source-preserving, and materializes the expected candidate metadata.
6. The setup action verifies released `0.2.0` and `1.1.0` provenance. Projects
   materialized by both releases upgrade to a separate `1.2.0` candidate and
   roll back without source mutation or unresolved conflicts.
7. Every generated profile runs on its declared native CI runner, and the
   reusable-workflow integration executes against an actual `1.1.0` generated
   repository and immutable release identity.
8. SBOMs bind every archive digest; release attestations cover every published
   asset; the release manifest binds source, catalog, snapshot, templates,
   automation, cutoff, binaries, schemas, and supported targets.
9. A separately approved read-only review confirms the complete candidate diff
   preserves the implementation, policy, migration, and release boundaries.

The stable tag must not be created until all nine gates have evidence against
the exact source commit. Final source SHA, asset digests, workflow run, release
URL, and bootstrap-locator pins are post-publication evidence and cannot be
predeclared.

## Compatibility and ownership boundary

The new request field is optional. Existing repositories do not need to change
their request, metadata, or generated files unless they adopt `1.2.0` through
their normal migration workflow. A component declares only capabilities its
repository actually implements. The generator merges the declaration into
metadata but does not infer publication from artifact type or create publishing
automation, credentials, registry configuration, or release evidence.

This is compatibility work for a released `1.1.0` request boundary. It is
bounded to accepting old requests unchanged and offering a direct one-way
candidate upgrade; it does not introduce dual schemas, dual writers, or a new
wire-version chain.

## GitHub Free private baseline

The checker needs no network, GitHub API, credentials, organization secret,
protected branch, ruleset, required check, Environment, Dependency Review, or
private artifact attestation. The reusable workflow preserves findings while
converting only conformance exit `1` to a report-only job result. Configuration
and internal errors (`2` and `3`) still fail closed.

The `1.2.0` channel contains only public generic implementation assets. It does
not contain, depend on, or claim support for a restricted implementation.

## CI cost boundary

Central pull-request validation remains 12 GitHub-hosted jobs: one repository
gate, four native targets, four generated-profile targets, two migration and
rollback matrix entries, and one reusable-workflow integration job. The maximum
configured aggregate runner time remains 275 minutes, the longest individual
timeout remains 40 minutes, and the matrix runs only in the public
implementation repository. The tag-only release path remains seven jobs with
95 aggregate timeout-minutes.

## Adoption boundary

New-repository materialization records tooling bundle `1.2.0`, emits a thin
report-only caller, and writes only to an explicitly selected empty staging
directory. Existing repositories are not discovered, enrolled, or changed.
Adding explicit capabilities is repository-owned adoption work based on actual
behavior, not a central migration side effect.

After publication, the organization bootstrap locator is updated in a separate
policy-repository change using the exact release source commit, manifest and
snapshot-manifest hashes, archive hashes, and immutable automation pins.

## Support, deprecation, and rollback

`1.x` remains the supported stable implementation line. `1.1.0` is the
immediate immutable rollback source. The 2026-08-01 announcement continues to
deprecate `0.1.x` and `0.2.x` through 2027-01-28; `0.2.0` remains a verified
legacy migration source, while `0.1.x` must first advance to `0.2.0`.

Rollback restores the exact binary, release manifest, archive and executable
checksums, generated-asset inventory, and full-commit workflow/action pins. A
repository that added explicit capabilities must use its previously recorded
legacy request when constructing a `1.1.0` rollback candidate. Rollback never
mutates a published release or consumer default branch.
