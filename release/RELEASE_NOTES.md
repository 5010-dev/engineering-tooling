# Golden Path tooling 1.6.0

Minor release candidate for the `2026.08.6` Developer Tooling Standard.
It closes the security-remediation scope gap without changing the
`golden-path/v1` epoch, repository ownership boundaries, or report-only central
enforcement.

Boundary classification: released — dependency observation/report v1 and
tooling `1.5.1` remain immutable distributed contracts. This correction uses
bounded observation/report v2 successors and a new tooling release rather than
rewriting prior artifacts.

## Outcome

- Observation v2 preserves GitHub alert relationship as `direct`, `transitive`,
  or `unknown`; central tooling does not infer it from component paths or locks.
- Report v2 groups every open alert by repository, advisory identity,
  ecosystem, and dependency, retains each alert number and path, carries
  source-bound closure runs from linked security pull requests, and reports
  pull-request association as `none`, `partial`, or `all-linked`.
- `all-linked` does not claim closure. Repository-owned native dependency graph
  proof remains authoritative for the exact integration head.
- `DT-DEP-012` fails when a security canonical gate omits a typed workflow/job
  reference and treats a broken declared reference as configuration error exit
  `2`. The compiler validates linkage but never runs the job or `just ci`.
- A synthetic fixture proves that linking only the direct alert while the same
  advisory remains transitive is `partial`. An unrelated advisory remains a
  separate group and is not a blocker for that remediation.
- Existing native roots, `.github/release-units.json`, native manifests and
  locks, repository-owned CI, routine budget, security routing, and
  `pending-classification` behavior remain unchanged.
- No package-level map, dummy manifest, component-path impact inference,
  cross-ecosystem lock resolver, central CI registry, duplicate dependency
  manager, global zero-alert gate, or central approval queue is introduced.

## Compatibility

- Standard: `2026.08.6` (`preferred`)
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.6.0`
- Dependency policy: `golden-path-dependency-policy/v1`
- Dependency defers: `golden-path-dependency-defers/v1`
- Dependency observation: `golden-path-dependency-observation/v2`
- Dependency candidate: `golden-path-dependency-candidate/v1`
- Dependency report: `golden-path-dependency-report/v2`
- Release manifest: `golden-path-release-manifest/v2`
- Enforcement: `report-only`

The `2026.08.6` snapshot is bound to the merged organization-policy source
commit `e782f90c2ca296db18c1896c0dc012e6cefea935`, source tree
`5e9d3bc62ffd3850069d6bfaf9ad902b4553b5ae`, catalog digest
`sha256:8e9910e19b2582f884af6170d70759dc23ac98d9c2d1bb3e2759d29495291355`,
and aggregate digest
`sha256:2eba72e63c71b0e236d85b29f98a4d4405b110a6acb2be5956f21b76363d2061`.
The retained August 4 tooling cutoff is unchanged because external tools and
runtime selections did not move. The new source-integrity record binds the
`1.6.0` template bundle without altering historical records.

## Upgrade from 1.5.1

1. Verify the exact `1.6.0` release manifest, checksums, archive, source commit,
   tag, release workflow, and GitHub artifact attestations.
2. Collect new live observations as v2, preserving the source-provided
   direct/transitive relationship and the full same-advisory alert set.
3. Reference the repository-owned conditional security closure workflow/job in
   the existing security `canonicalGate.ciEvidence`. Do not create a central
   command or package registry.
4. Run adoption preview into a separate empty candidate directory, then run
   repository `just ci` once through repository CI and structural conformance
   separately.
5. Treat a residual same-advisory graph path as partial, while allowing
   unrelated advisories to proceed independently. Verify expected alert numbers
   as fixed only after `main` promotion.

Historical sealed v1 observations and v1 reports remain immutable evidence with
their original tooling release. They do not require rewriting or a general
compatibility reader; recollect current live state as v2 when a new remediation
decision is made.

## Rollback

Restore the exact `1.5.1` managed Golden Path files only together with its
`2026.08.5` standard identity. Keep native manifests, locks, release units,
runtime state, durable data, security alerts, independent security PRs, and
historical observations untouched. Never move the `v1.5.1` or `v1.6.0` tag or
replace published assets.

## Evidence boundary

Local and synthetic tests prove deterministic grouping, alert-instance
coverage, typed workflow linkage, schema validation, configuration exit
semantics, staging safety, and checker behavior. They do not prove live pilot
dependency graphs, default-branch alert closure, release publication,
production promotion, freeze exit, or a complete weekly cycle. Those outcomes
require separately source-bound live evidence.
