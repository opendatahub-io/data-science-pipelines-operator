#!/usr/bin/env bash

set -ex

echo "Cut branch ${MINOR_RELEASE_BRANCH} from main/master"

echo "Current branches in ${DSPO_REPOSITORY_FULL}"
git branch -r

git checkout -B ${MINOR_RELEASE_BRANCH}
git push origin ${MINOR_RELEASE_BRANCH}
echo "::notice:: Created DSPO ${MINOR_RELEASE_BRANCH} branch"

echo "Current branches in ${DSP_REPOSITORY_FULL}"
DSP_DIR=$(dirname ${WORKING_DIR})/data-science-pipelines
git clone \
  --depth=1 \
  --branch=master \
  https://${GH_USER_NAME}:${GH_TOKEN}@github.com/${DSP_REPOSITORY_FULL} \
  ${DSP_DIR}
cd ${DSP_DIR}
git checkout -B ${MINOR_RELEASE_BRANCH}
git push origin ${MINOR_RELEASE_BRANCH}
echo "::notice:: Created DSP ${MINOR_RELEASE_BRANCH} branch"
