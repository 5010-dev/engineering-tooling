---
name: golden-path
description: Inspect and change a FiftyTen repository through the contract-backed, repository-owned Developer Tooling Golden Path. Use only when the user explicitly invokes this golden-path Skill for tooling bootstrap, adoption, retirement, diagnostics, planning, or copy-once application. Do not use implicitly, as repository CI, or as organization approval.
---

# FiftyTen Golden Path

Treat this Skill as procedure and local assistance only. Keep the current repository and reviewed `5010-dev/.github` source authoritative.

## Establish authority

1. Read the repository's agent instructions and the organization `CONTRIBUTING.md` before changing files.
2. Run `golden-path-agent info --root . --json` and retain the exact `.github/main` commit and selected source hashes. Stop write planning if the source is unavailable.
3. Run `golden-path-agent doctor --root .` for a factual readiness inventory. Treat its output as developer assistance, never as conformance or merge approval.
4. Read [authority-boundaries.md](references/authority-boundaries.md) before an architecture-significant or cross-repository change.

## Preserve repository ownership

1. Inventory actual source, manifests, locks, toolchain selectors, native roots, Just recipes, workflows, security routing, and release or deployment boundaries.
2. Read only the standard profiles and Golden Path journey applicable to that inventory.
3. Preserve working repository-native authorities. Do not create a central locator, managed footprint, generator/upgrader, shared checker, live report, dependency compiler, approval queue, or consumer registry.
4. Prefer ecosystem and platform tooling. Add custom logic only when the owning repository records why existing tools are insufficient.
5. Do not add compatibility or migration machinery for unreleased development intermediates.

## Plan and apply copy-once examples

1. Read [copy-once-workflow.md](references/copy-once-workflow.md).
2. Select only named examples applicable to the repository. Write the request and generated plan outside the repository.
3. Inspect the complete plan, source hashes, replacements, destinations, and repository-state binding before asking for write approval.
4. Record the exact plan digest printed by `golden-path-agent plan`. Invoke `golden-path-agent apply --approved-plan-sha256 <digest>` only after the user explicitly approves that digest and the complete plan. Never treat a copied file as managed by this package.
5. Replace example choices and complete repository-specific behavior through normal repository review.

## Validate and hand off

1. Run repository-local checks for changed behavior and then the repository's canonical `just ci` exactly once through its owning path.
2. Use `golden-path-agent check --root . --json` only as a report-only factual inventory. It must not run or substitute `just ci`.
3. Separate local checks, GitHub Actions, publication, deployment, security closure, and platform enforcement claims.
4. Report the exact authority commit, diff, validation commands, unresolved observations, and any external release or deployment evidence still required.
