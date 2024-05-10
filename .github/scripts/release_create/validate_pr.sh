#!/usr/bin/env bash

set -ex

echo "::notice:: Performing Release PR Validation for: ${PR_NUMBER}"

# Retrieve PR Author:
PR_AUTHOR=$(gh pr view ${PR_NUMBER} --json author -q .author.login)

echo "Current OWNERS:"
cat ./OWNERS

echo "::notice:: Checking if PR author ${PR_AUTHOR} is DSPO Owner..."

is_owner=$(cat ./OWNERS | var=${PR_AUTHOR} yq '[.approvers] | contains([env(var)])')
if [[ $is_owner == "false" ]]; then
  echo "::error:: PR author ${PR_AUTHOR} is not an approver in OWNERS file. Only approvers can create releases."
  exit 1
fi

echo "::notice:: PR author ${PR_AUTHOR} is an approver in DSPO OWNERS."

echo "::notice:: Validation successful."
