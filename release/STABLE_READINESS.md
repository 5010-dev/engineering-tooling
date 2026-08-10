# Stable minor release readiness

This is release evidence, not normative policy. The organization Developer
Tooling Standard remains authoritative for rule meaning and applicability.

## Release candidate identity

- Standard: `2026.08.6`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.6.0`
- Lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `37babbe962fa516b850d6740d8bf2cd9512d7a70`
- Normative source tree: `08ae4fc8107e4a1794cee228d4d1e2ee09e2a2fa`
- Catalog digest: `sha256:e1dadf8ecb1367bf0b1e85c12600dd4c9dc702c651286ea0d0a678cb5dd802f1`
- Snapshot digest: `sha256:f0a31981819b8de04b1dbaefcf9cf77cc4a0ab9f07a3356f7359ec6024bd1c3a`
- External-tool cutoff: `2026-08-04T12:54:32Z` (retained selections)
- Candidate source integrity: `2026-08-10T15:02:07Z`

[`source-integrity-2026-08-10-1.6.0.json`](./source-integrity-2026-08-10-1.6.0.json)
binds current source-controlled locks and the `1.6.0` template bundle.
Historical cutoff and source-integrity records remain unchanged.

Boundary classification: released — observation/report v1, tooling `1.5.1`,
and Standard `2026.08.5` remain immutable. The correction will be published
through Standard `2026.08.6`, observation/report v2, and tooling `1.6.0` only
after the required evidence below is complete.

The normative source identity above is provisional while organization PR #30
is open. Before this candidate can be committed as release-ready or tagged, the
snapshot and all release evidence must be rebound to the exact merged `main`
commit and revalidated.

## Required evidence before tagging

1. The `2026.08.6` normative source is merged, and the bundled snapshot binds
   that exact organization commit, subtree, catalog digest, aggregate digest,
   and all dependency schemas.
2. `just ci` succeeds from a clean checkout of the exact candidate commit.
3. A deterministic synthetic observation proves direct-only remediation with a
   same-advisory transitive residual is `partial`, while an unrelated advisory
   remains independent.
4. Compiler fixtures prove missing security closure evidence fails
   `DT-DEP-012`, a broken workflow/job reference exits `2`, and security routing
   remains present behind the finding.
5. Existing compiler fixtures still prove bounded budgets,
   `pending-classification`, security-router independence, unsupported Renovate
   rejection, duplicate-manager rejection, staging isolation, observation
   sealing, tamper rejection, and report source binding.
6. Checker and CLI tests prove semantic exit `0/1/2/3` behavior. Shared
   conformance remains checker-only and never invokes repository `just ci`.
7. Release assembly publishes the snapshot, dependency schemas, compatibility
   identity, source integrity, release notes, checksums, per-platform SBOMs,
   executable digests, and provenance attestations.
8. Existing `1.4.0`, `1.5.0`, and `1.5.1` tags, release assets, snapshots, and
   public standard history remain immutable.

The tag must not be created until these gates are green against the exact
source commit. Release URL, asset digests, release workflow identity, locator
pins, live pilot results, queue remediation, freeze exit, and weekly-cycle
observation are post-publication evidence and cannot be predeclared.

## Post-publication evidence

1. Verify all `1.6.0` assets and attestations against the exact released `main`
   commit and tag.
2. Advance the organization locator in a separate reviewed change that pins
   released source and asset digests.
3. Only then regenerate Design System, Collector, and Quant candidates against
   their exact current `dev` heads. Keep synthetic results separate from live
   pilot evidence.
4. Preserve security visibility and routing while remediating only the routine
   queue supported by the fresh observation.
5. Do not promote pilot `dev` branches to `main`, exit the bounded freeze, or
   claim the required weekly cycle without separate evidence and authority.

## Ownership and safety boundaries

The compiler consumes repository-owned native roots and release units. It does
not create a release-unit system, infer impact from component paths, replace
native manifests or locks, manage two dependency bots, centralize CI commands,
interpret cross-ecosystem lock graphs, or execute repository quality gates.

Security visibility and routing remain independent from routine queue budgets.
The central report preserves alert identity and relationship but remains
report-only; repository-owned native graph proof controls remediation closure.

## Rollback

Restore immutable `1.5.1` managed files together with Standard `2026.08.5`.
Do not delete or rewrite runtime state, durable data, alerts, security PRs,
native manifests, locks, release-unit authorities, tags, observations, or
release assets.
