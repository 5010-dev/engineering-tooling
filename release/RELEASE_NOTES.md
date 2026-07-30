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
- Positive, negative, exception, and malformed contract fixtures
- Darwin and Linux release targets for AMD64 and ARM64

## Limitations

Hybrid, manual, non-applicable, and only partially evaluated rules are emitted
as explicit `skip` findings. The `2026.07` runtime catalog uses a symbolic Rust
selector, so exact Rust disposition remains skipped until a coordinated exact
mapping is bundled. This release does not establish merge protection and must
not be represented as `policy-required` or `platform-enforced`.

## Rollback

Pin the previous exact checker version and verified checksum. Standard snapshot
contents and published release assets are immutable.
