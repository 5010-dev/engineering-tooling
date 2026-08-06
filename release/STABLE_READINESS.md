# Stable patch release readiness

This document is release evidence, not normative policy. The organization
Developer Tooling Standard remains the authority for rule meaning and
applicability.

## Release candidate identity

- Standard: `2026.08`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.2.3`
- Release lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `604a40886d5a1cba3b304471e6e072b72cec7601`
- External-tool cutoff: `2026-08-04T12:54:32Z`

The patch increment corrects the released `1.2.2` reusable workflow's Just
bootstrap and release-qualification coverage. Request, metadata, and result
schemas remain unchanged;
no contract epoch, compatibility reader, or repository migration is introduced.
Runtime selections, external tools, generated dependency locks, workflow
actions, and the checker rule catalog remain unchanged. Release-specific
integrity evidence is bound by
[`tooling-cutoff-2026-08-04.json`](./tooling-cutoff-2026-08-04.json).

The candidate must additionally prove that the reusable workflow installs both
the caller-selected exact GitHub CLI and release-pinned Just from recorded
checksums under strict Mise lock enforcement. It must invoke the resolved Just
directly, put it first on `PATH` for nested calls, install the consumer
toolchain from the selected build root, and reject ambient runner Just through
an executed guard. The acceptance matrix must exercise both a generated
bootstrap repository and an adoption repository whose local Mise configuration
and lock omit Just.

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
   source-preserving, and materializes the expected candidate metadata. An
   explicitly empty materialization mode is rejected, and both decoded and
   direct-render requests validate derived Go module paths.
6. The published request target definition matches the normative metadata
   target schema, preserves explicit `execution: false`, sorts deterministically,
   requires a primary or secondary representative, and rejects missing, empty,
   repeated-identity, duplicate, or invalid declarations.
7. Adoption renders exactly the fixed five managed control-plane assets plus an
   out-of-inventory staging plan, records only explicit capabilities (including
   an explicit empty set), does not invent a Go module path, and establishes an
   upgradeable baseline without tracking or mutating repository-owned source,
   manifests, locks, commands, or operational contracts. Bootstrap-to-adoption
   planning records retired generated assets and refuses to stage customized
   retirement conflicts.
8. The setup action verifies released `0.2.0`, `1.1.0`, `1.2.0`, `1.2.1`, and
   `1.2.2` provenance. Projects materialized by all five releases upgrade to a
   separate `1.2.3` candidate and roll back without source mutation or
   unresolved conflicts.
9. Every generated profile runs on its declared native CI runner. The executed
   reusable-workflow matrix covers an actual `1.1.0` generated bootstrap
   repository and a conformant `1.2.2` adoption repository whose
   repository-local Mise files omit Just. Exact GitHub CLI and Just resolution,
   hostile ambient-tool guards, nested Just calls, and selected consumer
   working-directory installation all pass before conformance evidence is
   published.
10. SBOMs bind every archive digest; release attestations cover every published
   asset; the release manifest binds source, catalog, snapshot, templates,
   automation, cutoff, binaries, schemas, and supported targets.
11. A separately approved read-only review confirms the complete candidate diff
   preserves the implementation, policy, migration, and release boundaries.

The stable tag must not be created until all eleven gates have evidence against
the exact source commit. Final source SHA, asset digests, workflow run, release
URL, and bootstrap-locator pins are post-publication evidence and cannot be
predeclared.

## Compatibility and ownership boundary

The new request fields are optional. Existing repositories do not need to
change their request, metadata, or generated files unless they adopt `1.2.3`
through their normal migration workflow. A component declares only capabilities
its repository actually implements. Bootstrap merges declarations with its
materialized baseline; adoption records exactly the declaration. The generator
does not infer publication or platform support from artifact type, profile, or
runner and does not create publishing automation, credentials, registry
configuration, or release evidence.

This is a behavior correction for the released `1.2.2` implementation. Exact
consumer pins move to the separate immutable `1.2.3` release; no old data or
request form requires dual parsing, dual writing, or a compatibility path.

## GitHub Free private baseline

The checker needs no network, GitHub API, credentials, organization secret,
protected branch, ruleset, required check, Environment, Dependency Review, or
private artifact attestation. The reusable workflow preserves findings while
converting only conformance exit `1` to a report-only job result. Configuration
and internal errors (`2` and `3`) still fail closed.

The `1.2.3` channel contains only public generic implementation assets. It does
not contain, depend on, or claim support for a restricted implementation.

## CI cost boundary

Central pull-request validation uses 16 GitHub-hosted jobs: one repository gate,
four native targets, four generated-profile targets, five migration and
rollback matrix entries, and two reusable-workflow integration matrix entries.
The maximum configured aggregate runner time is 325 minutes, the longest
individual timeout remains 40 minutes, and the matrix runs only in the public
implementation repository. The tag-only release path remains seven jobs with
95 aggregate timeout-minutes.

## Adoption boundary

New-repository bootstrap records tooling bundle `1.2.3`, emits the complete
starter and a thin report-only caller, and writes only to an explicitly selected
empty staging directory. First adoption for an existing repository emits only
the canonical request, metadata, generated-asset baseline, immutable bootstrap
script, and thin caller. Existing repositories are not discovered, enrolled, or
changed. Integrating the fixed adoption candidate, its explicit capabilities,
and its explicit targets is repository-owned work based on actual behavior and
support claims, not a central migration side effect. Normal `upgrade` begins
only after that baseline is committed.

After publication, the organization bootstrap locator is updated in a separate
policy-repository change using the exact release source commit, manifest and
snapshot-manifest hashes, archive hashes, and immutable automation pins.

## Support, deprecation, and rollback

`1.x` remains the supported stable implementation line. `1.2.2` is the
immediate immutable rollback source. The 2026-08-01 announcement continues to
deprecate `0.1.x` and `0.2.x` through 2027-01-28; `0.2.0` remains a verified
legacy migration source, while `0.1.x` must first advance to `0.2.0`.

Rollback restores the exact binary, release manifest, archive and executable
checksums, generated-asset inventory, and full-commit workflow/action pins.
Rollback to `1.2.2` retains the same request and metadata schema and never
mutates a published release or consumer default branch.
