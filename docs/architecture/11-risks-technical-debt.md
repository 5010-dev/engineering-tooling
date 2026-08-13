# 11. Risks and technical debt

State: **Open**

- Actual private package publication and package permission configuration require separately authorized release work and terminal workflow evidence.
- Fresh Codex and Claude Code sessions must confirm explicit discovery and non-implicit invocation after publication.
- Design System, Collector, and Quant read-only pilots remain evidence work; they must not mutate consumers or rerun canonical CI centrally.
- Atomic multi-file apply cannot be transactional across a process or host crash. Plans are intentionally small, destination state is hash-bound, and the owning Git repository provides review and recovery.
- Apply rechecks every existing parent and destination immediately before each atomic rename, but JavaScript has no descriptor-relative `openat`/`renameat` surface. A hostile local process that concurrently swaps a parent in the remaining syscall interval is outside this developer-local support tool's trust boundary; do not run apply against an actively adversarial worktree.
