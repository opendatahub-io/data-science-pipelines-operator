#!/usr/bin/env bash

set -ex

check_branch_exists(){
  branch_exists=$(git ls-remote --heads https://github.com/${1}.git refs/heads/${2})
  echo "Checking for existence of branch ${2} in GH Repo ${1}"
  if [[ $branch_exists ]]; then
    echo "::error:: Branch ${2} already exist in GH Repo ${1}"
    exit 1
  fi
  echo "::notice:: Confirmed Branch ${2} does not exist in GH Repo ${1}"
}

check_branch_exists ${DSPO_REPOSITORY_FULL} ${MINOR_RELEASE_BRANCH}
check_branch_exists ${DSP_REPOSITORY_FULL} ${MINOR_RELEASE_BRANCH}

echo "Ensure compatibility.yaml is upto date, and generate a new compatibility.md. Use [release-tools] to accomplish this"

BRANCH_NAME="compatibility-doc-generate-${TARGET_RELEASE}"

git config --global user.email "${GH_USER_EMAIL}"
git config --global user.name "${GH_USER_NAME}"
git remote add ${GH_USER_NAME} https://${GH_USER_NAME}:${GH_TOKEN}@github.com/${GH_USER_NAME}/${DSPO_REPOSITORY}.git
git checkout -B ${BRANCH_NAME}

echo "Created branch: ${BRANCH_NAME}"
echo "Checking if compatibility.yaml contains ${TARGET_RELEASE} release...."

contains_rel=$(cat docs/release/compatibility.yaml | rel=${MINOR_RELEASE_WILDCARD} yq '[.[].dsp] | contains([env(rel)])')

if [[ "$contains_rel" == "false" ]]; then

cat <<EOF >> /tmp/error.txt
compatibility.yaml has NOT been updated with target release.

Please add ${MINOR_RELEASE_WILDCARD} dsp row in compatibility.yaml,

then regenerate the compatibility.md by following the instructions here:
https://github.com/opendatahub-io/data-science-pipelines-operator/tree/main/scripts/release#compatibility-doc-generation
EOF

echo ::error::$(cat /tmp/error.txt)
exit 1

fi

echo "::notice:: Confirmed existence of ${MINOR_RELEASE_BRANCH} in compatibility.yaml."

echo "Confirming that compatibility.md is upto date."
python ./scripts/release/release.py version_doc --input_file docs/release/compatibility.yaml --out_file docs/release/compatibility.md

git status

prereqs_successful=true

if [[ `git status --porcelain` ]]; then
  echo "::notice:: Compatibility.md is not up to date with Compatibility.yaml, creating pr to synchronize."

  git add .
  git commit -m "Update DSPO to $TARGET_RELEASE"
  git push ${GH_USER_NAME} $BRANCH_NAME -f
  gh pr create \
    --repo https://github.com/${DSPO_REPOSITORY_FULL} \
    --body "This is an automated PR to update Data Science Pipelines Operator version compatibility doc." \
    --title "Update DSP version compatibility doc." \
    --head "${GH_USER_NAME}:$BRANCH_NAME" \
    --base "main"

  echo "::notice:: PR to update compatibility doc has been created, please re-run this workflow once this PR is merged."
  prereqs_successful=false
else
  echo "::notice:: Compatibility.md doc is up to date with Compatibility.yaml, continuing with workflow..."
fi

# Save step outputs
echo "prereqs_successful=${prereqs_successful}"
echo "prereqs_successful=${prereqs_successful}" >> $GITHUB_OUTPUT
