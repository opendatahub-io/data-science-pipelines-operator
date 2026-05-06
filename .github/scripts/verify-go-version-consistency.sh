#!/usr/bin/env bash
# Verifies that all Dockerfiles using a golang or go-toolset base image
# specify a Go version consistent with the root go.mod.

set -euo pipefail

if ! command -v skopeo &>/dev/null; then
    echo "ERROR: skopeo is required but not found in PATH" >&2
    exit 1
fi

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

GOMOD_VERSION=$(grep -E '^go [0-9]' "$REPO_ROOT/go.mod" | awk '{print $2}' || true)

if [[ -z "$GOMOD_VERSION" ]]; then
    echo "ERROR: Could not extract Go version from go.mod" >&2
    exit 1
fi

echo "go.mod Go version: $GOMOD_VERSION"

resolve_go_version() {
    local image_ref="$1" tag_version="$2"
    # X.Y.Z tags are trusted without inspection — the patch version is explicit
    if [[ "$tag_version" == *.*.* ]]; then
        echo "$tag_version"
        return
    fi
    local resolved skopeo_stderr skopeo_ref="$image_ref" attempt
    if [[ "$skopeo_ref" == *:*@* ]]; then
        skopeo_ref="${skopeo_ref%%:*}@${skopeo_ref#*@}"
    fi
    skopeo_stderr=$(mktemp)
    for attempt in 1 2 3; do
        resolved=$(skopeo inspect --override-arch amd64 --override-os linux "docker://$skopeo_ref" 2>"$skopeo_stderr" \
            | grep -oE '"(VERSION|GOLANG_VERSION)=[0-9]+\.[0-9]+\.[0-9]+"' \
            | head -1 \
            | sed -E 's/"(VERSION|GOLANG_VERSION)=([0-9]+\.[0-9]+\.[0-9]+)"/\2/')
        [[ -n "$resolved" ]] && break
        [[ $attempt -lt 3 ]] && sleep 2
    done
    if [[ -z "$resolved" ]]; then
        echo "  skopeo inspect failed for $image_ref after $attempt attempts:" >&2
        cat "$skopeo_stderr" >&2
        rm -f "$skopeo_stderr"
        echo ""
        return
    fi
    rm -f "$skopeo_stderr"
    echo "$resolved"
}

ERRORS=0
CHECKED=0
FOUND=0

while IFS= read -r dockerfile; do
    relative="${dockerfile#"$REPO_ROOT"/}"
    FOUND=$((FOUND + 1))
    while IFS= read -r line; do
        docker_version=$(echo "$line" | sed -E 's/.*[Ff][Rr][Oo][Mm][[:space:]]+(--[^[:space:]]+[[:space:]]+)*(golang|[^[:space:]]*go-toolset):([0-9]+\.[0-9]+(\.[0-9]+)?).*/\3/')

        if [[ ! "$docker_version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
            docker_version=$(echo "$line" | sed -E 's/.*go-toolset:([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/')
        fi

        if [[ ! "$docker_version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
            echo "ERROR: Could not parse Go version from line in $relative: $line" >&2
            ERRORS=$((ERRORS + 1))
            continue
        fi

        image_ref=$(echo "$line" | sed -E 's/^[[:space:]]*(FROM[[:space:]]+(--[^[:space:]]+[[:space:]]+)*|ARG[[:space:]]+[^=]+=)([^[:space:]]+)[[:space:]]*.*/\3/')

        resolved_version=$(resolve_go_version "$image_ref" "$docker_version")

        if [[ -z "$resolved_version" ]]; then
            echo "ERROR: Could not resolve Go version from image $image_ref in $relative" >&2
            ERRORS=$((ERRORS + 1))
            continue
        fi

        CHECKED=$((CHECKED + 1))

        if [[ "$resolved_version" != "$docker_version" ]]; then
            echo "  INFO: $relative tag $docker_version resolved to Go $resolved_version"
        fi

        if [[ "$resolved_version" != "$GOMOD_VERSION" ]]; then
            echo "MISMATCH: $relative has Go $resolved_version, but go.mod requires $GOMOD_VERSION" >&2
            ERRORS=$((ERRORS + 1))
        else
            echo "  OK: $relative (Go $resolved_version)"
        fi
    done < <(grep -iE '^\s*(FROM|ARG).*go-toolset:' "$dockerfile" || grep -iE '^FROM[[:space:]]+(--[^[:space:]]+[[:space:]]+)*(golang):' "$dockerfile" || true)
done < <(cd "$REPO_ROOT" && (git ls-files -z '*Dockerfile*' | xargs -0 grep -liE -- '(FROM[[:space:]]+(--[^[:space:]]+[[:space:]]+)*(golang|[^[:space:]]*go-toolset):|ARG[[:space:]]+.*go-toolset:)' | sed "s|^|$REPO_ROOT/|") || true)

echo ""

if [[ $FOUND -eq 0 ]]; then
    echo "ERROR: No Dockerfiles with Go base images found." >&2
    exit 1
fi

if [[ $CHECKED -eq 0 ]]; then
    echo "ERROR: Found $FOUND Dockerfile(s) with Go base images, but could not parse any Go version." >&2
    exit 1
fi

if [[ $ERRORS -gt 0 ]]; then
    echo "FAILED: $ERRORS error(s) found when checking Go base image stages against go.mod ($GOMOD_VERSION)." >&2
    exit 1
fi

echo "PASSED: All $CHECKED Go base image stage(s) use Go $GOMOD_VERSION, matching go.mod."
