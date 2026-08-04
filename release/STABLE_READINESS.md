# Stable minor release readiness

This document is release evidence, not normative policy. The organization
Developer Tooling Standard remains the authority for rule meaning and
applicability.

## Release candidate identity

- Standard: `2026.08`
- Contract: `golden-path/v1`
- Tooling and asset bundle: `1.1.0`
- Release lifecycle: `stable`
- Enforcement: `report-only`
- Normative source commit: `604a40886d5a1cba3b304471e6e072b72cec7601`
- External-tool cutoff: `2026-08-04T12:54:32Z`

The minor increment is required because the checker adds the optional
`golden-path-native-roots/v1` input and new backward-compatible multi-root
evaluation behavior. Runtime and executable tool selections remain unchanged.
The checker adds exact `github.com/hashicorp/hcl/v2` `2.24.0` parsing support
for Terraform/OpenTofu module declarations; all release-specific integrity
evidence is bound by
[`tooling-cutoff-2026-08-04.json`](./tooling-cutoff-2026-08-04.json).

## Required evidence before tagging

1. `just ci` succeeds from a clean checkout of the candidate source.
2. The bundled `2026.08` snapshot is byte-identical to the machine-readable
   rules and schemas at organization-policy commit
   `604a40886d5a1cba3b304471e6e072b72cec7601`; its source tree, complete file
   inventory, catalog digest, and aggregate digest verify together.
3. End-to-end checker tests prove polyglot same-path roots, independent
   same-ecosystem graphs, invalid-sidecar exit `2`, no-sidecar compatibility,
   profile coverage, overlap rejection, profile-specific exception isolation,
   committed Go workspace module coverage, structural IaC manifest/lock/module
   checks, and repository-root marker probing.
4. Every generated profile runs on its declared native CI runner, including a
   hostile developer-global mise tool that is absent from the repository lock.
5. The setup action verifies released `0.2.0` and `1.0.1` provenance. Projects
   materialized by both releases upgrade to a separate `1.1.0` candidate and
   roll back without source mutation or unresolved conflicts.
6. Positive, negative, exception, unsupported-profile, malformed-input, and
   artifact power-set fixtures retain their expected exit semantics.
7. The public implementation path works for a GitHub Free private consumer
   without paid hosting controls or consumer secrets.
8. SBOMs bind every archive digest; release attestations cover every published
   asset; the release manifest binds source, catalog, snapshot, templates,
   automation, cutoff, binaries, schemas, and supported targets.
9. A separately approved bounded set of representative repositories or exports
   is checked read-only. Their identities and results remain operational
   evidence and are not copied into normative policy.

The stable tag must not be created until all nine gates have evidence against
the exact source commit. Final source SHA, asset digests, workflow run, release
URL, and bootstrap-locator pins are post-publication evidence and cannot be
predeclared.

## GitHub Free private baseline

The checker needs no network, GitHub API, credentials, organization secret,
protected branch, ruleset, required check, Environment, Dependency Review, or
private artifact attestation. The reusable workflow preserves findings while
converting only conformance exit `1` to a report-only job result. Configuration
and internal errors (`2` and `3`) still fail closed. Paid adapters may strengthen
merge or review enforcement but do not change the release contract.

The `1.1.0` channel contains only public generic implementation assets. It does
not contain, depend on, or claim support for a restricted implementation. A
future restricted implementation must use a separate trust and release boundary
and preserve equivalent immutable identity, integrity, and rollback evidence.

## CI cost boundary

Central pull-request validation remains 12 GitHub-hosted jobs: one repository
gate, four native targets, four generated-profile targets, two migration and
rollback matrix entries, and one reusable-workflow integration job. The maximum
configured aggregate runner time remains 275 minutes, the longest individual
timeout remains 40 minutes, and the matrix runs only in the public
implementation repository. The tag-only release path remains seven jobs with
95 aggregate timeout-minutes. A private consumer still invokes one reusable job
with a 20-minute timeout and does not inherit central native,
generated-profile, or migration matrices.

## Adoption boundary

New-repository materialization records tooling bundle `1.1.0`, emits a thin
report-only caller, and writes only to an explicitly selected empty staging
directory. The generator does not create
`.github/golden-path-native-roots.yaml`; repositories add that sidecar only
after separately inventorying dependency-manager roots.

Existing repositories are not discovered, enrolled, or changed by this
release. A read-only result against a repository without explicit Golden Path
metadata is a configuration result, not authorization to infer metadata, open a
migration pull request, or add a required check.

After publication, the organization bootstrap locator is updated in a separate
policy-repository change using the exact release source commit, manifest and
snapshot-manifest hashes, archive hashes, and immutable automation pins.

## Support, deprecation, and rollback

`1.x` remains the supported stable implementation line. `1.0.1` is the
immediate immutable rollback source. The 2026-08-01 announcement continues to
deprecate `0.1.x` and `0.2.x` through 2027-01-28; `0.2.0` remains a verified
legacy migration source, while `0.1.x` must first advance to `0.2.0`.

Rollback restores all of the following as one identity:

- exact binary release and archive checksum;
- matching release and compatibility manifests;
- matching standard snapshot and template/materialization version; and
- matching reusable workflow and setup-action full commits.

Rollback to `1.0.1` does not evaluate native-roots sidecars and must not be
represented as equivalent multi-root coverage. Rollback never mutates a
published release or consumer default branch. Any materialized file change is
reviewed as a candidate diff owned by the consumer.
