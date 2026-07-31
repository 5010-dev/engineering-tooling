# Release contract

`golden-path` uses SemVer independently of the Golden Path standard CalVer.
All `0.x` releases are prerelease-quality and report-only.

A release workflow accepts only a tag whose peeled commit is contained in
validated `main`, builds the declared target matrix from that exact source,
executes every artifact on a matching native runner, generates per-asset
SHA-256 checksums and CycloneDX SBOMs, attests build provenance, and publishes
an immutable GitHub prerelease.

Each release must contain:

- `golden-path_<version>_<os>_<architecture>.tar.gz`
- `golden-path_<version>_<os>_<architecture>.cdx.json`
- `checksums.txt`
- `compatibility-manifest.json`
- `golden-path-checker-compatibility-v1.schema.json`
- `golden-path-generated-assets-v1.schema.json`
- `golden-path-generator-request-v1.schema.json`
- `golden-path-materialization-plan-v1.schema.json`
- `golden-path-release-manifest-v1.schema.json`
- `release-manifest.json`
- `standard-snapshot-manifest.json`
- generated GitHub artifact attestations
- release notes with compatibility and rollback guidance

Consumers select an exact SemVer asset and verify both its archive checksum and
GitHub artifact attestation before extraction. The release manifest also binds
each archive to the SHA-256 digest of the executable inside it; generated
bootstrap scripts verify that executable before first use and on every cache
reuse. Verification is bound to the release workflow, source commit, signer
commit, and exact tag using the GitHub CLI version pinned in the generated lock.
Mutable tags, `latest`, unverified provenance, and network-to-interpreter
bootstrap are not supported.

Build and assembly jobs hold only `contents: read`. The final job receives
`contents`, `attestations`, and `id-token` write permissions after downloading
the already assembled bundle, and publishes it with the exact `gh` version
recorded in `mise.lock`.
