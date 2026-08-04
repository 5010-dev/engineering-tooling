# Golden Path tooling 1.1.0

Minor release for the `2026.08` Developer Tooling Standard. This release adds
backward-compatible native dependency-root declarations while retaining the
`golden-path/v1` compatibility epoch and report-only enforcement.

## Changes from 1.0.1

- Embeds the immutable `2026.08` standard snapshot from organization-policy
  commit `604a40886d5a1cba3b304471e6e072b72cec7601` and verifies its exact source
  tree, file inventory, catalog digest, and aggregate digest.
- Loads the optional repository-owned
  `.github/golden-path-native-roots.yaml` contract when dependency-manager
  roots differ from generated artifact-component paths.
- Evaluates native manifest, lock, selector, and profile rules independently
  for every declared root while preserving component-derived behavior when the
  sidecar is absent.
- Supports committed `go.work` roots by validating every repository-local
  referenced module and its conditional checksum record.
- Checks structural IaC dependency authority, including CDK and Pulumi project
  manifests, Terraform/OpenTofu locks, and exact or immutable module sources.
- Allows disjoint profiles to share one path, requires every selected native
  profile to have a root, and rejects same-profile overlaps or roots outside
  aggregate profile metadata.
- Keeps artifact types and capabilities out of native-root declarations and
  exception matching so artifact classification cannot silently waive a
  dependency-graph finding.
- Reports schema and semantic sidecar failures as configuration errors with
  actionable detail.
- Adds end-to-end coverage for polyglot same-path roots, invalid declarations,
  no-sidecar compatibility, independent same-ecosystem graphs, exception
  scoping, and undeclared root markers.

## Compatibility

- Golden Path standard: `2026.08` (`preferred`)
- Contract: `golden-path/v1`
- Metadata: `golden-path-metadata/v1`
- Native roots: `golden-path-native-roots/v1`
- Exceptions: `golden-path-exceptions/v1`
- Output: `golden-path-checker-output/v1`
- Generator request: `golden-path-generator-request/v1`
- Generated asset inventory: `golden-path-generated-assets/v1`
- Materialization plan: `golden-path-materialization-plan/v1`
- Release manifest: `golden-path-release-manifest/v2`
- Enforcement: `report-only`

Repositories without the native-roots sidecar retain the `1.0.1` component
fallback. The sidecar is optional and introduces no generated-metadata schema
change.

## External-tool cutoff

The exact external tools, runtime selections, generated-profile package locks,
and workflow action commits remain unchanged from `1.0.1`. The checker adds the
exact `github.com/hashicorp/hcl/v2` `2.24.0` library to parse Terraform/OpenTofu
module declarations; the 2026-08-04 cutoff binds the resulting Go checksum
record and the new `1.1.0` template-bundle identity. This release does not
silently upgrade executable tools or generated project dependencies.

## Support and deprecation

| Line | Status | Support deadline | Successor and migration path |
| --- | --- | --- | --- |
| `1.x` | Supported | — | Latest compatible `1.x` |
| `0.2.x` | Deprecated | 2027-01-28 | Verified direct migration through `0.2.0` |
| `0.1.x` | Deprecated | 2027-01-28 | Advance to `0.2.0`, then migrate to supported `1.x` |

Deprecated `0.x` receives only critical security or integrity fixes. Standard
`2026.08` becomes preferred when the organization bootstrap locator selects
this released implementation.

## Migration from 1.0.1 or 0.2.0

1. Download the exact `1.1.0` archive and `release-manifest.json`, then verify
   the archive checksum and both artifact attestations against the `v1.1.0`
   tag, source commit, signer commit, and release workflow.
2. Run `golden-path upgrade` without `--write` against the existing repository,
   its recorded request, and the verified `1.1.0` release manifest.
3. Repositories whose native dependency roots already match generated
   artifact-component paths need no sidecar. Review the ordinary generated
   candidate and continue the existing upgrade workflow.
4. A repository with additional or different dependency-manager roots must
   inventory those roots separately and add
   `.github/golden-path-native-roots.yaml` as repository-owned configuration.
   Do not edit generated `.github/golden-path.yaml` or copy artifact types and
   capabilities into the sidecar.
5. Resolve any generated-asset conflict before repeating the upgrade with
   `--write` and a separate empty candidate directory. Review
   `golden-path-plan.json`, the complete candidate diff, and the sidecar change,
   then run the candidate's `just init` and `just ci`.
6. Adopt the candidate through the consumer repository's normal review. Update
   binary version, archive and executable checksums, reusable-workflow and
   setup-action full commits, and generated asset inventory as one change.

The upgrader never modifies the source repository or its default branch. It
does not generate or overwrite the repository-owned native-roots sidecar.

## Limitations

Hybrid, manual, non-applicable, and only partially evaluated rules remain
explicit `skip` findings. Native-root discovery is declaration-based rather
than recursive: an undeclared nested dependency graph is not inferred. The
overlap rule intentionally rejects a root workspace and a separate nested root
for the same profile. That competing-authority declaration is a configuration
error rather than a waivable conformance finding; restructure or de-duplicate
the manager boundary before checking the repository.

This release does not establish merge protection and must not be represented
as `policy-required` or `platform-enforced`. Existing repositories are neither
modified nor enrolled; adoption remains separately owned repository work.

## Rollback

Restore the exact immutable `1.0.1` release identity: binary, release manifest,
archive and executable checksums, generated asset inventory, and full-commit
workflow/action pins. A `1.0.1` checker predates the native-roots contract and
therefore does not provide multi-root conformance evidence; retain the sidecar
as repository-owned data but do not represent the rollback result as equivalent
coverage. `0.2.0` remains the verified legacy fallback.

Re-materialize or upgrade only in a separate candidate directory and review the
resulting diff. Do not move a tag, replace an asset, overwrite a consumer
repository, or infer rollback from `latest`.
