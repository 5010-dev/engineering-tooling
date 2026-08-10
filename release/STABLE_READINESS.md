# Stable patch release readiness

This is release evidence, not normative policy. The organization Developer
Tooling Standard remains authoritative for rule meaning and applicability.

## Release candidate identity

- Standard: `2026.08.6`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.6.1`
- Lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `e782f90c2ca296db18c1896c0dc012e6cefea935`
- Normative source tree: `5e9d3bc62ffd3850069d6bfaf9ad902b4553b5ae`
- Catalog digest: `sha256:8e9910e19b2582f884af6170d70759dc23ac98d9c2d1bb3e2759d29495291355`
- Snapshot digest: `sha256:2eba72e63c71b0e236d85b29f98a4d4405b110a6acb2be5956f21b76363d2061`
- External-tool cutoff: `2026-08-04T12:54:32Z` (retained selections)
- Candidate source integrity: `2026-08-10T19:15:00Z`

[`source-integrity-2026-08-10-1.6.1.json`](./source-integrity-2026-08-10-1.6.1.json)
binds current source-controlled locks and the `1.6.1` template bundle.
Historical cutoff, snapshot, source-integrity, tag, and release records remain
unchanged.

Boundary classification: released — tooling `1.6.0` is already distributed.
The import-resolution correction must be published only as a new immutable
`1.6.1` release.

## Required evidence before tagging

1. Regression tests prove that a root justfile can expose canonical `ci`
   through bounded repository-relative imports and that a missing required
   import fails as configuration error.
2. The modified checker evaluates the real Design System pilot policy as
   complete while retaining its two routine surfaces as
   `pending-classification`.
3. `just ci` succeeds from a clean checkout of the exact candidate commit.
4. Existing compiler, observation, report, generator, checker, and release
   tests remain green.
5. Release assembly binds the unchanged Standard snapshot and schemas plus the
   new checker, generator bundle, source integrity, release notes, checksums,
   SBOMs, executable digests, and provenance attestations.
6. Existing `1.4.0`, `1.5.0`, `1.5.1`, and `1.6.0` tags, release assets,
   snapshots, and public standard history remain immutable.

The tag must not be created until these gates are green against the exact
merged source commit. Release URL, asset digests, release workflow identity,
locator advance, live pilot runs, queue remediation, freeze exit, and the
weekly-cycle observation are post-publication evidence and cannot be
predeclared.

## Post-publication evidence

1. Verify all `1.6.1` assets and attestations against the exact released `main`
   commit and tag.
2. Advance the organization locator in a separate reviewed change that pins
   released source and asset digests.
3. Only then regenerate the Design System candidate against its exact current
   `dev` head and run repository-owned validation.
4. Preserve security visibility and routing; do not promote the pilot, exit the
   routine freeze, or claim a weekly cycle without separate evidence and
   authority.

## Ownership and safety boundaries

The resolver reads a bounded Just import graph only to validate the typed
canonical reference. It does not run `just ci`, resolve native dependency
graphs, infer release impact, create release units, duplicate dependency
managers, or establish a central approval queue.
