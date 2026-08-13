# ADR 0001: Replace the retired control plane with a developer-installed agent package

Status: **Accepted**

## Context

The current Developer Tooling Standard declares no active central executable tooling. ENG-219 retired the former locator, managed footprint, snapshots, checker, generator, dependency compiler, live report, and shared workflows. ENG-221 accepts a local assistance package only if it preserves the current policy and repository ownership boundaries.

## Decision

Replace the unreleased main-tree implementation in place with private package `@5010-dev/golden-path-agent@1.0.0`, executable `golden-path-agent`, and one developer-installed `golden-path` Skill for Codex and Claude Code.

The Skill is explicit-only. Read operations resolve current reviewed source. Copy assistance accepts only named Golden Path examples, creates an external hash-bound plan, requires a separate apply invocation, and records no repository managed footprint. Publication uses the package-specific tag namespace only from validated `main`.

Boundary classification: unreleased — corrected in place.

## Consequences

The repository retains its identity, history, CODEOWNERS, security reporting, and historical tags and releases. The new package has no compatibility relationship with the retired executable line. Consumer repositories continue to own every executable and operational authority. Publication, host activation, and pilot observations remain separate evidence after review.

## Rejected alternatives

- Reviving the retired control plane or a smaller shared checker.
- Shipping old and new implementations together.
- Embedding normative snapshots or fetching mutable source in consumer CI.
- Automatic consumer application, upgrade campaigns, queues, or central approval.
- A custom implementation of Git, GitHub, Just, YAML, schema validation, formatting, lint, type, test, or package inspection behavior already provided off the shelf.
