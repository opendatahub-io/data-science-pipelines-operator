#!/usr/bin/env bash
set -euo pipefail

# Build DSP images from source and push to the local registry.
# Requires: GIT_WORKSPACE, REGISTRY_ADDRESS, GITHUB_REPOSITORY_OWNER, GITHUB_SHA
# Optional: PR_HEAD_OWNER, PR_HEAD_BRANCH (set by workflow for fork PR detection)

# Check if a branch exists in a remote repo. Returns:
#   0 — branch exists
#   2 — branch or repo not found (safe to fall through)
#   1 — transport/auth/service error (should fail hard)
check_remote_ref() {
  local repo_url="$1"
  local branch="$2"
  local rc=0
  git ls-remote --exit-code --heads "$repo_url" "refs/heads/${branch}" >/dev/null 2>&1 || rc=$?
  if [ "$rc" -eq 0 ]; then
    return 0
  elif [ "$rc" -eq 2 ]; then
    return 2
  else
    return 1
  fi
}

# Clone a repo with 3-tier fallback:
#   1. <fork_owner>/<repo> @ <head_branch>  (skipped when fork_owner is empty or equals upstream)
#   2. <upstream_owner>/<repo> @ <head_branch>  (skipped when head_branch is empty)
#   3. <upstream_owner>/<repo> @ <default_branch>
# Uses ls-remote to distinguish missing refs (fall through) from transport errors (fail hard).
clone_with_fallback() {
  local repo_name="$1"
  local target_dir="$2"
  local upstream_owner="$3"
  local default_branch="$4"
  local fork_owner="${PR_HEAD_OWNER:-}"
  local head_branch="${PR_HEAD_BRANCH:-}"

  if [ -n "$fork_owner" ] && [ "$fork_owner" != "$upstream_owner" ] && [ -n "$head_branch" ]; then
    local fork_url="https://github.com/${fork_owner}/${repo_name}.git"
    echo "Checking ${fork_owner}/${repo_name} for branch ${head_branch}..."
    local ref_rc=0
    check_remote_ref "$fork_url" "$head_branch" || ref_rc=$?
    if [ "$ref_rc" -eq 0 ]; then
      echo "Branch found, cloning ${fork_owner}/${repo_name}@${head_branch}..."
      git clone --depth 1 --branch "$head_branch" "$fork_url" "$target_dir" 2>&1
      echo "Cloned from fork: ${fork_owner}/${repo_name}@${head_branch}"
      return 0
    elif [ "$ref_rc" -eq 1 ]; then
      echo "ERROR: transport/auth failure reaching ${fork_url}" >&2
      return 1
    fi
    echo "Fork ${fork_owner}/${repo_name}@${head_branch} not found, trying next tier"
  fi

  if [ -n "$head_branch" ]; then
    local upstream_url="https://github.com/${upstream_owner}/${repo_name}.git"
    echo "Checking ${upstream_owner}/${repo_name} for branch ${head_branch}..."
    local ref_rc=0
    check_remote_ref "$upstream_url" "$head_branch" || ref_rc=$?
    if [ "$ref_rc" -eq 0 ]; then
      echo "Branch found, cloning ${upstream_owner}/${repo_name}@${head_branch}..."
      git clone --depth 1 --branch "$head_branch" "$upstream_url" "$target_dir" 2>&1
      echo "Cloned from upstream: ${upstream_owner}/${repo_name}@${head_branch}"
      return 0
    elif [ "$ref_rc" -eq 1 ]; then
      echo "ERROR: transport/auth failure reaching ${upstream_url}" >&2
      return 1
    fi
    echo "Branch ${head_branch} not found in ${upstream_owner}/${repo_name}, trying next tier"
  fi

  echo "Cloning ${upstream_owner}/${repo_name} branch ${default_branch}..."
  git clone --depth 1 --branch "$default_branch" "https://github.com/${upstream_owner}/${repo_name}.git" "$target_dir" 2>&1
  echo "Cloned from upstream default: ${upstream_owner}/${repo_name}@${default_branch}"
}

echo "---------------------------------"
echo "Building DSP images from source"
echo "---------------------------------"

upstream_owner="${GITHUB_REPOSITORY_OWNER:-opendatahub-io}"
dsp_default_branch="master"
if [ "$upstream_owner" = "red-hat-data-services" ]; then
  dsp_default_branch="main"
fi

tag="${GITHUB_SHA:0:7}"
if [ -z "$tag" ]; then
  tag="local"
fi

dsp_dir="${GIT_WORKSPACE}/_dsp_source"
rm -rf "$dsp_dir"

clone_with_fallback "data-science-pipelines" "$dsp_dir" "$upstream_owner" "$dsp_default_branch"

registry="${REGISTRY_ADDRESS}"
bake_file="${GIT_WORKSPACE}/.github/resources/docker-bake.hcl"

echo "Configuring buildx for insecure registry ${registry}..."
buildx_config="/tmp/buildkitd-ci.toml"
REGISTRY_ADDRESS="${registry}" envsubst < "${GIT_WORKSPACE}/.github/resources/buildkitd.toml" > "$buildx_config"
docker buildx create --name dsp-builder --driver docker-container \
  --driver-opt network=host --config "$buildx_config" --use 2>/dev/null || \
  docker buildx use dsp-builder

echo "Building all DSP images in parallel with docker bake..."
BUILDX_BAKE_ENTITLEMENTS_FS=0 TAG="${tag}" REGISTRY="${registry}" KFP_REPO="${dsp_dir}" \
  docker buildx bake -f "$bake_file" --push

params_file="${GIT_WORKSPACE}/config/base/params.env"

if grep -q "IMAGES_ARGO_WORKFLOWCONTROLLER" "$params_file"; then
  argo_default_branch="main"

  argo_dir="${GIT_WORKSPACE}/_argo_source"
  rm -rf "$argo_dir"

  clone_with_fallback "argo-workflows" "$argo_dir" "$upstream_owner" "$argo_default_branch"

  echo "Building Argo images in parallel with docker bake..."
  BUILDX_BAKE_ENTITLEMENTS_FS=0 TAG="${tag}" REGISTRY="${registry}" ARGO_REPO="${argo_dir}" \
    docker buildx bake -f "$bake_file" argo --push

  sed -i "s|IMAGES_ARGO_WORKFLOWCONTROLLER=.*|IMAGES_ARGO_WORKFLOWCONTROLLER=${registry}/ds-pipelines-argo-workflowcontroller:${tag}|" "$params_file"
  sed -i "s|IMAGES_ARGO_EXEC=.*|IMAGES_ARGO_EXEC=${registry}/ds-pipelines-argo-argoexec:${tag}|" "$params_file"

  rm -rf "$argo_dir"
fi

echo "Updating params.env with locally built images..."
sed -i "s|IMAGES_APISERVER=.*|IMAGES_APISERVER=${registry}/ds-pipelines-apiserver:${tag}|" "$params_file"
sed -i "s|IMAGES_PERSISTENCEAGENT=.*|IMAGES_PERSISTENCEAGENT=${registry}/ds-pipelines-persistenceagent:${tag}|" "$params_file"
sed -i "s|IMAGES_SCHEDULEDWORKFLOW=.*|IMAGES_SCHEDULEDWORKFLOW=${registry}/ds-pipelines-scheduledworkflow:${tag}|" "$params_file"
sed -i "s|IMAGES_DRIVER=.*|IMAGES_DRIVER=${registry}/ds-pipelines-driver:${tag}|" "$params_file"
sed -i "s|IMAGES_LAUNCHER=.*|IMAGES_LAUNCHER=${registry}/ds-pipelines-launcher:${tag}|" "$params_file"

echo "Updated params.env:"
grep "^IMAGES_" "$params_file"

echo "DSP_TESTS_IMAGE=${registry}/ds-pipelines-tests:${tag}" >> "$GITHUB_ENV"

rm -rf "$dsp_dir"
echo "DSP images built and pushed successfully"
