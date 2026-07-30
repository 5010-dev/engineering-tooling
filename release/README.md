# Release contract

`golden-path` uses SemVer independently of the Golden Path standard CalVer.
All `0.x` releases are prerelease-quality and report-only.

A release workflow builds the declared target matrix from a clean source
checkout, tests with the race detector, generates per-asset SHA-256 checksums,
attests build provenance, and publishes an immutable GitHub prerelease.

Each release must contain:

- `golden-path_<version>_<os>_<architecture>.tar.gz`
- `checksums.txt`
- `compatibility-manifest.json`
- `golden-path-checker-compatibility-v1.schema.json`
- `golden-path-release-manifest-v1.schema.json`
- `release-manifest.json`
- `standard-snapshot-manifest.json`
- generated GitHub artifact attestations
- release notes with compatibility and rollback guidance

Consumers select an exact SemVer asset and verify its checksum before
execution. Mutable tags, `latest`, and network-to-interpreter bootstrap are not
supported.
