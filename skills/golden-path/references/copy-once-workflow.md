# Copy-once workflow

## Request shape

Write the JSON request outside the target repository:

```json
{
  "schemaVersion": 1,
  "copies": [
    {
      "source": "node-toolchain",
      "destination": "mise.toml",
      "overwrite": true,
      "replacements": [
        {
          "match": "<EXACT_SUPPORTED_MISE_VERSION>",
          "value": "2026.7.18",
          "expectedOccurrences": 1
        },
        {
          "match": "<EXACT_JUST_VERSION>",
          "value": "1.57.0",
          "expectedOccurrences": 1
        },
        {
          "match": "<EXACT_SUPPORTED_NODE_PATCH>",
          "value": "24.18.1",
          "expectedOccurrences": 1
        }
      ]
    }
  ]
}
```

Allowed source names are `canonical-ci`, `dependabot`, `go-just`, `native-roots`, `node-just`, `node-toolchain`, and `python-just`.

## Commands

```sh
golden-path-agent plan \
  --root . \
  --request /tmp/golden-path-request.json \
  --output /tmp/golden-path-plan.json

golden-path-agent apply \
  --root . \
  --plan /tmp/golden-path-plan.json \
  --approved-plan-sha256 sha256:<exact-plan-digest>
```

Planning rejects an in-repository request or output, duplicate destinations, unknown sources, unresolved exact-value tokens, silent replacement-count drift, symlink traversal, and an unapproved overwrite. Apply requires the exact digest printed for the reviewed plan, re-fetches every source from the planned commit, and revalidates all hashes and repository state before writing.

After apply, inspect and edit the copied source for the repository's actual commands, profiles, permissions, runners, release boundaries, and owners. The package provides no update or synchronization operation.
