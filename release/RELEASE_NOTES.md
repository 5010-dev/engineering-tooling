# Golden Path tooling 1.5.1

Stable patch implementation for the `2026.08.5` Developer Tooling Standard.
It corrects adapter availability and root-total routine PR budget semantics
without changing the `golden-path/v1` epoch, serialized dependency schemas,
report-only enforcement, or repository ownership boundaries.

Boundary classification: released — `1.5.0` remains an immutable distributed
implementation of Standard `2026.08.4`; this correction is published as a new
patch release and does not rewrite prior assets.

## Outcome

- A native root that resolves to more than one dependency-automation adapter
  ecosystem is a configuration error with exit `2`. Disjoint ecosystems may
  share a repository-relative path only as separate existing native roots.
- The compiler therefore applies the default or overridden routine PR budget
  once per native root instead of multiplying it across adapter blocks.
- The deterministic synthetic test covers a root that combines Go and Rust and
  proves the exact configuration-error boundary.
- Dependabot remains the only implemented adapter. Explicit `adapter: renovate`
  still exits `2` and is never interpreted as Dependabot; Standard `2026.08.5`
  now states that implementation precondition explicitly.
- Native roots, `.github/release-units.json`, native manifests and locks,
  repository-owned `just ci`, and canonical caller workflows remain untouched.
- No package-level map, dummy manifest, component-path impact inference,
  central CI registry, duplicate dependency manager, or central approval queue
  is introduced.
- `pending-classification` continues to stop only the affected routine root.
  Security visibility, fallback ownership, and conditional routing remain
  independent from routine budget or grouping.
- Central conformance remains checker-only and does not execute repository
  `just ci`. Synthetic fixtures remain distinct from live pilot evidence.

## Compatibility

- Standard: `2026.08.5` (`preferred`)
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.5.1`
- Dependency policy: `golden-path-dependency-policy/v1`
- Dependency defers: `golden-path-dependency-defers/v1`
- Dependency observation: `golden-path-dependency-observation/v1`
- Dependency candidate: `golden-path-dependency-candidate/v1`
- Dependency report: `golden-path-dependency-report/v1`
- Release manifest: `golden-path-release-manifest/v2`
- Enforcement: `report-only`

The `2026.08.5` snapshot is bound to organization-policy source commit
`631b0adb4ef605ed973a65b58180daf31b47b718`, source tree
`fad6a8213ac8a8ba718a2ff70dfe0c04c289f8c6`, catalog digest
`sha256:9e37b6c777206dc8a021102b9ff461eca55763d1c9f9b829a60c11e10091f55e`,
and aggregate digest
`sha256:1b96dc8239eaf5c22bc3f3006555a94a5c9a818b74b9658a87756c326a4ac1cd`.
The retained August 4 tooling cutoff is unchanged because external tools and
runtime selections did not move. The new source-integrity record binds the
`1.5.1` template bundle without altering historical records.

## Upgrade from 1.5.0

1. Verify the exact `1.5.1` release manifest, checksums, archive, source commit,
   tag, release workflow, and GitHub artifact attestations.
2. Run the ordinary upgrade preview into a separate empty candidate directory.
   Existing valid single-ecosystem roots require no repository-fact changes.
3. If one native root contains profiles that resolve to multiple adapter
   ecosystems, split them into separate existing native-root IDs. The roots may
   retain the same path; do not create package mappings or release units.
4. Preserve repository-owned manifests, locks, release units, dependency
   configuration, canonical commands, caller workflows, and security PRs.
5. Run repository `just ci` once through repository CI and structural
   conformance separately.

No durable state, production runtime, wire contract, or stored observation
migration is required. Consumers may remain on immutable `1.5.0` with Standard
`2026.08.4` until their normal maintenance window.

## Rollback

Restore the exact `1.5.0` managed Golden Path files only together with its
`2026.08.4` standard identity. Keep native manifests, locks, release units,
runtime state, durable data, security alerts, and independent security PRs
untouched. Never move the `v1.5.0` or `v1.5.1` tag or replace published assets.

## Evidence boundary

Local and synthetic tests prove deterministic compilation, configuration exit
semantics, schema validation, staging safety, and checker behavior. They do not
prove organization-wide queue state, live alerts, pilot repository CI, release
publication, production promotion, freeze exit, or a complete weekly cycle.
Those observations require separately source-bound live evidence.
