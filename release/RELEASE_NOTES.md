# Golden Path tooling 1.2.4

Patch release for the `2026.08` Developer Tooling Standard. It corrects
adoption composition and dependency-root validation without changing the
`golden-path/v1` compatibility epoch, schema identifiers, runtime selections,
or report-only enforcement.

## Correction from 1.2.3

- Allows an adoption component to declare multiple language profiles when one
  existing artifact actually spans them, including Python and Rust native
  extensions. Bootstrap retains the narrower combinations its starter source
  templates can materialize safely.
- Adds executed regression coverage for the existing ability of multiple
  artifact components to share one repository-owned native workspace root.
  Component paths continue to identify artifacts, while
  `.github/golden-path-native-roots.yaml` independently identifies dependency
  manager and lockfile boundaries. This shared-workspace behavior was already
  accepted by `1.2.3`; the new coverage prevents the adoption correction from
  regressing it.
- Applies source-bearing artifact, single-language, single-IaC-engine, and
  Node-hosted code-first IaC starter constraints only to bootstrap. Adoption
  records the existing repository contract and emits no starter source.
- Keeps code-first IaC adoption truthful by requiring a supported host profile
  (`node-typescript`, `python`, or `go`) without imposing the Node-only
  bootstrap template constraint.
- Separates native dependency-root marker validation from artifact-component
  marker and coverage validation. This preserves detection of undeclared or
  uncovered manifests without treating a dependency root as an artifact.
- Adds executed Quant-shaped Python/Rust/native-extension and Core-shaped
  shared Rust workspace fixtures, plus adoption-only IaC and Zig composition
  coverage. Existing bootstrap requests remain unchanged and retain their
  previous validation and rendering behavior.

The reusable-workflow Just bootstrap correction introduced in `1.2.3` remains
unchanged and is summarized below.

## Corrections retained from 1.2.3

### Just bootstrap correction from 1.2.2

- Installs the release-pinned Just `1.57.0` beside the exact GitHub CLI verifier
  from the existing checksum-bearing isolated Mise lock. The workflow no longer
  assumes that a consumer's Mise configuration owns Just or that a hosted
  runner happens to provide it.
- Installs the remaining consumer-pinned toolchain from the selected
  `working-directory` instead of always starting discovery at the checkout
  root. Native Mise parent-discovery semantics remain unchanged.
- Invokes the resolved Just executable directly and places its directory first
  on `PATH` for each root command. This preserves nested `just` calls while a
  hostile ambient-Just guard proves that runner or consumer PATH precedence
  cannot satisfy the workflow by accident.
- Executes the reusable workflow as a bounded matrix over a prior generated
  bootstrap repository and a `1.2.2` adoption repository whose local
  `mise.toml` and `mise.lock` omit Just. The adoption fixture also runs its
  repository-owned `init`, nested
  `check` and `build`, `ci`, and zero-failure structural conformance paths.

The exact GitHub CLI bootstrap correction introduced in `1.2.2` remains
unchanged and is summarized below.

## Corrections retained from 1.2.2

- Installs the caller-selected exact GitHub CLI before provenance verification
  from the release-recorded cross-platform checksums under strict Mise lock
  enforcement. The Mise `2026.7.18` bootstrap is itself selected by exact
  platform checksum.
- Isolates project, parent, global, system, cache, data, and state configuration
  boundaries so ambient runner configuration cannot add or replace verifier
  tools. Hostile parent, default-global, and default-system guard entries make
  any isolation regression fail the strict locked install.
- Resolves and invokes that exact executable directly instead of trusting the
  GitHub-hosted runner's bundled `gh` or PATH precedence. A scoped bare-CLI
  guard makes that independence an executed reusable-workflow invariant.
- Exercises the released reusable-workflow path against an immutable prior
  Golden Path release, so a missing or mismatched runner CLI cannot stop the
  workflow before `just init`, `just ci`, and conformance.

The structure-sensitive conformance and developer-global Mise isolation
corrections introduced in `1.2.1` remain unchanged and are summarized below.

## Corrections retained from 1.2.1

- Parses workflows by their YAML structure so output names and block-scalar
  text are not mistaken for action or image references.
- Covers job-container shorthand, service containers, reusable workflows,
  step actions, and the bounded graph of repository-local composite and Docker
  actions, including their nested remote references.
- Requires direct VCS and URL dependencies to carry a full commit or validated
  digest in an ecosystem-defined revision or integrity position; unrelated
  40-character text no longer counts as immutability evidence.
- Reads `go.mod` and `go.work` runtime directives through Go's native module
  parser, preserving valid directive comments.
- Recognizes quoted default expressions, quiet recipes, and top-level imports
  in the Just façade without interpreting recipe-body commands as imports.
- Uses an isolated `MISE_CONFIG_DIR` plus an explicit empty
  `MISE_GLOBAL_CONFIG_FILE` for generated initialization and exercises both
  hostile user-config inputs in CI.
- Replaces the release cutoff's raw workflow `uses:` scan with a parsed YAML
  oracle that ignores block-scalar content and supports job and step syntax.

The capability, target, and adoption functionality introduced in `1.2.0`
remains unchanged and is summarized below for the complete current `1.2.x`
surface.

## Changes from 1.1.0

- Adds the optional `capabilities` array to each
  `golden-path-generator-request/v1` component.
- Adds optional top-level `targets` that mirror the normative metadata target
  contract and are recorded without profile- or runner-based inference.
- Adds explicit `bootstrap` and `adoption` materialization modes. Adoption
  emits only five managed assets: the canonical request, metadata,
  generated-asset baseline, immutable bootstrap script, and thin caller
  workflow. A staging-only materialization plan remains outside that inventory.
- Requires explicit targets whenever a request selects a materialization mode,
  requires at least one primary or secondary representative, rejects repeated
  platform identities with conflicting attributes, and requires every adoption
  component to declare its exact implemented-capability set, including an
  explicit empty array when none apply.
- Keeps existing source, entry points, native manifests and locks, Mise and
  Just configuration, dependency automation, build and smoke behavior, and
  release or deployment contracts outside the adoption asset set.
- Rejects an explicitly empty materialization mode instead of silently treating
  it as an omitted legacy field, and validates derived Go module paths in both
  decoded CLI requests and direct generator API calls.
- Accepts only capabilities defined by the normative metadata contract and
  rejects duplicate or unknown values. Bootstrap rejects an explicitly empty
  declaration; adoption preserves it as an exact truthful set.
- In bootstrap mode, merges explicit capabilities with the generator's
  deterministic baseline and records the sorted union in aggregate and
  component metadata. Adoption records the explicit set without inference.
- Enables capability-scoped conformance rules to evaluate repositories that
  really package, publish, or release artifacts instead of skipping them as
  outside declared applicability.
- Preserves the canonical bytes and SHA-256 digest of every legacy request that
  omits the new optional fields.
- Adds conflict-free upgrade coverage from a legacy generated request to an
  explicit-capability request without mutating the source repository.
- Adds a package-producing Node fixture that verifies generated metadata and
  capability-scoped rule activation end to end.

## Capability ownership boundary

Capabilities are explicit declarations of repository behavior. The generator
does not infer `publish` or `released-artifact` from
`artifactTypes: [package]`, and a capability declaration does not materialize a
release workflow, registry credential, package metadata, or publication
evidence. The consumer repository owns those implementation details. A newly
applicable rule may therefore report a real failure until that repository
implements the declared contract.

Bootstrap mode records the union of explicit declarations and the behavior it
materializes. Adoption mode records exactly the explicit declarations because
the existing repository owns every executable quality, build, package, and
release path.

## Target and adoption boundary

Targets are repository support claims, not generator inference. A request may
declare OS, architecture, runtime or target triple, support tier, and whether
semantic execution is actually established. Compilation alone does not justify
`execution: true`. At least one target must be primary or secondary, and a
single platform identity cannot carry multiple tier or execution claims.

Adoption is a fixed control-plane materialization mode rather than an arbitrary
per-file selector. It does not accept a product entry point, binary name, smoke
command, include list, or exclude list. The consumer integrates the generated
control-plane assets into its existing tree and keeps product and operational
authority repository-local. Once the generated-asset baseline is committed,
normal conflict-aware `upgrade` applies to that fixed asset set.
Changing a generated bootstrap baseline to adoption produces explicit retirement
plan entries for the no-longer-managed starter assets; consumer customization
turns the affected retirement into a conflict and prevents candidate staging.

## Compatibility

- Golden Path standard: `2026.08` (`preferred`)
- Contract: `golden-path/v1`
- Metadata: `golden-path-metadata/v1`
- Native roots: `golden-path-native-roots/v1`
- Exceptions: `golden-path-exceptions/v1`
- Output: `golden-path-checker-output/v1`
- Generator request: `golden-path-generator-request/v1`
- Generated asset inventory: `golden-path-generated-assets/v1`
- Materialization plan: `golden-path-materialization-plan/v1`
- Release manifest: `golden-path-release-manifest/v2`
- Enforcement: `report-only`

Existing generator requests remain valid and render with their previous
canonical request shape and implicit bootstrap behavior. No compatibility
reader, migration schema, or new contract epoch is required.

## External-tool cutoff

The exact external tools, runtime selections, generated-profile package locks,
workflow action commits, and Go module graph remain unchanged from `1.1.0`.
The 2026-08-04 cutoff is retained and binds the corrected `1.2.4` template-bundle
identity; no executable tool or generated dependency is silently upgraded.

## Support and deprecation

| Line | Status | Support deadline | Successor and migration path |
| --- | --- | --- | --- |
| `1.x` | Supported | — | Latest compatible `1.x` |
| `0.2.x` | Deprecated | 2027-01-28 | Verified direct migration through `0.2.0` |
| `0.1.x` | Deprecated | 2027-01-28 | Advance to `0.2.0`, then migrate to supported `1.x` |

Deprecated `0.x` receives only critical security or integrity fixes. Standard
`2026.08` remains preferred.

## Upgrade from 1.2.3 or migration from 1.2.2, 1.2.1, 1.2.0, 1.1.0, or 0.2.0

1. Download the exact `1.2.4` archive and `release-manifest.json`, then verify
   their checksums and artifact attestations against the `v1.2.4` tag, source
   commit, signer commit, and release workflow.
2. Keep an existing request unchanged when the generated baseline describes
   the repository truthfully. Add component `capabilities` and top-level
   `targets` only for behavior and support claims the repository actually
   implements.
3. Run `golden-path upgrade` without `--write` against the source repository,
   selected request, and verified `1.2.4` manifest.
4. Review newly applicable capability-scoped findings. Fix the repository
   implementation or correct an inaccurate declaration; do not add dummy
   evidence merely to obtain a pass.
5. Resolve generated-asset conflicts before repeating the upgrade with
   `--write` and a separate empty candidate directory. Review the plan and full
   candidate diff, then run the candidate's `just init` and `just ci`.
6. Adopt the candidate through the consumer repository's normal review,
   updating the binary, checksums, immutable automation pins, request, metadata,
   and generated-asset inventory as one change.

For a repository that has never committed a Golden Path generated-asset
inventory, create a separate request with `materializationMode: adoption`,
explicit component capabilities, and at least one primary or secondary
representative target. Run `generate` into an empty directory, integrate only
its fixed five managed assets, retain the staging plan as review evidence
rather than repository configuration, and preserve the existing repository's
source, dependency, command, release, and deployment contracts. Run the
consumer repository's native checks and `just ci` after integration. Do not run
`upgrade` until this first baseline is committed.

The upgrader never modifies the source repository or its default branch.

## Limitations

Hybrid, manual, non-applicable, and only partially evaluated rules remain
explicit `skip` findings. Explicit capabilities improve applicability truth;
they do not turn a structural checker into a registry, release, or operational
verification system.

This release does not establish merge protection and must not be represented
as `policy-required` or `platform-enforced`. Existing repositories are neither
modified nor enrolled; adoption remains separately owned repository work, and
the generator never writes into the existing repository.

## Rollback

For a repository whose request remains within the composition accepted by
`1.2.3`, restore that exact immutable release identity: binary, release
manifest, archive and executable checksums, generated asset inventory, and
full-commit workflow/action pins.

A repository that adopts a cross-language or other composition newly accepted
by `1.2.4` cannot pass that request directly to `1.2.3`. Its rollback restores
the previous Golden Path control-plane baseline from repository history as one
unit, including the prior request, metadata, generated asset inventory, binary,
and workflow pins. Product source, manifests, dependency roots, locks, and
runtime contracts remain repository-owned and do not require rewriting. No
compatibility reader or generated-state migration is introduced. `1.2.2`,
`1.2.1`, `1.2.0`, `1.1.0`, and `0.2.0` remain verified older migration sources.

Re-materialize or upgrade only in a separate candidate directory and review the
resulting diff. Do not move a tag, replace an asset, overwrite a consumer
repository, or infer rollback from `latest`.
