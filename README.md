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

The default text result contains identity, status counts, a categorized skip
summary, and actionable findings only. Pass and skip detail is available
explicitly with `--show-all` or `--verbose`. The JSON result always retains the
complete finding set and is written to the selected path. Use `--json-output -`
to write JSON to standard output and text to standard error. CI callers can
additionally select `--github-summary-output "$GITHUB_STEP_SUMMARY"` and
`--github-annotations`; both human surfaces are bounded, while JSON remains the
complete record. File outputs must resolve outside the checked repository,
including through symbolic-link parents.

The `1.x` compatibility contract is stable. Conformance remains report-only:
the CLI preserves exit codes and evidence, while the reusable workflow does not
establish or claim a merge policy. A later enforcement decision is independent
of implementation stability.

The generated conformance caller invokes the immutable
`golden-path-quality.yml` reusable workflow with only its runner, working
directory, and expected profiles. The reusable workflow derives its release
identity from the called workflow SHA, verifies the matching release archive,
runs the structural checker, and uploads the complete JSON result only for a
non-passing result. Passing runs keep the bounded summary without duplicate
artifact storage. It does not install the consumer toolchain, run `just init`,
or run `just ci`; the consumer
repository's ordinary quality CI owns that execution exactly once.

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
Golden Path baseline. Bootstrap creates a complete starter, root onboarding,
and a repository-owned quality workflow that prepares the pinned environment
and runs `just ci` once. Adoption emits only the canonical request, truthful
metadata, generated-asset inventory, immutable bootstrap script, and thin
caller workflow. It does not generate or replace source entry points, native
manifests or locks, Mise and Just configuration, dependency automation, or
repository-specific build, smoke, release, and deployment behavior. Generate
the adoption candidate into an empty directory, integrate its fixed asset set
through the consumer repository's normal review, retain
the plan emitted on standard output as external staging evidence rather than
repository configuration, and use `upgrade` only after that baseline inventory
is committed. A written candidate never contains `golden-path-plan.json`.

New requests state their materialization mode explicitly. Bootstrap targets
remain optional when a documentation, source-only, or otherwise pre-release
starter has no truthful production or release representative. Adoption of an
existing repository still requires its actual production or release
representative targets. When targets are declared, at least one uses the
`primary` or `secondary` tier;
evaluation-only targets cannot satisfy that boundary. Each target records OS,
architecture, support tier, and optional runtime, target triple, and
semantic-execution claim. The generator preserves these declarations exactly
and never infers support from a profile, runner label, or compilation result.
Legacy requests that omit the mode retain their canonical request shape and
implicit bootstrap behavior.

Generation records the standard, asset bundle, release source commit, request
digest, canonical request, and the digest and mode of the long-lived managed
integration set. That set is the request, metadata, inventory itself,
conformance caller, and `scripts/golden-path`; the inventory is tracked
implicitly because it cannot hash itself. Source, README, native manifests and
locks, Mise/Just files, dependency policy, Dependabot configuration, and
repository quality CI are one-time scaffold and become repository-owned
immediately. Upgrade reads only the managed set, including the bounded handoff
from older inventories that listed scaffold. A deleted managed file, local
managed-file customization, or changed managed executable mode is a conflict.
Repository-owned edits are not.
An unresolved conflict returns exit `1` and never materializes a candidate. A
conflict-free upgrade writes only managed files to a separate candidate; it
never writes to the source repository or a default branch.

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
directory, selected profiles, and the immutable automation source. Release
checksums and provenance identity stay inside the shared release boundary. The
reusable workflow installs only its exact GitHub CLI verifier through an
isolated Mise configuration, blocks the runner-bundled `gh`, verifies the
checker archive against its checksum and GitHub artifact attestation, and then
runs the structural checker. It does not install Just or the consumer
toolchain, and it does not invoke repository quality commands. Repository-local
bootstrap remains separate and uses the exact GitHub CLI pinned by the
repository's `mise.toml` and `mise.lock` when installing the checker outside the
reusable workflow. Neither path requires GitHub Environments, protected
branches, paid rulesets, deployment credentials, or consumer secrets, so
private consumers on GitHub Free retain the baseline outcome.

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

The compatibility manifest maps the selected standard's runtime lines to exact Node.js,
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

## Dependency operations

The dependency compiler consumes repository-owned facts and produces a
deterministic review candidate. It never edits the repository, runs `just ci`,
creates release units, infers impact from component paths, or owns native
manifests and locks.

```sh
golden-path dependency check --root /path/to/repository

golden-path dependency preview \
  --root /path/to/repository \
  --observation /path/to/sealed-observation.json \
  --write \
  --output /tmp/dependency-candidate
```

`compile` is an alias for `preview`. Both write only to an explicitly selected
separate empty staging directory. Exit `0` is semantically aligned, `1` is a
reviewable policy/adapter difference, `2` is a repository declaration error,
and `3` is a tooling failure. An unknown routine surface remains
`pending-classification` at budget zero; classified roots default to three
open routine PRs unless a dated, owned override says otherwise.

Typed gate references bind repository `just ci` and optional workflow/job
evidence without executing the gate. Dependabot security routing is conditional
and independent from routine budgets. Existing guarded routers are preserved,
and no candidate removes or delays a security PR because routine regrouping is
pending.

Live reports require a sealed observation whose identity covers observation
time, collection query and scope, repository/default-branch identities, PR
refs/checks, alerts, native-manager sources, and SHA-256. Synthetic fixtures
exercise compiler behavior only and are never represented as live organization
evidence.

## Repository layout

- `checker/`: checker library and stable result contract
- `dependency/`: repository-fact compiler, semantic checker, and evidence reports
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
