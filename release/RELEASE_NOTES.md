# Golden Path checker 0.1.0

Initial report-only prerelease.

## Compatibility

- Golden Path standard: `2026.07`
- Contract: `golden-path/v1`
- Metadata: `golden-path-metadata/v1`
- Exceptions: `golden-path-exceptions/v1`
- Output: `golden-path-checker-output/v1`

## Included

- Offline and read-only repository structural checker
- Stable text and JSON output with exit codes `0` through `3`
- Immutable standard snapshot bound to source commit and SHA-256 digests
- Exact patch-level runtime compatibility mapping for Node.js, Python, Go,
  Rust, and Zig
- Positive, negative, exception, and malformed contract fixtures
- Native-validated Darwin and Linux release targets for AMD64 and ARM64
- Digest-bound CycloneDX 1.6 SBOM for every release archive
- Bounded GitHub step summary and workflow annotations

## Limitations

Hybrid, manual, non-applicable, and only partially evaluated rules are emitted
as explicit `skip` findings. This release does not establish merge protection
and must not be represented as `policy-required` or `platform-enforced`.

## Rollback

Pin the previous exact checker version and verified checksum. Standard snapshot
contents and published release assets are immutable.
