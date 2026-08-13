# 5. Building-block view

State: **As-built**

- `authority` resolves the exact governance commit and reads an explicit path allowlist.
- `repository` uses Git's bounded tracked/untracked file query to inventory root and nested native manifests and locks, repository-owned native-root declarations, applicability, Just recipes, and workflow calls without executing repository recipes.
- `report` binds observation time, package and installed Skill identity, authority digests, inventory query, repository state, and report-only observations without promoting them to policy or platform enforcement.
- `plan` validates external request and plan documents, applies deterministic replacements, and enforces source, path, symlink, overwrite, and repository-state boundaries.
- `skill` installs host-specific explicit-invocation content while protecting unmanaged and locally modified directories.
- `cli` exposes the user commands and keeps publication and repository CI out of the command surface.
