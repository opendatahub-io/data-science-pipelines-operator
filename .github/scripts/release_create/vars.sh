#!/usr/bin/env bash

set -ex

cat ./config.yaml
target_version_tag=$(yq .target_version_tag ./config.yaml)
previous_version_tag=$(yq .previous_release_tag ./config.yaml)
release_branch=$(yq .release_branch ./config.yaml)
odh_org=$(yq .odh_org ./config.yaml)
pr_number=$(cat ./pr_number)

echo "pr_number=${pr_number}" >> $GITHUB_OUTPUT
echo "target_version_tag=${target_version_tag}" >> $GITHUB_OUTPUT
echo "previous_version_tag=${previous_version_tag}" >> $GITHUB_OUTPUT
echo "release_branch=${release_branch}" >> $GITHUB_OUTPUT
echo "odh_org=${odh_org}" >> $GITHUB_OUTPUT
