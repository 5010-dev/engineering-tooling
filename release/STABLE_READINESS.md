# Stable minor release readiness

This is release evidence, not normative policy. The organization Developer
Tooling Standard remains authoritative for rule meaning and applicability.

## Release candidate identity

- Standard: `2026.08.4`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.5.0`
- Lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `416dec3053befbc5f54371bbb51950dd5cdee305`
- Normative source tree: `61b248457e0f2210da3acc4dd8138350bcee270e`
- Catalog digest: `sha256:ebe93eec9b095e8d6763207ffcac12b9836eb6bae366084a9f98d2cdbb4fddd9`
- Snapshot digest: `sha256:55cc033ca9df35837c6b5f2b7424128d6899cab02978ae90db8595538ee0f881`
- External-tool cutoff: `2026-08-04T12:54:32Z` (retained selections)
- Candidate source integrity: `2026-08-08T23:28:33Z`

[`source-integrity-2026-08-09.json`](./source-integrity-2026-08-09.json)
binds the current source-controlled locks and template bundle. Historical
cutoff and source-integrity records remain unchanged.

Boundary classification: unreleased — corrected in place.

There was no released tooling consumer, stored observation, or distributed
dependency v1 asset using the intermediate implementation. The first complete
tooling contract is therefore v1, with no compatibility reader or migration.
The public standard correction was published as `2026.08.4`; `2026.08.3` was
not rewritten.

## Required evidence before tagging

1. The 2026.08.4 snapshot is byte-complete and binds the exact organization
   commit, subtree, catalog digest, aggregate digest, and all five dependency
   schemas.
2. `just ci` succeeds from a clean checkout of the exact candidate commit.
3. Synthetic fixtures prove deterministic compilation for the three declared
   shapes, default and overridden bounded budgets, pending-classification,
   secure router preservation/generation, duplicate-adapter rejection, staging
   isolation, observation sealing, tamper rejection, and report generation.
4. Checker and CLI tests prove semantic exit `0/1/2/3` behavior. Shared
   conformance remains checker-only and never invokes repository `just ci`.
5. Release assembly publishes all dependency schemas, compatibility identity,
   source integrity, release notes, checksums, per-platform SBOMs, archive
   executable digests, and provenance attestations.
6. Existing 1.4.0 tags, release assets, standard snapshots, and public standard
   documents remain immutable.

The tag must not be created until these gates are green against the exact
source commit. Release URL, asset digests, release workflow identity, locator
pins, live pilot results, queue remediation, and weekly-cycle observation are
post-publication evidence and cannot be predeclared.

## Post-publication evidence

1. Verify all 1.5.0 assets and attestations against the exact released main
   commit and tag.
2. Advance the organization locator in a separate reviewed change that pins the
   released source and asset digests. Do not point consumers at a branch or
   sibling checkout.
3. Only then generate and review Design System, Collector, and Quant adoption
   candidates. Keep synthetic results separate from live pilot evidence.
4. Preserve security PRs independently while remediating only the routine
   queue supported by the fresh observation.
5. Record unfreeze evidence and observe one complete scheduled weekly cycle.

## Ownership and safety boundaries

The compiler consumes repository-owned native roots and release units. It does
not create a release-unit system, infer impact from component paths, replace
native manifests/locks, manage two dependency bots, centralize CI commands, or
execute repository quality gates. Preview output is external staging evidence;
live organization reports are source-bound operational evidence owned and
retained under the organization policy contract.

Security visibility and routing are independent of routine queue budgets.
Routine roots that cannot be classified remain stopped individually; their
uncertainty does not block security remediation or justify a repository-wide
approval queue.

## Rollback

Restore the immutable 1.4.0 managed baseline and repository-owned dependency
changes from Git history. Do not delete or rewrite runtime state, durable data,
alerts, security PRs, native manifests, locks, or release-unit authorities.
Never mix 1.4.0 callers with 1.5.0 release inputs, move a published tag, or
replace release assets.
