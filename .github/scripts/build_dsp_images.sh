#!/usr/bin/env bash
set -euo pipefail

# Build DSP images from source and push to the local registry.
# Requires: GIT_WORKSPACE, REGISTRY_ADDRESS, GITHUB_REPOSITORY_OWNER,
#           GITHUB_HEAD_REF or GITHUB_REF_NAME, GITHUB_SHA

echo "---------------------------------"
echo "Building DSP images from source"
echo "---------------------------------"

dsp_repo_owner="${GITHUB_REPOSITORY_OWNER:-opendatahub-io}"
dsp_repo="${dsp_repo_owner}/data-science-pipelines"
dsp_branch="${GITHUB_BASE_REF:-${GITHUB_REF_NAME:-master}}"
dsp_default_branch="master"
if [ "$dsp_repo_owner" = "red-hat-data-services" ]; then
  dsp_default_branch="main"
fi

tag="${GITHUB_SHA:0:7}"
if [ -z "$tag" ]; then
  tag="local"
fi

dsp_dir="${GIT_WORKSPACE}/_dsp_source"
rm -rf "$dsp_dir"

echo "Cloning ${dsp_repo} branch ${dsp_branch}..."
if ! git clone --depth 1 --branch "$dsp_branch" "https://github.com/${dsp_repo}.git" "$dsp_dir" 2>/dev/null; then
  echo "Branch ${dsp_branch} not found, falling back to ${dsp_default_branch}"
  git clone --depth 1 --branch "$dsp_default_branch" "https://github.com/${dsp_repo}.git" "$dsp_dir"
fi

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
  argo_repo_owner="${GITHUB_REPOSITORY_OWNER:-opendatahub-io}"
  argo_repo="${argo_repo_owner}/argo-workflows"
  argo_branch="${GITHUB_BASE_REF:-${GITHUB_REF_NAME:-main}}"
  argo_default_branch="main"

  argo_dir="${GIT_WORKSPACE}/_argo_source"
  rm -rf "$argo_dir"

  echo "Cloning ${argo_repo} branch ${argo_branch}..."
  if ! git clone --depth 1 --branch "$argo_branch" "https://github.com/${argo_repo}.git" "$argo_dir" 2>/dev/null; then
    echo "Branch ${argo_branch} not found, falling back to ${argo_default_branch}"
    git clone --depth 1 --branch "$argo_default_branch" "https://github.com/${argo_repo}.git" "$argo_dir"
  fi

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
