# FiftyTen engineering tooling

Shared, versioned implementations of the FiftyTen Golden Path developer-tooling
standard.

This repository is the implementation plane. Normative policy remains in the
organization `.github` repository; released binaries embed an immutable,
digest-bound snapshot of that policy.

## Golden Path checker

The `golden-path` CLI performs bounded, read-only structural conformance checks
without network access, credentials, a GitHub API, or user-global
configuration.

```sh
just init
just ci

go run ./cmd/golden-path check \
  --root /path/to/repository \
  --evaluated-at 2026-07-31T00:00:00Z \
  --json-output /tmp/golden-path-result.json
```

The text result is written to standard output. The JSON result is written to
the explicitly selected path. Use `--json-output -` to write JSON to standard
output and text to standard error.

The checker is intentionally `0.x` and report-only while its compatibility,
rollback, platform, and migration fixtures are being completed.

Every catalog rule is represented in output. Fully evaluated structural rules
can pass or fail; non-applicable, hybrid, manual, or only partially evaluated
rules are explicit `skip` findings. The checker never converts missing evidence
into a pass.

The `2026.07` runtime catalog uses a symbolic coordinated selector for Rust.
Until an exact Rust version-to-disposition mapping is included in a later
immutable snapshot, `DT-RUNTIME-001` is therefore a truthful `skip` for Rust
instead of an inferred pass.

## Repository layout

- `checker/`: checker library and stable result contract
- `compatibility/`: supported standard and schema compatibility
- `standards/snapshots/`: immutable normative source snapshots
- `testdata/`: positive, negative, exception, and malformed fixtures
- `release/`: release manifest and packaging contracts

## Security and disclosure boundary

This is a public, generic tooling repository. Do not add secrets, customer
data, private endpoints, proprietary product logic, non-public dependencies,
or sensitive workflow inputs and outputs.

See [SECURITY.md](SECURITY.md) for private vulnerability reporting.
