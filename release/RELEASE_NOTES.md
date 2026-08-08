# Golden Path tooling 1.5.0

Stable implementation release for the `2026.08.4` Developer Tooling Standard.
It adds the repository-fact dependency operations contract without changing the
`golden-path/v1` epoch, the report-only enforcement boundary, or repository
ownership of native dependency graphs, release units, manifests, locks, and
canonical quality CI.

Boundary classification: unreleased — corrected in place.

The dependency implementation was completed as one v1 contract before its
first tooling release. No intermediate dependency wire shape, migration,
dual-reader, mixed-version rollout, or compatibility adapter is published.
The organization standard's incomplete `2026.08.3` observation shape remains
immutable; `2026.08.4` is the authoritative bounded correction imported here.

## Outcome

- `golden-path dependency check` performs semantic, read-only validation with
  stable exit meanings: `0` aligned, `1` reviewable policy drift, `2`
  repository declaration/configuration error, and `3` tooling failure.
- `dependency preview` and its `compile` alias produce a deterministic candidate
  and may write only to an explicitly selected separate empty staging
  directory. They never mutate the source repository.
- Root bindings reference existing `.github/golden-path-native-roots.yaml` IDs
  and repository-owned `.github/release-units.json` IDs. The compiler neither
  creates release units nor infers impact from component paths.
- Canonical gates are typed references to repository `just ci` and optional
  GitHub Actions workflow/job evidence. Structural conformance verifies those
  references and never re-executes repository `just ci`.
- The default routine PR budget is three per classified root. A root override
  requires a positive value plus a reason, owner, and review date. Unknown or
  unmatched surfaces stay `pending-classification` with a compiled budget of
  zero.
- Existing secure Dependabot routers are preserved. Where a `main` release
  branch and `dev` integration branch differ, the candidate conditionally emits
  a guarded same-repository Dependabot security router independent of routine
  budget findings.
- Explicit Dependabot/Renovate manager overlap is rejected. No central approval
  queue, CI command registry, hand-maintained package map, dummy manifest, or
  second dependency manager is introduced.
- The 1.5.0 compiler implements the organization-default Dependabot adapter.
  A repository that explicitly selects Renovate receives configuration exit
  `2`; it is never silently interpreted through Dependabot semantics.
- Sealed observations bind observation time, query scope, repository/default
  branch identity, PR refs and checks, alerts, native-manager evidence, source
  identity, and SHA-256. Organization reports retain that identity and remain
  distinct from synthetic compiler fixtures.
- Three synthetic repository fixtures cover a polyglot multi-unit repository,
  a package-workspace publisher, and a single OCI service. These fixtures prove
  compiler behavior only; they are not live organization or pilot evidence.
- New bootstrap repositories start routine Dependabot lanes at zero with an
  empty root-binding policy. Repository owners must classify actual native
  roots before enabling a routine budget. Security handling is not coupled to
  that routine state.

## Compatibility

- Standard: `2026.08.4` (`preferred`)
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.5.0`
- Dependency policy: `golden-path-dependency-policy/v1`
- Dependency defers: `golden-path-dependency-defers/v1`
- Dependency observation: `golden-path-dependency-observation/v1`
- Dependency candidate: `golden-path-dependency-candidate/v1`
- Dependency report: `golden-path-dependency-report/v1`
- Release manifest: `golden-path-release-manifest/v2`
- Enforcement: `report-only`

The `2026.08.4` snapshot is bound to organization-policy source commit
`416dec3053befbc5f54371bbb51950dd5cdee305`, source tree
`61b248457e0f2210da3acc4dd8138350bcee270e`, catalog digest
`sha256:ebe93eec9b095e8d6763207ffcac12b9836eb6bae366084a9f98d2cdbb4fddd9`,
and aggregate digest
`sha256:55cc033ca9df35837c6b5f2b7424128d6899cab02978ae90db8595538ee0f881`.
The retained August 4 tooling cutoff is unchanged because external tool and
runtime selections did not move. The August 9 source-integrity record binds the
current 1.5.0 lock and template bytes without rewriting earlier evidence.

## Upgrade from 1.4.0

1. Verify the exact 1.5.0 release manifest, checksums, archive, source commit,
   tag, release workflow, and GitHub artifact attestations.
2. Run the ordinary Golden Path adoption or upgrade preview into a separate
   empty candidate directory. Preserve repository-owned manifests, locks,
   native roots, release units, Dependabot configuration, and CI.
3. Add a repository-owned dependency policy only from confirmed repository
   facts. Leave an unclear routine root pending with budget zero; do not invent
   release units or package mappings.
4. Run `golden-path dependency preview` and review its separate staged output.
   Apply only the approved repository-owned diff.
5. Run the repository's `just ci` once in repository CI and run structural
   conformance separately. The shared conformance workflow does not run it.

No durable state or production runtime migration is required. Existing 1.4.0
consumers may remain on their immutable release until their normal maintenance
window; a central locator change does not mutate them.

## Rollback

Restore the exact 1.4.0 managed Golden Path files and dependency policy/adapter
changes from repository history. Keep native manifests, locks, release units,
runtime state, durable data, security alerts, and independent security PRs
untouched. Never move the `v1.4.0` or `v1.5.0` tag or replace published assets.

## Evidence boundary

Local and synthetic tests prove deterministic compilation, schema validation,
staging safety, and checker semantics. They do not prove GitHub settings,
organization-wide queue state, live alerts, pilot repository CI, release
publication, deployment, production capacity, or a weekly operating cycle.
Those observations require separately source-bound live evidence.
