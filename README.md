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

The `1.x` compatibility contract is stable. Conformance remains report-only:
the CLI preserves exit codes and evidence, while the reusable workflow does not
establish or claim a merge policy. A later enforcement decision is independent
of implementation stability.

## Golden Path materialization

`generate` and `upgrade` use the template bundle embedded in the exact CLI
release. Both commands are preview-only unless `--write` and an empty staging
directory are provided together.

```sh
golden-path generate \
  --request golden-path-request.yaml \
  --release-manifest release-manifest.json

golden-path upgrade \
  --root /path/to/existing-repository \
  --request /path/to/existing-repository/.github/golden-path-request.json \
  --release-manifest release-manifest.json \
  --write \
  --output /tmp/golden-path-candidate
```

New repositories use an explicit `materializationMode: bootstrap` request.
Existing repositories use `materializationMode: adoption` for their first
Golden Path baseline. Adoption emits only the canonical request, truthful
metadata, generated-asset inventory, immutable bootstrap script, and thin
caller workflow. It does not generate or replace source entry points, native
manifests or locks, Mise and Just configuration, dependency automation, or
repository-specific build, smoke, release, and deployment behavior. Generate
the adoption candidate into an empty directory, integrate its fixed asset set
through the consumer repository's normal review, retain
`golden-path-plan.json` as staging evidence rather than managed repository
configuration, and use `upgrade` only after that baseline inventory is
committed.

An explicit materialization mode requires at least one production or release
representative `target` in the `primary` or `secondary` tier. Evaluation-only
targets cannot satisfy that boundary. Each target records OS, architecture,
support tier, and optional runtime, target triple, and semantic-execution
claim. The generator preserves these declarations exactly and never infers
support from a profile, runner label, or compilation result. Legacy requests
that omit both new fields retain their canonical request shape and implicit
bootstrap behavior.

Generation records the standard, asset bundle, release source commit, request
digest, canonical request, and every generated file digest and mode. Upgrade
reads that inventory to distinguish unchanged generated files from consumer
customization. A deleted managed file or changed executable mode is a conflict,
not an implicit create or update. An unresolved conflict returns exit `1` and
never materializes a candidate. A conflict-free upgrade writes a separate
candidate and `golden-path-plan.json`; it never writes to the source repository
or a default branch.

Each request component may declare `capabilities` from the published request
schema when the repository really provides behavior such as `package`,
`publish`, or `released-artifact`. Bootstrap mode merges those declarations
with the deterministic behavior materialized by the full starter. Adoption
mode requires the declaration and records exactly those capabilities because
the generator does not own the existing repository's implementation.
An adoption component uses an explicit empty array when it implements none of
the capability catalog entries; omission remains invalid because it would make
the repository claim ambiguous.
Capabilities are declarative: selecting `artifactTypes: [package]` does not
infer publication, and declaring `publish` does not create a release workflow.
The consumer repository still owns the corresponding implementation and
evidence.

Adoption records the existing artifact composition rather than constraining it
to starter templates. One component may therefore declare multiple language or
IaC profiles when the existing artifact actually spans them. Bootstrap keeps
the narrower profile combinations that its source templates can materialize.
Artifact components remain distinct from native dependency roots: shared
workspaces, cross-language roots, and independent same-profile graphs belong in
the repository-owned `.github/golden-path-native-roots.yaml` declaration rather
than the generated component inventory.

The generated caller owns events, permissions, concurrency, runner, working
directory, selected profiles, immutable automation source, and release
checksums and provenance identity. The reusable workflow installs the caller's
exact GitHub CLI verifier and the release-pinned Just runner from a
checksum-bearing lock through a fully isolated Mise configuration. It
checksum-pins the Mise bootstrap, blocks bare `gh`
during provenance verification, and invokes the resolved verifier executable
directly, so it never trusts a runner-bundled CLI or ambient configuration. It
verifies the archive checksum and GitHub artifact attestation against the
release workflow, source commit, signer commit, and tag. The workflow invokes
its exact Just binary and places that binary first on `PATH` for each root
command, so nested Just calls cannot fall back to a runner-bundled tool. The
consumer's Mise configuration remains authoritative for the rest of its
toolchain and does not have to own Just. Repository-local
bootstrap uses the exact GitHub CLI pinned by the repository's `mise.toml` and
`mise.lock`. The reusable workflow otherwise owns only stable quality
orchestration and calls the repository's root `just init` and `just ci`. It
does not require GitHub Environments, protected branches, paid rulesets,
deployment credentials, or consumer secrets, so private consumers on GitHub
Free retain the baseline outcome.

Executable artifact types receive native entry points and post-build runtime
smoke checks for Go, Node.js, Python, Rust, and Zig. Zig and `zig cc` profiles
map each supported Darwin or Linux AMD64/ARM64 host to an exact target and keep
their global cache repository-local. CI materializes the full profile matrix,
runs generated setup and quality commands, validates both automation entry
points against a released attested artifact, and repeats the Zig contract on
all four native runners.

Every catalog rule is represented in output. Fully evaluated structural rules
can pass or fail; non-applicable, hybrid, manual, or only partially evaluated
rules are explicit `skip` findings. The checker never converts missing evidence
into a pass.

The compatibility manifest maps the `2026.08` runtime lines to exact Node.js,
Python, Go, Rust, and Zig patch releases. Mise selectors may use an exact
release or a major/minor selector that a committed `mise.lock` resolves to one
exact release. The resolved patch must still appear in the immutable runtime
mapping; the checker never infers runtime support from a broad selector.

The same versioned manifest defines the checker release's 30-day
exception-expiry warning window. Golden Path `1.x` intentionally does not
admit Zig's previous-tagged-stable compatibility-only tier: the normative
standard requires a bounded reason, owner, and review date, while the v1
repository metadata contract does not yet define that evidence. Until a future
contract represents it, the previous Zig stable fails as unsupported rather
than passing without the required evidence.

## Repository layout

- `checker/`: checker library and stable result contract
- `generator/`: deterministic materialization and conflict-aware upgrade logic
- `templates/`: versioned common, profile, lock, and thin-caller assets
- `actions/`: reusable checksum- and provenance-verifying setup action
- `compatibility/`: supported standard and schema compatibility
- `standards/snapshots/`: immutable normative source snapshots
- `testdata/`: positive, negative, exception, and malformed fixtures
- `release/`: release manifest and packaging contracts

Every public Darwin and Linux AMD64/ARM64 target is built and executed on a
matching native GitHub-hosted runner. Each archive has a deterministic
CycloneDX 1.6 SBOM whose root component is bound to the archive SHA-256 digest.
The stable release also publishes the exact normative source, external-tool
cutoff, compatibility and schema evidence, checker, generator, asset-bundle and
automation tree digests, migration notes, release lifecycle, and report-only
enforcement state.

## Security and disclosure boundary

This is a public, generic tooling repository. Do not add secrets, customer
data, private endpoints, proprietary product logic, non-public dependencies,
or sensitive workflow inputs and outputs.

See [SECURITY.md](SECURITY.md) for private vulnerability reporting.
