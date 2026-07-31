# Golden Path tooling 0.2.0

Report-only prerelease adding deterministic Golden Path materialization and
shared quality automation to the existing checker.

## Compatibility

- Golden Path standard: `2026.07`
- Contract: `golden-path/v1`
- Metadata: `golden-path-metadata/v1`
- Exceptions: `golden-path-exceptions/v1`
- Output: `golden-path-checker-output/v1`
- Generator request: `golden-path-generator-request/v1`
- Generated asset inventory: `golden-path-generated-assets/v1`
- Materialization plan: `golden-path-materialization-plan/v1`

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
- Versioned common, language, and IaC template bundle for every Accepted
  profile
- Preview-first `generate` and conflict-aware `upgrade` commands
- Reviewable staging output that never writes to an upgraded repository
- Thin caller workflow, reusable quality workflow, and checksum-verifying
  setup action
- Synthetic single-repository, monorepo, polyglot, documentation, and GitHub
  Free private-consumer fixtures

## Limitations

Hybrid, manual, non-applicable, and only partially evaluated rules are emitted
as explicit `skip` findings. This release does not establish merge protection
and must not be represented as `policy-required` or `platform-enforced`.
Generated candidates do not migrate or directly modify organization
repositories; adoption remains repository-owned work.

## Rollback

Pin the previous exact tooling version and verified checksum. Standard
snapshot contents, template bundles, automation source pins, and published
release assets are immutable.
