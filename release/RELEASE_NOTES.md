# Golden Path tooling 1.3.0

Compatible minor release for the `2026.08.1` Developer Tooling Standard. It
separates structural conformance from repository quality execution and makes
human evidence concise without changing `golden-path/v1`, the v1 JSON finding
contract, runtime selections, or report-only enforcement.

## Outcome

- A generated conformance caller passes only `runner`, `working-directory`,
  and `profiles` to the immutable reusable workflow.
- The published entrypoint remains `.github/workflows/golden-path-quality.yml`
  so released manifest v2 keeps its existing automation identity; its workflow
  and job display contract is now `Developer Tooling / Conformance`.
- The reusable workflow derives release identity from the called workflow's
  full commit SHA, verifies the matching attested release checksums and archive,
  and runs only the structural checker.
- It no longer installs the consumer toolchain or runs `just init` and
  `just ci`. Repository-owned quality CI remains the single owner of that
  execution.
- The default CLI text and GitHub summary show identity, status counts,
  categorized skip counts, and actionable findings. `--show-all` and
  `--verbose` retain exhaustive human diagnostics.
- The complete `golden-path-checker-output/v1` JSON finding set remains the
  canonical output. Non-passing workflow runs retain it as a short-lived
  artifact; passing runs avoid duplicate storage.

## Caller and evidence simplification

The caller no longer duplicates checker version, source commit, GitHub CLI
version, or four platform archive checksums. Those values remain inside the
immutable shared release boundary. The full workflow SHA is still visible and
reviewable in every consumer repository.

Pass and skip details are not deleted. They remain in canonical JSON and
explicit exhaustive text mode. A compatible `extensions.skipCategory` value
classifies skip totals without deriving semantics from message wording.
Report-only finding exit `1` remains visible evidence but does not make the
workflow a merge gate; configuration and internal exits `2` and `3` still fail
the workflow.

## Compatibility

- Standard: `2026.08.1` (`preferred`)
- Contract: `golden-path/v1`
- Metadata: `golden-path-metadata/v1`
- Native roots: `golden-path-native-roots/v1`
- Exceptions: `golden-path-exceptions/v1`
- Output: `golden-path-checker-output/v1`
- Generator request: `golden-path-generator-request/v1`
- Generated asset inventory: `golden-path-generated-assets/v1`
- Materialization plan: `golden-path-materialization-plan/v1`
- Release manifest: `golden-path-release-manifest/v2`
- Source integrity: `golden-path-source-integrity/v1`
- Enforcement: `report-only`

The `1.2.4` release and all earlier published tags and assets remain immutable.
The `1.2.4` polyglot-adoption and shared-native-workspace corrections are
retained with their regression fixtures. No compatibility reader, generated
state migration, dual workflow, or new contract epoch is introduced.

The August 4 external-tool cutoff is retained unchanged from `1.2.4` because
the selections did not move. A separate August 6 source-integrity record binds
the `1.3.0` locks and generated bundle, so dated evidence is not overwritten.

## Support and upgrade policy

| Line | Status | Required action |
| --- | --- | --- |
| `1.3.x` | Preferred | Default for new adoption |
| `1.2.x` | Supported | Upgrade on the consumer's normal maintenance cadence |
| `1.1.x` | Supported | Upgrade on the consumer's normal maintenance cadence |
| `0.2.x` | Deprecated until 2027-01-28 | Migrate through the documented supported `1.x` path |
| `0.1.x` | Deprecated until 2027-01-28 | Advance to `0.2.0`, then migrate to supported `1.x` |

A central locator update does not require an immediate consumer pull request.
This release is recommended where duplicate `just ci` execution materially
affects cost or feedback time; otherwise compatible upgrades may be batched.

## Upgrade from 1.2.4

1. Download the exact `1.3.0` release manifest, checksum list, and platform
   archive. Verify their attestations against the `v1.3.0` tag, source commit,
   signer commit, and release workflow.
2. Run `golden-path upgrade` without `--write` using the repository's existing
   request and the verified `1.3.0` manifest.
3. Review the generated candidate. The conformance caller changes to the new
   checker-only workflow and removes duplicated release inputs; product source,
   native manifests, locks, runtime behavior, quality CI, and deployment
   contracts remain repository-owned.
4. Materialize only into a separate empty candidate, review the diff, and run
   the consumer repository's native quality gate once plus structural
   conformance separately.

An existing adoption request does not need capability, target, profile, or
component changes merely to upgrade tooling. Do not mix unrelated dependency,
runtime, product, or deployment hardening into this maintenance change.

## Rollback

Restore the complete prior `1.2.4` Golden Path control-plane baseline from
repository history: request, metadata, generated asset inventory, bootstrap
script, binary identity, and workflow pin. The `1.2.4` reusable workflow owns
its previous caller inputs and quality execution; the `1.3.0` workflow owns the
new checker-only contract. Do not combine the workflow from one release with
generated inputs from the other.

Rollback does not require changing product source, manifests, locks, quality
commands, deployment behavior, runtime state, or durable data. Re-materialize
only in a separate candidate directory. Never move a tag or replace a published
asset.

## Limitations

Structural conformance still does not prove runtime behavior, hosting-plan
settings, merge protection, advisory state, release publication, deployment,
or production capacity. Manual, hybrid, and inapplicable rules remain explicit
skip findings in the complete JSON result. This release does not establish
branch protection, a ruleset, `policy-required`, or `platform-enforced` status.
