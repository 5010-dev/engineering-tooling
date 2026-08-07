# Stable minor release readiness

This document is release evidence, not normative policy. The organization
Developer Tooling Standard remains the authority for rule meaning and
applicability.

## Release candidate identity

- Standard: `2026.08.2`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.4.0`
- Release lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `58f48c7f29a567f201c4302c3b81d34051c2d1ff`
- Normative source tree: `62e80b0920f75528f6b3d8d6c3bd82f27e928ecd`
- External-tool cutoff: `2026-08-04T12:54:32Z` (retained selections)
- Candidate source integrity: `2026-08-07` (current release bytes)

This compatible minor release completes repository bootstrap and establishes a
bounded ownership handoff for previously generated scaffold. It keeps all v1
serialized contracts and the checker-only conformance workflow. The immutable
1.2.4
[`tooling-cutoff-2026-08-04.json`](./tooling-cutoff-2026-08-04.json) retains
the external-tool selections reused by this release. The separate
[`source-integrity-2026-08-07.json`](./source-integrity-2026-08-07.json)
binds the current locks and bundle identity without modifying historical
evidence.

Boundary classification: released — compatibility is required because 1.3.0
published full-scaffold inventories must upgrade without overwriting
repository-owned files.

## Required evidence before tagging

1. The accepted 2026.08.2 standard has an immutable organization-policy source
   commit and subtree. The imported 12-file snapshot must be byte-identical,
   complete, and bind source commit, subtree, catalog digest, and aggregate
   digest together.
2. `just ci` succeeds from a clean checkout of the exact candidate source.
3. Bootstrap fixtures for every supported generated profile prove root
   onboarding, pinned repository quality CI, Dependabot targeting `dev`,
   staged and unstaged whitespace checks, and no staged plan file.
4. Generator tests prove the managed inventory contains only the request,
   metadata, inventory, conformance caller, and bootstrap script, with the
   inventory tracked implicitly rather than self-hashed.
5. Upgrade tests prove repository-owned scaffold changes are preserved,
   managed-file byte and mode drift still conflicts, and a released 1.3.0
   full-scaffold inventory hands off to the bounded managed set without source
   mutation.
6. Adoption fixtures retain their fixed control-plane output and target
   requirement. Bootstrap may omit targets, while every declared target set
   still contains a primary or secondary production or release representative.
7. The attestation-verified migration and rollback matrix covers every released
   boundary from 0.2.0 through 1.3.0, including `just init` and `just ci`
   under the selected prior release after rollback.
8. Every release archive is built and executed on its native platform. SBOMs
   bind archive digests; attestations cover every published asset; the release
   manifest binds source, standard snapshot, catalog, templates, automation,
   compatibility, retained tool cutoff, schemas, and supported targets.
   Release checksums and attestations bind the separate current
   source-integrity record.

The stable tag must not be created until these gates and an independent review
are green against the exact source commit. Final asset digests, release URL,
release-run identity, smoke runs, and organization locator pins are
post-publication evidence and cannot be predeclared.

## Post-publication evidence

Before advancing the organization locator:

1. The push or dispatch run at the exact released `main` commit must exercise
   the reusable conformance workflow through attestation-verified 1.4.0 release
   acquisition.
2. The organization locator pull request must pin the exact 1.4.0 source commit
   and archive digests, then run the released bootstrap fixture end to end.
3. The generated repository quality workflow and the structural conformance
   caller must remain separate owners: the former runs `just ci` once; the
   latter runs only the checker.
4. If either smoke fails, leave the organization locator on 1.3.0 and publish a
   corrective release. Never replace 1.4.0 assets or move its tag.

## Compatibility and support boundary

1.3.0 remains an immutable supported release. Advancing the organization
locator to 1.4.0 does not force immediate consumer pull requests. Exact source
pins keep existing 1.x workflows and setup actions on their published bytes.

The only released-state compatibility work in this candidate is the bounded
1.3.0 inventory handoff and the existing migration/rollback matrix. No wire
version, dual reader, dual workflow, deployment rollout, runtime-state
migration, or open-ended compatibility framework is introduced.

## Quality and conformance ownership

A bootstrapped repository owns `.github/workflows/quality.yml`; it installs
the pinned environment and runs `just init` followed by `just ci`. The
generated `.github/workflows/developer-tooling.yml` remains a thin caller of
the immutable checker-only reusable workflow. These are distinct signals and
must not execute the same quality graph twice.

The public tooling repository retains the broader native-platform, generated
profile, migration, rollback, packaging, schema, and provenance matrix needed
to publish the shared release. Consumer repositories do not inherit that
release-owner evidence burden.

## Rollback

Restore the exact 1.3.0 managed control-plane baseline from repository history:
request, metadata, generated asset inventory, bootstrap script, binary
identity, and full-commit workflow pin. Repository-owned scaffold and runtime
or durable state are outside this rollback.

Never mix a 1.4.0 caller or setup action with 1.3.0 release inputs, move a
published tag, or replace release assets.
