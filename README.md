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

`just check` and `just ci` are stable zero-argument commands. They select the
current UTC evaluation time lazily; deterministic replays can set
`GOLDEN_PATH_EVALUATED_AT` or pass the timestamp as the recipe argument.

The text result is written to standard output. The JSON result is written to
the explicitly selected path. Use `--json-output -` to write JSON to standard
output and text to standard error. CI callers can additionally select
`--github-summary-output "$GITHUB_STEP_SUMMARY"` and
`--github-annotations`; both outputs are bounded, while JSON remains the
complete record. File outputs must resolve outside the checked repository,
including through symbolic-link parents.

The checker is intentionally `0.x` and report-only while its compatibility,
rollback, platform, and migration fixtures are being completed.

Every catalog rule is represented in output. Fully evaluated structural rules
can pass or fail; non-applicable, hybrid, manual, or only partially evaluated
rules are explicit `skip` findings. The checker never converts missing evidence
into a pass.

The compatibility manifest maps the `2026.07` runtime lines to exact Node.js,
Python, Go, Rust, and Zig patch releases. Mise selectors may use an exact
release or a major/minor selector that a committed `mise.lock` resolves to one
exact release. The resolved patch must still appear in the immutable runtime
mapping; the checker never infers runtime support from a broad selector.

The same versioned manifest defines the checker release's 30-day
exception-expiry warning window. Golden Path `0.1.0` intentionally does not
admit Zig's previous-tagged-stable compatibility-only tier: the normative
standard requires a bounded reason, owner, and review date, while the v1
repository metadata contract does not yet define that evidence. Until a future
contract represents it, the previous Zig stable fails as unsupported rather
than passing without the required evidence.

## Repository layout

- `checker/`: checker library and stable result contract
- `compatibility/`: supported standard and schema compatibility
- `standards/snapshots/`: immutable normative source snapshots
- `testdata/`: positive, negative, exception, and malformed fixtures
- `release/`: release manifest and packaging contracts

Every public Darwin and Linux AMD64/ARM64 target is built and executed on a
matching native GitHub-hosted runner. Each archive has a deterministic
CycloneDX 1.6 SBOM whose root component is bound to the archive SHA-256 digest.

## Security and disclosure boundary

This is a public, generic tooling repository. Do not add secrets, customer
data, private endpoints, proprietary product logic, non-public dependencies,
or sensitive workflow inputs and outputs.

See [SECURITY.md](SECURITY.md) for private vulnerability reporting.
