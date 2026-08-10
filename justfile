set dotenv-load := false

default_evaluated_at := env_var_or_default("GOLDEN_PATH_EVALUATED_AT", `date -u +'%Y-%m-%dT%H:%M:%SZ'`)

default:
    @just --list

init:
    mise install
    go mod download

format:
    gofmt -w .
    go tool goimports -w .

format-check:
    test -z "$(gofmt -l .)"
    test -z "$(go tool goimports -l .)"

lint:
    go vet ./...
    golangci-lint run
    actionlint

module-check:
    go mod tidy -diff
    go mod verify

test:
    go test -mod=readonly -race ./...

build:
    go build -mod=readonly -trimpath -o bin/golden-path ./cmd/golden-path

package-check source_commit:
    go run -mod=readonly ./cmd/release-package --source . --dist dist
    version="$(go run -mod=readonly ./cmd/golden-path --version)"; version="${version#golden-path }"; test -n "$version"; \
    cp compatibility/manifest.json dist/compatibility-manifest.json; \
    cp compatibility/golden-path-checker-compatibility-v1.schema.json dist/; \
    cp standards/snapshots/2026.08.6/manifest.json dist/standard-snapshot-manifest.json; \
    cp release/RELEASE_NOTES.md dist/RELEASE_NOTES.md; \
    cp release/golden-path-release-manifest-v1.schema.json dist/; \
    cp release/golden-path-release-manifest-v2.schema.json dist/; \
    cp release/golden-path-source-integrity-v1.schema.json dist/; \
    cp release/golden-path-tooling-cutoff-v1.schema.json dist/; \
    cp release/source-integrity-2026-08-10-1.6.0.json dist/source-integrity.json; \
    cp release/tooling-cutoff-2026-08-04.json dist/tooling-cutoff.json; \
    cp generator/schemas/*.json dist/; \
    cp standards/snapshots/2026.08.6/schemas/golden-path-dependency-*.schema.json dist/; \
    go run -mod=readonly ./cmd/release-manifest --source . --dist dist --tag "v$version" --source-commit "{{ source_commit }}" --output dist/release-manifest.json

conformance evaluated_at=default_evaluated_at:
    test -n "{{ evaluated_at }}"
    go run -mod=readonly ./cmd/golden-path check --root . --evaluated-at "{{ evaluated_at }}" --json-output - >/dev/null

check evaluated_at=default_evaluated_at: module-check format-check lint test
    just conformance "{{ evaluated_at }}"

ci evaluated_at=default_evaluated_at: (check evaluated_at) build
    just package-check "$(git rev-parse HEAD)"
