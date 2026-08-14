# 11. Risks and technical debt

State: **Open**

- Private package onboarding depends on per-developer package read access, a
  least-privilege `read:packages` credential, and separate authenticated `gh`
  access to the current authority. Documentation must keep those identities and
  troubleshooting boundaries distinct without recording tokens.
- Each new exact package release still requires terminal publication evidence
  and fresh Codex and Claude Code installation smoke. Routine consumer mutation
  or central reruns of consumer CI are not release gates.
- Atomic multi-file apply cannot be transactional across a process or host crash. Plans are intentionally small, destination state is hash-bound, and the owning Git repository provides review and recovery.
- Apply rechecks every existing parent and destination immediately before each atomic rename, but JavaScript has no descriptor-relative `openat`/`renameat` surface. A hostile local process that concurrently swaps a parent in the remaining syscall interval is outside this developer-local support tool's trust boundary; do not run apply against an actively adversarial worktree.
