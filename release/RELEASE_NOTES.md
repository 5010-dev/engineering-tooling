# Golden Path tooling 1.6.1

Patch release for the `2026.08.6` Developer Tooling Standard. It corrects the
repository gate-reference resolver without changing the `golden-path/v1`
contract, dependency schemas, repository ownership boundaries, or report-only
central enforcement.

Boundary classification: released — tooling `1.6.0` is an immutable distributed
contract. This correction is published as `1.6.1`; the `v1.6.0` tag and assets
remain unchanged.

## Outcome

- Canonical `just ci` validation follows bounded, repository-relative Just
  imports instead of requiring the recipe to be declared directly in the root
  justfile. The resolver and command checker share the same lexical handling
  for LF/CRLF input, single- or double-quoted imports, escaped double-quoted
  paths, optional imports, and quiet recipe headers.
- Required imports that are missing, escape the repository, or exceed the
  bounded import graph remain configuration errors. Optional missing imports
  remain optional.
- The resolver parses references only. It does not execute Just, repository CI,
  or the security closure job.
- Regression fixtures cover the imported canonical gate used by the Design
  System candidate shape, CRLF missing-import failure, legal quoted imports,
  quiet recipes, absolute-path rejection, and recipe-body command isolation.
- Existing native roots, release units, native manifests and locks, routine
  budgets, security routing, conditional closure evidence, and
  `pending-classification` behavior remain unchanged.

## Compatibility

- Standard: `2026.08.6` (`preferred`)
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.6.1`
- Dependency policy: `golden-path-dependency-policy/v1`
- Dependency observation: `golden-path-dependency-observation/v2`
- Dependency report: `golden-path-dependency-report/v2`
- Release manifest: `golden-path-release-manifest/v2`
- Enforcement: `report-only`

The normative source binding, Standard snapshot, catalog digest, runtime
selections, and retained August 4 tooling cutoff are unchanged from `1.6.0`.
The new source-integrity record binds the `1.6.1` template bundle and current
source-controlled locks without rewriting historical evidence.

## Upgrade from 1.6.0

1. Verify the exact `1.6.1` release manifest, checksums, archive, source commit,
   tag, release workflow, and GitHub artifact attestations.
2. Advance the organization locator in a separately reviewed change.
3. Regenerate an adoption candidate into a separate empty directory and review
   the managed diff.
4. Keep repository-owned `just ci`, native graphs, security routing, and
   conditional closure jobs authoritative.

No policy, observation, report, root, release-unit, or lock migration is
required. Consumers already using a directly declared root `ci` recipe remain
compatible.

## Rollback

Restore the exact `1.6.0` managed Golden Path files together with Standard
`2026.08.6`. Do not move either release tag or replace published assets. Keep
native manifests, locks, release units, alerts, security pull requests, runtime
state, and historical observations untouched.

## Evidence boundary

Local and synthetic tests prove import resolution, missing-import failure,
existing compiler semantics, release assembly, and the Design System pilot
shape. They do not prove release publication, pilot workflow success,
default-branch alert closure, production promotion, freeze exit, or a complete
weekly cycle.
