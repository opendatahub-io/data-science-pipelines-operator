#!/usr/bin/env bash

set -e

KUSTOMIZE_BIN="${KUSTOMIZE_BIN:-./bin/kustomize}"

dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)
      dir="${2:-}"
      shift 2
      ;;
    -h|--help)
      echo "Usage: scripts/validate-kustomize.sh [--dir DIR]" >&2
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

if [[ ! -x "$KUSTOMIZE_BIN" ]]; then
  echo "kustomize binary not found/executable at: $KUSTOMIZE_BIN" >&2
  exit 1
fi

declare -a dirs=()
if [[ -n "$dir" ]]; then
  dirs=("$dir")
else
  while IFS= read -r kf; do
    d="$(dirname "$kf")"
    dirs+=("$d")
  done < <(find config -name kustomization.yaml -print | LC_ALL=C sort)
fi

if [[ ${#dirs[@]} -eq 0 ]]; then
  echo "No kustomization.yaml files found under ./config" >&2
  exit 1
fi

for d in "${dirs[@]}"; do
  if [[ "$d" == "config/manifests" ]]; then
    echo "Skipping: kustomize build ${d} (bundle/OLM manifests are not part of deploy validation)"
    continue
  fi

  echo "Running: kustomize build ${d}"

  out="$(mktemp)"
  trap 'rm -f "$out"' EXIT

  if ! "$KUSTOMIZE_BIN" build "$d" >"$out" 2>&1; then
    echo "Kustomize build failed for ${d}:"
    cat "$out"
    exit 1
  fi

  if grep -qE '^# Warning:' "$out"; then
    echo "Kustomize emitted warnings for ${d}:"
    grep -nE '^# Warning:' "$out"
    exit 1
  fi

  rm -f "$out"
  trap - EXIT
done
