#!/usr/bin/env bash

# Repository-owned copy of the 5010-arc42-v1 reference checker from
# 5010-dev/.github/scripts/docs/check-contract.sh.
set -euo pipefail

profile_id="5010-arc42-v1"
target_dir="$(pwd)"

usage() {
    cat <<'EOF'
Usage:
  check-contract.sh [--target PATH]

Options:
  --target PATH  Repository or engineering-layer root to validate.
  -h, --help     Show this help.
EOF
}

while [[ "$#" -gt 0 ]]; do
    case "$1" in
        --target)
            if [[ -z "${2:-}" ]]; then
                echo "error: --target requires a value" >&2
                usage >&2
                exit 2
            fi
            target_dir="$2"
            shift 2
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            echo "error: unknown option: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [[ ! -d "$target_dir" ]]; then
    echo "error: target directory does not exist: $target_dir" >&2
    exit 2
fi

target_root="$(cd "$target_dir" && pwd -P)"
errors=0

report() {
    echo "error: $*" >&2
    errors=$((errors + 1))
}

required_architecture=(
    README.md
    01-introduction-goals.md
    02-constraints.md
    03-context-scope.md
    04-solution-strategy.md
    05-building-block-view.md
    06-runtime-view.md
    07-deployment-view.md
    08-crosscutting-concepts.md
    09-architecture-decisions.md
    10-quality.md
    11-risks-technical-debt.md
    12-glossary.md
)

for required_file in docs/README.md docs/decisions/README.md; do
    if [[ ! -s "$target_root/$required_file" ]]; then
        report "required documentation file missing or empty: $required_file"
    fi
done

for name in "${required_architecture[@]}"; do
    path="$target_root/docs/architecture/$name"
    if [[ ! -s "$path" ]]; then
        report "required architecture file missing or empty: docs/architecture/$name"
    fi
done

architecture_index="$target_root/docs/architecture/README.md"
if [[ -f "$architecture_index" ]]; then
    if ! grep -Fq 'Authority: **Canonical**' "$architecture_index"; then
        report "docs/architecture/README.md must declare canonical authority"
    fi
    if ! grep -Eq '^Scope: \*\*.+\*\*$' "$architecture_index"; then
        report "docs/architecture/README.md must declare a non-empty Scope"
    fi
    if ! grep -Fq "Organization profile: **$profile_id**" "$architecture_index"; then
        report "docs/architecture/README.md must adopt $profile_id"
    fi
fi

for name in "${required_architecture[@]:1}"; do
    path="$target_root/docs/architecture/$name"
    if [[ -f "$path" ]] &&
        ! grep -Eq '^State: \*\*(As-built|Target|Open|Deprecated)\*\*$' "$path"; then
        report "architecture chapter has no recognized default State: docs/architecture/$name"
    fi
done

decisions_index="$target_root/docs/decisions/README.md"
architecture_decisions="$target_root/docs/architecture/09-architecture-decisions.md"

if compgen -G "$target_root/docs/decisions/[0-9][0-9][0-9][0-9]-*.md" >/dev/null; then
    for adr_file in "$target_root"/docs/decisions/[0-9][0-9][0-9][0-9]-*.md; do
        filename="$(basename "$adr_file")"
        if ! grep -Eq '^(- )?Status: \*{0,2}(Proposed|Accepted|Superseded|Deprecated|Rejected)\*{0,2}$' "$adr_file"; then
            report "ADR has no recognized lifecycle status: docs/decisions/$filename"
        fi
        if [[ -f "$decisions_index" ]] && ! grep -Fq "$filename" "$decisions_index"; then
            report "ADR is missing from docs/decisions/README.md: $filename"
        fi
        if [[ -f "$architecture_decisions" ]] && ! grep -Fq "$filename" "$architecture_decisions"; then
            report "ADR is missing from architecture chapter 9: $filename"
        fi
    done
else
    report "at least one indexed architecture decision is required"
fi

check_markdown_file() {
    local file="$1"
    local display="${file#"$target_root"/}"
    local destination path resolved

    if grep -nE '[[:blank:]]+$' "$file" >/dev/null; then
        report "trailing whitespace: $display"
    fi

    if grep -nE '\{\{[A-Z_][A-Z_]*\}\}' "$file" >/dev/null; then
        report "unresolved scaffold token: $display"
    fi

    while IFS= read -r destination; do
        case "$destination" in
            http://* | https://* | mailto:* | \#*)
                continue
                ;;
        esac

        path="${destination%%#*}"
        path="${path%%\?*}"
        path="${path#<}"
        path="${path%>}"
        if [[ -z "$path" ]]; then
            continue
        fi

        if [[ "$path" == /* ]]; then
            resolved="$target_root$path"
        else
            resolved="$(dirname "$file")/$path"
        fi

        if [[ ! -e "$resolved" ]]; then
            report "broken local Markdown link in $display: $destination"
        fi
    done < <(perl -ne 'while (/\]\(([^)]+)\)/g) { print "$1\n" }' "$file")
}

if [[ -d "$target_root/docs" ]]; then
    while IFS= read -r -d '' markdown_file; do
        check_markdown_file "$markdown_file"
    done < <(find "$target_root/docs" -type f -name '*.md' -print0)
fi

if [[ "$errors" -ne 0 ]]; then
    printf 'arc42 contract check: FAILED (%d problem(s))\n' "$errors" >&2
    exit 1
fi

printf 'arc42 contract check: OK (%s)\n' "$profile_id"
