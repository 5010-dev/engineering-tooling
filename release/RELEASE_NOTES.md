# Golden Path tooling 1.0.1

Patch release for the `2026.07` Developer Tooling Standard. This release changes
implementation behavior and evidence only; the normative standard, contract,
runtime selections, and report-only enforcement state are unchanged.

## Changes from 1.0.0

- Generated `just init` ignores developer-global mise tool declarations while
  retaining the repository-local `mise.toml` and locked tool resolution.
- The reusable quality workflow publishes bounded checker evidence after
  repository initialization or `just ci` failures whenever verified checker
  setup completed.
- Generated-profile CI injects a hostile global mise tool to prevent regression
  of the isolated initialization contract.
- The generator release fixture now satisfies the published v2 release-manifest
  schema and is continuously schema-validated.
- Release documentation states the tag-invisible checkout context required for
  byte-for-byte Go artifact reproduction.
- The standard snapshot manifest now specifies the exact aggregate line format,
  and the checker validates that definition as part of snapshot loading.

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

## External-tool cutoff

The exact external tools, runtimes, native locks, package locks, and workflow
action commits remain unchanged from the 2026-08-01 cutoff used by `1.0.0`.
Updated integrity evidence binds the `1.0.1` template-bundle identity.

## Support and deprecation

| Line | Status | Support deadline | Successor and migration path |
| --- | --- | --- | --- |
| `1.x` | Supported | — | Latest compatible `1.x` |
| `0.2.x` | Deprecated | 2027-01-28 | Verified direct migration through `0.2.0` |
| `0.1.x` | Deprecated | 2027-01-28 | Advance to `0.2.0`, then migrate to supported `1.x` |

Deprecated `0.x` receives only critical security or integrity fixes. Standard
`2026.07` remains preferred because this patch changes no normative rule
semantics.

## Migration from 1.0.0 or 0.2.0

1. Download the exact `1.0.1` archive and `release-manifest.json`, then verify
   the archive checksum and both artifact attestations against the `v1.0.1` tag,
   source commit, signer commit, and release workflow.
2. Run `golden-path upgrade` without `--write` against the existing repository,
   its recorded request, and the verified `1.0.1` release manifest.
3. Resolve any reported conflict before running the same command with `--write`
   and a separate empty candidate directory.
4. Review `golden-path-plan.json` and the complete candidate diff, then run the
   candidate's `just init` and `just ci`.
5. Adopt the candidate through the consumer repository's normal review. Update
   binary version, archive and executable checksums, reusable-workflow and
   setup-action full commits, and generated asset inventory as one change.

The upgrader never modifies the source repository or its default branch. When a
plan contains unresolved conflicts, `--write` returns exit `1` without creating
the requested candidate directory. Repositories without a prior generated asset
inventory use the separately owned adoption workflow rather than inferring a
migration.

## Limitations

Hybrid, manual, non-applicable, and only partially evaluated rules remain
explicit `skip` findings. This release does not establish merge protection and
must not be represented as `policy-required` or `platform-enforced`. Existing
repositories are neither modified nor enrolled; adoption remains separately
owned repository work.

## Rollback

Restore the exact immutable `1.0.0` release identity: binary, release manifest,
archive and executable checksums, generated asset inventory, and full-commit
workflow/action pins. `0.2.0` remains the verified legacy fallback. Re-materialize
or upgrade only in a separate candidate directory and review the resulting diff.
Do not move a tag, replace an asset, overwrite a consumer repository, or infer
rollback from `latest`.
