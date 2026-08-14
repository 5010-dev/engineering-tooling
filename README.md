# Golden Path Agent

`@5010-dev/golden-path-agent` is a private, developer-installed package that
helps a developer or coding agent inspect and apply the FiftyTen Developer
Tooling Golden Path without becoming a repository or organization control
plane.

The active policy and guidance remain reviewed source in
[`5010-dev/.github`](https://github.com/5010-dev/.github). Consumer repositories
continue to own their manifests, locks, toolchain selectors, Just graph,
canonical CI, security routing, and release or deployment workflows.

## Package access and authentication

`@5010-dev/golden-path-agent` is a private GitHub Package. Each developer needs
`5010-dev` organization membership, package `Read` through an authorized team
or user permission, and a personal access token (classic) with the
`read:packages` scope. If the organization uses SAML SSO, authorize that token
for `5010-dev` before using it.

Configure the `5010-dev` scope in the user-level `~/.npmrc`; do not add a package
token to a repository-owned `.npmrc`:

```ini
@5010-dev:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${NODE_AUTH_TOKEN}
```

Export `NODE_AUTH_TOKEN` from an approved local secret store for the install
session. Do not paste the token into a tracked file, command argument, shell
history, issue, pull request, or support transcript. The token authenticates
package downloads only; `gh` authentication is a separate requirement for
reading the current Golden Path authority.

Verify package access before installation:

```sh
pnpm view @5010-dev/golden-path-agent@1.0.1 version \
  --registry=https://npm.pkg.github.com
gh auth status --hostname github.com
```

Interpret common registry failures as follows:

| Result             | Check                                                                                                                                                                        |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `401 Unauthorized` | `NODE_AUTH_TOKEN` is present, current, and referenced by the user-level npm configuration                                                                                    |
| `403 Forbidden`    | The token has `read:packages`, any required SSO authorization is active, and the user has package read access                                                                |
| `404 Not Found`    | The exact package coordinate and registry are correct, then verify organization membership and package read access because private-package denial may be masked as not found |

For missing package access or general installation support, [create an
Engineering issue in Linear](https://linear.new?team=ENG). Never include the
token or other sensitive material in that issue.

## Install the package and Skill

After authentication, install the exact package version globally so the
`golden-path-agent` executable remains available when Codex or Claude Code later
invokes the installed Skill:

```sh
pnpm add --global @5010-dev/golden-path-agent@1.0.1
golden-path-agent skill install --host all
golden-path-agent skill check --host all
```

Package updates and rollback are exact reinstalls of a reviewed version. There
is no locator, update channel, or compatibility bridge to the retired
`golden-path` executable line.

Runtime authority reads require GitHub CLI `gh` authenticated to `github.com`
with read access to `5010-dev/.github`. The package does not capture, persist,
or print the authentication token.

The one-shot `pnpm dlx` form may inspect package metadata or run a bounded
command, but using it only for `skill install` does not install the executable
persistently and is therefore not the supported host-install procedure.

The installer writes only the official user-level Skill directories:

- Codex: `$HOME/.agents/skills/golden-path`
- Claude Code: `$HOME/.claude/skills/golden-path`

It refuses to replace an unmanaged directory and refuses to overwrite locally
modified managed content unless `--force` is supplied explicitly. It also
fails closed when the home or host Skill parent is a symbolic link.

## Repository support commands

All commands are local developer support. They are not organization approval,
merge enforcement, or repository CI.

```sh
golden-path-agent info --root . --json
golden-path-agent doctor --root .
golden-path-agent check --root . --json
golden-path-agent plan --root . --request /tmp/request.json --output /tmp/plan.json
golden-path-agent apply --root . --plan /tmp/plan.json \
  --approved-plan-sha256 sha256:<exact-plan-digest>
```

`info`, `doctor`, `check`, and `plan` are read-only. JSON inventory records the
observation interval, package content identity, both installed Skill states,
the exact bounded Git file query, repository applicability, native roots, and
authority file digests. A pnpm workspace is one native dependency root when its
reviewed workspace file, selected package manifests, root lock, and default
shared-lock setting make that boundary unambiguous. Exclusions remain effective
regardless of pattern order wherever the negative glob itself matches. Wildcards
do not implicitly select dot-directories, and a trailing globstar also matches its
base directory, following pnpm. Other descendant fixture manifests inherit that
dependency boundary, while excluded roots, member-level competing locks,
complete nested workspaces, and `sharedWorkspaceLockfile: false` roots remain
visible. Repeated same-profile roots remain `pending-classification` unless the
repository owns an explicit
`.github/golden-path-native-roots.yaml` declaration.

`plan` binds its observation interval, exact package content digest, selected
normative and applicable profile document digests, copy-once source digests,
repository state, and existing destination content and mode. It prints the
digest of the exact external plan bytes for review. `apply` is the only
repository-writing command. It accepts only the current Golden Path's named
copy-once examples, resolves current `.github/main` again, requires it to remain
the exact planned commit, and re-reads every selected authority file before
verifying the approved plan, package, repository, destination content, and mode.
Existing file modes are preserved; new files use `0644`. Use `apply --json` to
retain the apply interval, approved digest, authority repository/ref/commit,
normative and copy-once file digests, and written destinations as evidence.

Example request:

```json
{
  "schemaVersion": 1,
  "copies": [
    {
      "source": "canonical-ci",
      "destination": ".github/workflows/ci.yml",
      "overwrite": false,
      "replacements": []
    }
  ]
}
```

The plan file must be outside the target repository. A copied example becomes
repository-owned immediately; this package does not record managed ownership,
upgrade it, or open consumer pull requests.

For general support, [create an Engineering issue in
Linear](https://linear.new?team=ENG) for triage by the `engineering-tooling`
maintainers. Sensitive security reports must follow
[SECURITY.md](./SECURITY.md). Linear is the support intake and work-tracking
system; reviewed source at `5010-dev/.github@main` remains the policy and Golden
Path authority. `SECURITY.md` is also the repository source of truth for
supported-version windows, the first operational review, and retirement/removal
conditions.

## Development

```sh
just init
just check
just ci
```

Publication is separate from quality validation. Only an immutable
`golden-path-agent-vX.Y.Z` tag pointing at the validated `main` commit may
publish the matching private GitHub Package. Canonical validation packs once,
records the tarball SHA-256 and SRI, tests that exact file in an isolated
consumer, and the release workflow publishes the same file. Candidate evidence
labels the repository-build content digest separately from the digest reported by
that packed installation; the tarball identity binds the two materializations.

## Historical release line

Repository tags and public releases `v0.1.0` through `v1.6.1`, including their
checksums and attestations, are immutable audit history for the retired Go
executable. They are not versions of this npm package and are not an active
compatibility or support boundary.
