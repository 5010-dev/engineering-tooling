# Stable release readiness

This document is release evidence, not normative policy. The organization
Developer Tooling Standard remains the authority for rule meaning and
applicability.

## Release candidate identity

- Standard: `2026.07`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.0.0`
- Release lifecycle: `stable`
- Enforcement: `report-only`
- External-tool cutoff: `2026-08-01T11:21:19Z`

The existing CalVer remains correct because no normative rule, profile,
contract, or schema meaning changes in this implementation release. The exact
tool selections are release data and are bound by
[`tooling-cutoff-2026-08-01.json`](./tooling-cutoff-2026-08-01.json).

## Required evidence before tagging

1. `just ci` succeeds from a clean checkout of the candidate source.
2. Every generated profile runs on its declared native CI runner.
3. The setup action and reusable workflow verify released `0.2.0` provenance,
   proving the rollback source remains consumable.
4. A repository materialized by the verified `0.2.0` release upgrades to a
   separate `1.0.0` candidate without conflicts or source mutation, and the
   verified rollback binary can produce a separate `0.2.0` candidate from it
   without source mutation. The rollback candidate passes its locked `just init`
   and `just ci` contract.
5. Positive, negative, exception, unsupported-profile, malformed-input, and
   artifact power-set fixtures retain their expected exit semantics.
6. The public implementation path works for a GitHub Free private consumer
   without paid hosting controls or consumer secrets.
7. SBOMs bind every archive digest; release attestations cover every published
   asset; the release manifest binds source, catalog, snapshot, templates,
   automation, cutoff, binaries, and supported targets.
8. A separately approved bounded set of representative repositories or exports
   is checked read-only. Their identities and results remain operational
   evidence and are not copied into normative policy.

The stable tag must not be created until all eight gates have evidence against
the exact source commit. The final source SHA, asset digests, workflow run, and
release URL are post-publication evidence and cannot be predeclared in this
candidate.

## GitHub Free private baseline

The checker needs no network, GitHub API, credentials, organization secret,
protected branch, ruleset, required check, Environment, Dependency Review, or
private artifact attestation. The reusable workflow preserves findings while
converting only conformance exit `1` to a report-only job result. Configuration
and internal errors (`2` and `3`) still fail closed. Paid adapters may strengthen
merge or review enforcement but do not change the release contract.

The `1.0.0` channel contains only public generic implementation assets. It does
not contain, depend on, or claim support for a restricted implementation. A
future restricted implementation must use a separate trust and release
boundary and preserve equivalent immutable identity, integrity, and rollback
evidence. Its existence or visibility cannot become a conformance requirement,
and the public baseline cannot acquire private-package credentials merely to
support it.

## CI cost boundary

Central pull-request validation expands to 11 GitHub-hosted jobs: one repository
gate, four native targets, four generated-profile targets, one rollback and
migration job, and one reusable-workflow integration job. The maximum configured
aggregate runner time is 265 minutes, the longest individual timeout is 40
minutes, and the matrix runs only in the public implementation repository. The
tag-only release path expands to seven jobs with 95 aggregate timeout-minutes.
A private consumer invokes one reusable job with a 20-minute timeout; it does
not inherit the central native or generated-profile matrices.

## Adoption boundary

New-repository materialization records tooling bundle `1.0.0`, emits a thin
report-only caller, and writes only to an explicitly selected empty staging
directory. Existing repositories are not discovered, enrolled, or changed by
this release. A read-only result against a repository without explicit Golden
Path metadata is a configuration result, not authorization to infer metadata,
open a migration pull request, or add a required check.

## Support, deprecation, and rollback

`1.x` is the supported stable implementation line. The 2026-08-01 announcement
deprecates `0.1.x` and `0.2.x` through 2027-01-28. `0.2.0` is the verified
rollback and migration source; `0.1.x` must first advance to `0.2.0` before the
supported `1.x` migration. Rollback restores all of the following as one
identity:

- exact binary release and archive checksum;
- matching release and compatibility manifests;
- matching template/materialization version; and
- matching reusable workflow and setup-action full commit.

Rollback never mutates a published release or consumer default branch. Any
materialized file change is reviewed as a candidate diff owned by the consumer.
