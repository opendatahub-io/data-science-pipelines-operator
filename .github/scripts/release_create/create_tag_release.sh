#!/usr/bin/env bash

set -ex

echo "Create a tag release for ${TARGET_VERSION_TAG} in ${REPOSITORY}"

RELEASE_REPO_DIR=$(dirname ${WORKING_DIR})/repo_dir
git clone \
  --depth=1 \
  --branch=${RELEASE_BRANCH} \
  https://${GH_USER_NAME}:${GH_TOKEN}@github.com/${REPOSITORY} \
  ${RELEASE_REPO_DIR}
cd ${RELEASE_REPO_DIR}

gh release create ${TARGET_VERSION_TAG} --target ${RELEASE_BRANCH} --generate-notes --notes-start-tag ${PREVIOUS_VERSION_TAG}

cat <<EOF >> /tmp/release-notes.md

This is a release comprising of multiple repos:
* DSP component for ${TARGET_VERSION_TAG} can be found [here](https://github.com/${GH_ORG}/data-science-pipelines/releases/tag/${TARGET_VERSION_TAG})
* DSPO component for ${TARGET_VERSION_TAG} can be found [here](https://github.com/${GH_ORG}/data-science-pipelines-operator/releases/tag/${TARGET_VERSION_TAG})

Version Table for components can be found [here](https://github.com/${GH_ORG}/data-science-pipelines-operator/blob/main/docs/release/compatibility.md)
EOF

echo "$(gh release view ${TARGET_VERSION_TAG} --json body --jq .body)" >> /tmp/release-notes.md

echo "Release notes to be created:"
cat /tmp/release-notes.md

gh release edit ${TARGET_VERSION_TAG} --notes-file /tmp/release-notes.md
rm /tmp/release-notes.md
