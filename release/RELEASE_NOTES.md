# Golden Path tooling 1.2.0

Minor release for the `2026.08` Developer Tooling Standard. This release adds
backward-compatible explicit component capability declarations to the
generator while retaining the `golden-path/v1` compatibility epoch and
report-only enforcement.

## Changes from 1.1.0

- Adds the optional `capabilities` array to each
  `golden-path-generator-request/v1` component.
- Accepts only capabilities defined by the normative metadata contract and
  rejects empty, duplicate, or unknown values in both the published schema and
  generator semantics.
- Merges explicit capabilities with the generator's deterministic baseline and
  records the sorted union in aggregate and component metadata.
- Enables capability-scoped conformance rules to evaluate repositories that
  really package, publish, or release artifacts instead of skipping them as
  outside declared applicability.
- Preserves the canonical bytes and SHA-256 digest of every legacy request that
  omits the new optional field.
- Adds conflict-free upgrade coverage from a legacy generated request to an
  explicit-capability request without mutating the source repository.
- Adds a package-producing Node fixture that verifies generated metadata and
  capability-scoped rule activation end to end.

## Capability ownership boundary

Capabilities are explicit declarations of repository behavior. The generator
does not infer `publish` or `released-artifact` from
`artifactTypes: [package]`, and a capability declaration does not materialize a
release workflow, registry credential, package metadata, or publication
evidence. The consumer repository owns those implementation details. A newly
applicable rule may therefore report a real failure until that repository
implements the declared contract.

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

Existing generator requests remain valid and render with their previous
canonical request shape. No compatibility reader, migration schema, or new
contract epoch is required.

## External-tool cutoff

The exact external tools, runtime selections, generated-profile package locks,
workflow action commits, and Go module graph remain unchanged from `1.1.0`.
The 2026-08-04 cutoff is retained and binds the new `1.2.0` template-bundle
identity; no executable tool or generated dependency is silently upgraded.

## Support and deprecation

| Line | Status | Support deadline | Successor and migration path |
| --- | --- | --- | --- |
| `1.x` | Supported | — | Latest compatible `1.x` |
| `0.2.x` | Deprecated | 2027-01-28 | Verified direct migration through `0.2.0` |
| `0.1.x` | Deprecated | 2027-01-28 | Advance to `0.2.0`, then migrate to supported `1.x` |

Deprecated `0.x` receives only critical security or integrity fixes. Standard
`2026.08` remains preferred.

## Migration from 1.1.0 or 0.2.0

1. Download the exact `1.2.0` archive and `release-manifest.json`, then verify
   their checksums and artifact attestations against the `v1.2.0` tag, source
   commit, signer commit, and release workflow.
2. Keep an existing request unchanged when the generated baseline describes
   the repository truthfully. Add component `capabilities` only for behavior
   the repository actually implements.
3. Run `golden-path upgrade` without `--write` against the source repository,
   selected request, and verified `1.2.0` manifest.
4. Review newly applicable capability-scoped findings. Fix the repository
   implementation or correct an inaccurate declaration; do not add dummy
   evidence merely to obtain a pass.
5. Resolve generated-asset conflicts before repeating the upgrade with
   `--write` and a separate empty candidate directory. Review the plan and full
   candidate diff, then run the candidate's `just init` and `just ci`.
6. Adopt the candidate through the consumer repository's normal review,
   updating the binary, checksums, immutable automation pins, request, metadata,
   and generated-asset inventory as one change.

The upgrader never modifies the source repository or its default branch.

## Limitations

Hybrid, manual, non-applicable, and only partially evaluated rules remain
explicit `skip` findings. Explicit capabilities improve applicability truth;
they do not turn a structural checker into a registry, release, or operational
verification system.

This release does not establish merge protection and must not be represented
as `policy-required` or `platform-enforced`. Existing repositories are neither
modified nor enrolled; adoption remains separately owned repository work.

## Rollback

Restore the exact immutable `1.1.0` release identity: binary, release manifest,
archive and executable checksums, generated asset inventory, and full-commit
workflow/action pins. Because `1.1.0` does not recognize the new field,
repositories that added it must select their previously recorded request when
constructing a rollback candidate. `0.2.0` remains the verified legacy
fallback.

Re-materialize or upgrade only in a separate candidate directory and review the
resulting diff. Do not move a tag, replace an asset, overwrite a consumer
repository, or infer rollback from `latest`.
