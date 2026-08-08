# Release contract

`golden-path` uses SemVer independently of the Golden Path standard CalVer.
`0.x` releases are prerelease-quality. `1.x` is the stable implementation
contract, while its compatibility manifest continues to declare report-only
enforcement.

A release workflow accepts only a tag whose peeled commit is contained in
validated `main`, builds the declared target matrix from that exact source,
executes every artifact on a matching native runner, generates per-asset
SHA-256 checksums and CycloneDX SBOMs, attests build provenance, and publishes
an immutable GitHub prerelease for `0.x` or stable release for `1.x` and later.

Byte-for-byte reproduction MUST mirror the native build job's tag-invisible
source context. Native jobs check out the exact source commit without tag refs,
so Go records a pseudo-version for the main module in build information. A
tag-visible clone records the release tag instead and therefore produces a
different executable and archive digest. Reproduce published bytes from a
clean, tag-invisible checkout of the exact source commit with the pinned
toolchain and release commands.

Each release must contain:

- `golden-path_<version>_<os>_<architecture>.tar.gz`
- `golden-path_<version>_<os>_<architecture>.cdx.json`
- `checksums.txt`
- `compatibility-manifest.json`
- `golden-path-checker-compatibility-v1.schema.json`
- `golden-path-dependency-candidate-v1.schema.json`
- `golden-path-dependency-defers-v1.schema.json`
- `golden-path-dependency-observation-v1.schema.json`
- `golden-path-dependency-policy-v1.schema.json`
- `golden-path-dependency-report-v1.schema.json`
- `golden-path-generated-assets-v1.schema.json`
- `golden-path-generator-request-v1.schema.json`
- `golden-path-materialization-plan-v1.schema.json`
- `golden-path-release-manifest-v1.schema.json`
- `golden-path-release-manifest-v2.schema.json`
- `golden-path-source-integrity-v1.schema.json`
- `golden-path-tooling-cutoff-v1.schema.json`
- `RELEASE_NOTES.md`
- `release-manifest.json`
- `source-integrity.json`
- `standard-snapshot-manifest.json`
- `tooling-cutoff.json`
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

Release manifest v2 separates implementation lifecycle from enforcement and
binds the exact normative source, supported compatibility set, published
schemas, checker, generator, asset bundle, reusable automation, retained
external-tool cutoff, and release notes with SHA-256 digests. The release
checksums and attestations separately bind current source integrity without
changing the published v2 manifest shape. The v1 schema remains available only
to interpret immutable `0.x` releases.

Build and assembly jobs hold only `contents: read`. The final job receives
`contents`, `attestations`, and `id-token` write permissions after downloading
the already assembled bundle, and publishes it with the exact `gh` version
recorded in `mise.lock`.
