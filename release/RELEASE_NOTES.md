# Golden Path tooling 1.0.0

First stable implementation release for the `2026.07` Developer Tooling
Standard. Stability applies to compatibility, distribution, materialization,
and rollback contracts; conformance remains report-only.

## Compatibility

- Golden Path standard: `2026.07` (`preferred`)
- Contract: `golden-path/v1`
- Metadata: `golden-path-metadata/v1`
- Exceptions: `golden-path-exceptions/v1`
- Output: `golden-path-checker-output/v1`
- Generator request: `golden-path-generator-request/v1`
- Generated asset inventory: `golden-path-generated-assets/v1`
- Materialization plan: `golden-path-materialization-plan/v1`
- Release manifest: `golden-path-release-manifest/v2`
- Enforcement: `report-only`

## Stable readiness

- Exact 2026-08-01 external-tool cutoff with official sources, support status,
  native integrity authority, and workflow-action commit identities
- Deterministic offline checker with stable text, JSON, and exit-code contracts
- Preview-first generation and conflict-aware upgrade into a separate candidate
- Compatibility, migration, rollback, exception, false-positive, and malformed
  input fixtures
- Native execution on Darwin and Linux AMD64/ARM64 runners
- GitHub Free private-consumer path without protected branches, required
  checks, Environments, Dependency Review, or private artifact attestations
- Public generic release assets with no dependency on a restricted tooling
  channel; any future restricted implementation requires a separate trust and
  release boundary
- Digest-bound CycloneDX 1.6 SBOMs and GitHub artifact attestations for all
  released assets
- Release manifest source and component digests for the checker, generator,
  asset bundle, shared automation, schemas, cutoff, and migration notes

## Tooling cutoff changes from 0.2.0

- GitHub CLI `2.97.0`
- mise minimum `2026.7.18`
- uv and uv-build `0.12.1`
- aws-cdk-lib remains `2.262.2`; `2.263.0` had not satisfied pnpm's
  minimum-release-age policy at the cutoff
- actions/checkout `v7.0.1`, mise-action `v4.2.4`, and actions/attest
  `v4.2.1`, all pinned to full commits
- TypeScript remains `6.0.3`: the observed `7.0.2` release is outside
  typescript-eslint `8.65.0`'s supported peer range

## Support and deprecation

This release announces the following lifecycle state on 2026-08-01:

| Line | Status | Support deadline | Successor and migration path |
| --- | --- | --- | --- |
| `1.x` | Supported | — | Latest compatible `1.x` |
| `0.2.x` | Deprecated | 2027-01-28 | Verified direct migration and rollback with `0.2.0` and `1.0.0` |
| `0.1.x` | Deprecated | 2027-01-28 | Advance to `0.2.0`, then use the verified `1.0.0` migration |

Deprecated `0.x` receives only critical security or integrity fixes. Standard
`2026.07` remains preferred because this release changes exact implementation
selections without changing normative rule semantics.

## Migration from 0.2.0

1. Download the exact `1.0.0` archive and `release-manifest.json`, then verify
   the archive checksum and both artifact attestations against the `v1.0.0`
   tag, source commit, signer commit, and release workflow.
2. Run `golden-path upgrade` without `--write` against the existing repository,
   its recorded request, and the verified `1.0.0` release manifest.
3. Resolve any reported conflict before running the same command with `--write`
   and a separate empty candidate directory.
4. Review `golden-path-plan.json` and the complete candidate diff, then run the
   candidate's `just init` and `just ci`.
5. Adopt the candidate through the consumer repository's normal review. Update
   the binary version, archive and executable checksums, reusable-workflow and
   setup-action full commits, and generated asset inventory as one change.

The upgrader never modifies the source repository or its default branch.
Repositories without a prior generated asset inventory use the separately
owned adoption workflow rather than inferring a migration.

## Limitations

Hybrid, manual, non-applicable, and only partially evaluated rules remain
explicit `skip` findings. This release does not establish merge protection and
must not be represented as `policy-required` or `platform-enforced`. Existing
repositories are neither modified nor enrolled; adoption remains separately
owned repository work.

## Rollback

Restore the exact `0.2.0` binary, release manifest, archive checksum, and
full-commit workflow/action pin. Re-materialize or upgrade only in a separate
candidate directory and review the resulting diff. Do not move a tag, replace
an asset, overwrite a consumer repository, or infer rollback from `latest`.
