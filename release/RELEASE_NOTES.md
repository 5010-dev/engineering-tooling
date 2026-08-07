# Golden Path tooling 1.4.0

Compatible minor release for the `2026.08.2` Developer Tooling Standard. It
completes the new-repository bootstrap path while narrowing long-lived generator
ownership to the five Golden Path integration assets. It does not change the
`golden-path/v1` contract epoch, structural report-only enforcement, or the
repository-owned quality boundary introduced in 1.3.0.

## Outcome

- Bootstrap now emits a root onboarding README and a repository-owned
  `.github/workflows/quality.yml` that installs the pinned toolchain, runs
  `just init`, and executes `just ci` once.
- The generated quality workflow targets the organization `dev` branch flow;
  generated Dependabot entries also target `dev`.
- A bootstrap request may omit targets while the new repository has no truthful
  production or release representative. Adoption still requires at least one
  primary or secondary target.
- Bootstrap output is split into a small managed integration set and one-time
  repository-owned scaffold. Upgrade rewrites only the managed set.
- Materialization plans remain command output and external review evidence.
  Neither bootstrap nor upgrade writes `golden-path-plan.json` into a
  candidate repository.
- Node starters ignore exactly the five managed integration files during
  Prettier checks, so generated JSON/YAML/shell assets do not hide
  repository-owned formatting problems.

## Ownership boundary

The generator continues to manage only:

1. `.github/golden-path-assets.json`
2. `.github/golden-path-request.json`
3. `.github/golden-path.yaml`
4. `.github/workflows/developer-tooling.yml`
5. `scripts/golden-path`

The inventory itself is tracked implicitly because it cannot include its own
digest. README, source, native manifests and locks, Mise and Just files,
Dependabot configuration, and repository quality CI become repository-owned
immediately after bootstrap.

Upgrade accepts the released 1.3.0 full-scaffold inventory, verifies the prior
managed bytes and modes, and performs a bounded handoff to the five-file
inventory. Local changes to repository-owned scaffold do not create upgrade
conflicts. Deleted or modified managed assets still do.

## Compatibility

- Standard: `2026.08.2` (`preferred`)
- Contract: `golden-path/v1`
- Metadata: `golden-path-metadata/v1`
- Native roots: `golden-path-native-roots/v1`
- Exceptions: `golden-path-exceptions/v1`
- Output: `golden-path-checker-output/v1`
- Generator request: `golden-path-generator-request/v1`
- Generated asset inventory: `golden-path-generated-assets/v1`
- Materialization plan: `golden-path-materialization-plan/v1`
- Release manifest: `golden-path-release-manifest/v2`
- Source integrity: `golden-path-source-integrity/v1`
- Enforcement: `report-only`

The 2026.08.2 snapshot keeps the existing 73-rule catalog and v1 schemas. Its
accepted standard identity is bound to the organization-policy source commit
and subtree, and its changed normative bytes only advance embedded standard
identity.

The 1.3.0 release and all earlier tags and assets remain immutable. The August
4 external-tool cutoff is retained because the selected tools and locks did not
move. A separate August 7 source-integrity record binds the 1.4.0 bundle
identity without rewriting historical evidence.

## Support and upgrade policy

| Line | Status | Required action |
| --- | --- | --- |
| `1.4.x` | Preferred | Default for new bootstrap and adoption |
| `1.3.x` | Supported | Upgrade on the consumer's normal maintenance cadence |
| `1.2.x` | Supported | Upgrade on the consumer's normal maintenance cadence |
| `1.1.x` | Supported | Upgrade on the consumer's normal maintenance cadence |
| `0.2.x` | Deprecated until 2027-01-28 | Migrate through the documented supported `1.x` path |
| `0.1.x` | Deprecated until 2027-01-28 | Advance to `0.2.0`, then migrate to supported `1.x` |

A central locator update does not require every consumer to open an immediate
upgrade pull request. Existing 1.x consumers may batch compatible maintenance
unless a later release is explicitly classified as a security, integrity,
material false-failure, or unusable-workflow replacement.

## Upgrade from 1.3.0

1. Download the exact 1.4.0 release manifest, checksum list, and platform
   archive. Verify attestations against the `v1.4.0` tag, source commit,
   signer commit, and release workflow.
2. Run `golden-path upgrade` without `--write` using the repository's
   existing request and verified 1.4.0 manifest.
3. Review the plan emitted on standard output and the separate candidate. The
   candidate contains only the five managed integration files.
4. Preserve repository-owned README, source, manifests, locks, Just/Mise files,
   dependency automation, quality CI, release, and deployment behavior.
5. Run the repository's own `just ci` once and structural conformance
   separately before integrating the managed update.

Do not add a target merely to satisfy bootstrap. Declare targets only when the
repository has a real production or release representative. Adoption continues
to require that evidence.

## Rollback

Restore the complete prior 1.3.0 managed control-plane baseline from repository
history: request, metadata, generated asset inventory, bootstrap script, binary
identity, and full-commit workflow pin. Repository-owned scaffold, product
source, locks, quality commands, deployment behavior, runtime state, and durable
data do not need rollback or migration.

Never combine the workflow or setup action from one release with generated
inputs from another, move a tag, or replace published assets.

## Limitations

Generated quality CI proves only the repository's declared `just ci` contract
on its configured hosted runner. Structural conformance still does not prove
runtime behavior, hosting settings, merge protection, advisory state, release
publication, deployment, production capacity, or a target that the repository
does not actually declare. This release establishes no branch protection,
ruleset, `policy-required`, or `platform-enforced` status.
