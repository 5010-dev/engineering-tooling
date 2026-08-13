# Security policy

## Support channels and ownership

The `5010-dev/engineering-tooling` maintainers own support for
`@5010-dev/golden-path-agent`. General installation, behavior, and documentation
questions belong in this repository's GitHub Issues. Do not put credentials,
vulnerability details, or other sensitive material in a public issue.

Supported developer hosts are Codex and Claude Code. Host support means the
package's documented user-level Skill installation and explicit invocation
contract; it does not make either host a repository CI or organization approval
layer.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting for this repository. Do not open a
public issue or pull request for a suspected vulnerability.

Include the affected package version, reproduction steps, impact, and any
suggested mitigation. Do not include credentials, customer data, or other
sensitive material beyond what is strictly necessary to reproduce the issue.

## Supported versions

Before the first publication there is no released package version and no active
support window. After publication:

- the latest stable package version is supported;
- the immediately previous stable version remains supported for rollback for
  30 calendar days after the newer stable version is published; and
- older exact versions remain immutable audit and recovery artifacts without an
  active support contract.

Public repository releases `v0.1.0` through `v1.6.1` belong to the retired Go
executable line. They remain immutable audit history but are not active package
versions or compatibility targets.

## Operational review

The first operational review is due 90 calendar days after
`@5010-dev/golden-path-agent@1.0.0` is published. Until publication, the review
date is intentionally recorded as the rule `1.0.0 publication timestamp + 90
days`; publication evidence must replace this rule with the resulting date.

After that review, a reviewed issue reassesses the package only after a material
Developer Tooling Standard change, a supported-host contract change, a security
incident, or repeated support failure. Routine releases do not create a separate
review program.

## Retirement and removal

Retirement requires a separate reviewed decision confirming that no supported
workflow remains, that a maintained replacement or documentation-led path
exists, and that an owner and user-communication plan are recorded. Except for a
severe security or legal necessity, retirement records the package as deprecated
or unsupported and preserves package versions, Git history, tags, releases, and
evidence.

Removal never silently uninstalls developer Skills, mutates consumer
repositories, publishes a compatibility retirement release, or creates an
organization migration program.
