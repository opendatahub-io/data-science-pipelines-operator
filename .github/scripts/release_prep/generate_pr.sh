#!/usr/bin/env bash

# Note: The yaml in the body of the PR is used to feed inputs into the release workflow
# since there's no easy way to communicate information between the pr closing, and then triggering the
# release creation workflow.
# Therefore, take extra care when adding new code blocks in the PR body, or updating the existing one.
# Ensure any changes are compatible with the release_create workflow.

set -ex
set -o pipefail

echo "Retrieve the sha images from the resulting workflow (check quay.io for the digests)."
echo "Using [release-tools] generate a params.env and submit a new pr to vx.y+1.**x** branch."
echo "For images pulled from registry, ensure latest images are upto date"

BRANCH_NAME="release-${TARGET_RELEASE}"
git config --global user.email "${GH_USER_EMAIL}"
git config --global user.name "${GH_USER_NAME}"
git remote add ${GH_USER_NAME} https://${GH_USER_NAME}:${GH_TOKEN}@github.com/${GH_USER_NAME}/${DSPO_REPOSITORY}.git
git checkout -B ${BRANCH_NAME}

echo "Created branch: ${BRANCH_NAME}"

python ./scripts/release/release.py params --quay_org ${QUAY_ORG} --tag ${MINOR_RELEASE_TAG} --out_file ./config/base/params.env

git add .
git commit -m "Generate params for ${TARGET_RELEASE}"
git push ${GH_USER_NAME} $BRANCH_NAME -f

# Used to feed inputs to release creation workflow.
# target_version is used as the GH TAG
tmp_config="/tmp/body-config.txt"
body_txt="/tmp/body-text.txt"
cp $CONFIG_TEMPLATE $tmp_config

var=${GH_ORG} yq -i '.odh_org=env(var)' $tmp_config
var=${MINOR_RELEASE_BRANCH} yq -i '.release_branch=env(var)' $tmp_config
var=${MINOR_RELEASE_TAG} yq -i '.target_version_tag=env(var)' $tmp_config
var=${PREVIOUS_RELEASE_TAG} yq -i '.previous_release_tag=env(var)' $tmp_config

cat <<"EOF" > $body_txt
This is an automated PR to prep Data Science Pipelines Operator for release.
```yaml
<CONFIG_HERE>
```
EOF

sed -i "/<CONFIG_HERE>/{
    s/<CONFIG_HERE>//g
    r ${tmp_config}
}" $body_txt

pr_url=$(gh pr create \
  --repo https://github.com/${DSPO_REPOSITORY_FULL} \
  --body-file $body_txt \
  --title "Release ${MINOR_RELEASE_TAG}" \
  --head "${GH_USER_NAME}:$BRANCH_NAME" \
  --label "release-automation" \
  --base "${MINOR_RELEASE_BRANCH}")

echo "::notice:: PR successfully created: ${pr_url}"
