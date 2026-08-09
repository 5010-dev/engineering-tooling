# Stable patch release readiness

This is release evidence, not normative policy. The organization Developer
Tooling Standard remains authoritative for rule meaning and applicability.

## Release candidate identity

- Standard: `2026.08.5`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.5.1`
- Lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `631b0adb4ef605ed973a65b58180daf31b47b718`
- Normative source tree: `fad6a8213ac8a8ba718a2ff70dfe0c04c289f8c6`
- Catalog digest: `sha256:9e37b6c777206dc8a021102b9ff461eca55763d1c9f9b829a60c11e10091f55e`
- Snapshot digest: `sha256:1b96dc8239eaf5c22bc3f3006555a94a5c9a818b74b9658a87756c326a4ac1cd`
- External-tool cutoff: `2026-08-04T12:54:32Z` (retained selections)
- Candidate source integrity: `2026-08-09T10:57:30Z`

[`source-integrity-2026-08-09-1.5.1.json`](./source-integrity-2026-08-09-1.5.1.json)
binds current source-controlled locks and the `1.5.1` template bundle.
Historical cutoff and source-integrity records remain unchanged.

Boundary classification: released — `1.5.0` and Standard `2026.08.4` remain
immutable; the correction is published through Standard `2026.08.5` and a new
`1.5.1` release.

## Required evidence before tagging

1. The `2026.08.5` snapshot is byte-complete and binds the exact organization
   commit, subtree, catalog digest, aggregate digest, and dependency schemas.
2. `just ci` succeeds from a clean checkout of the exact candidate commit.
3. A deterministic synthetic test proves that one root cannot multiply its
   routine PR budget across Go and Rust adapter ecosystems and receives exit
   `2` with the separate-root remediation.
4. Existing compiler fixtures still prove bounded budgets,
   `pending-classification`, security-router independence, unsupported
   Renovate rejection, duplicate-manager rejection, staging isolation,
   observation sealing, tamper rejection, and report generation.
5. Checker and CLI tests prove semantic exit `0/1/2/3` behavior. Shared
   conformance remains checker-only and never invokes repository `just ci`.
6. Release assembly publishes the snapshot, dependency schemas, compatibility
   identity, source integrity, release notes, checksums, per-platform SBOMs,
   executable digests, and provenance attestations.
7. Existing `1.4.0` and `1.5.0` tags, release assets, standard snapshots, and
   public standard history remain immutable.

The tag must not be created until these gates are green against the exact
source commit. Release URL, asset digests, release workflow identity, locator
pins, live pilot results, queue remediation, freeze exit, and weekly-cycle
observation are post-publication evidence and cannot be predeclared.

## Post-publication evidence

1. Verify all `1.5.1` assets and attestations against the exact released main
   commit and tag.
2. Advance the organization locator in a separate reviewed change that pins
   released source and asset digests.
3. Only then regenerate Design System, Collector, and Quant candidates against
   their current `dev` heads. Keep synthetic results separate from live pilot
   evidence.
4. Preserve security PRs independently while remediating only the routine queue
   supported by the fresh observation.
5. Do not promote pilot `dev` branches to `main`, exit the bounded freeze, or
   claim the required weekly cycle without separate evidence and authority.

## Ownership and safety boundaries

The compiler consumes repository-owned native roots and release units. It does
not create a release-unit system, infer impact from component paths, replace
native manifests or locks, manage two dependency bots, centralize CI commands,
or execute repository quality gates. A multi-ecosystem root is a configuration
error; disjoint roots may use the same path.

Security visibility and routing remain independent from routine queue budgets.
Routine roots that cannot be classified stay stopped individually.

## Rollback

Restore the immutable `1.5.0` managed baseline together with its `2026.08.4`
identity. Do not delete or rewrite runtime state, durable data, alerts, security
PRs, native manifests, locks, release-unit authorities, tags, or release assets.
