# 7. Deployment view

State: **Target**

Source, build, tests, and canonical CI are As-built in this repository. The private GitHub Package and installed user Skill are not deployed by a pull request.

Before promotion, the exact materialized `dev` candidate must pass isolated Codex and Claude Code installation, fresh-session explicit-invocation, synthetic journey, and read-only pilot validation. Candidate validation does not publish the package, install into a consumer repository, mutate a consumer, or run consumer canonical CI centrally.

Publication remains Target until an immutable `golden-path-agent-v1.0.0` tag points to the independently reviewed and validated `main` commit and the dedicated workflow completes successfully. The workflow retains the canonical-validation tarball, records its SHA-256 and SRI plus source, publisher workflow, access declaration, and toolchain evidence, publishes that exact file, and compares registry integrity. Post-publication evidence then installs the exact registry coordinate into fresh Codex and Claude Code environments and exercises explicit activation, update, and rollback installation procedures. Publication and that post-publication smoke are distinct from pre-release candidate validation.
