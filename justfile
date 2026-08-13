set dotenv-load := false
set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

mise := "env MISE_CONFIG_DIR=${TMPDIR:-/tmp}/golden-path-agent-mise-config mise"
pnpm := "env MISE_CONFIG_DIR=${TMPDIR:-/tmp}/golden-path-agent-mise-config mise exec node@24.18.1 -- ./scripts/pnpm"

default:
    @just --list

init:
    {{mise}} install --locked actionlint@1.7.12 github-cli@2.97.0 just@1.57.0 node@24.18.1
    {{pnpm}} install --frozen-lockfile

format:
    {{pnpm}} format

format-check:
    {{pnpm}} format:check

lint:
    {{pnpm}} lint
    {{mise}} exec actionlint@1.7.12 -- actionlint

lint-fix:
    {{pnpm}} lint:fix

typecheck:
    {{pnpm}} typecheck

test:
    {{pnpm}} test

docs-check:
    bash scripts/docs/check-contract.sh

check: format-check lint typecheck test docs-check

build:
    {{pnpm}} build

package-check:
    {{pnpm}} package:check

ci: check build package-check
