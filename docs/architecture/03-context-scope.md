# 3. Context and scope

State: **As-built**

The developer invokes the packaged CLI locally. The CLI reads the target Git repository with `git`, uses Just's own summary mode for recipe discovery, parses workflow YAML, and resolves the current governance commit and allowlisted raw files through authenticated `gh`.

The system may write only official user Skill directories through `skill install`, an external plan file through `plan`, and explicitly planned repository files through `apply`. It does not write organization state, mutate remote consumers, run consumer CI, publish, deploy, approve, or interpret live organization conformance.
