# 7. Deployment view

State: **Target**

Source, build, tests, and canonical CI are As-built in this repository. The private GitHub Package and installed user Skill are not deployed by a pull request.

Publication remains Target until an immutable `golden-path-agent-v1.0.0` tag points to the validated `main` commit and the dedicated workflow completes successfully. The workflow retains the canonical-validation tarball, records its SHA-256 and SRI plus source, publisher workflow, access declaration, and toolchain evidence, publishes that exact file, and compares registry integrity. Fresh-session Codex and Claude Code activation and read-only pilot observations remain post-publication evidence; no consumer mutation is authorized by this rewrite.
