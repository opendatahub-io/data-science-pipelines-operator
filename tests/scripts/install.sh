#!/bin/bash

echo "Installing DataScienceCluster from test directory"

set -x

ODHREPO=${ODHREPO:-"data-science-pipelines-operator"}

## Install the opendatahub-operator
pushd ~/peak
retry=5
if ! [ -z "${SKIP_OPERATOR_INSTALL}" ]; then
    ## SKIP_OPERATOR_INSTALL is used in the opendatahub-operator repo
    ## because openshift-ci will install the operator for us
    echo "Relying on odh operator installed by openshift-ci"
    ./setup.sh -t ~/peak/operatorsetup 2>&1
else
  echo "Installing operator from community marketplace"
  while [[ $retry -gt 0 ]]; do
    ./setup.sh -o ~/peak/operatorsetup 2>&1
    if [ $? -eq 0 ]; then
      retry=-1
    else
      echo "Trying restart of marketplace community operator pod"
      oc delete pod -n openshift-marketplace $(oc get pod -n openshift-marketplace -l marketplace.operatorSource=community-operators -o jsonpath="{$.items[*].metadata.name}")
      sleep 3m
    fi
    retry=$(( retry - 1))
    sleep 1m
  done
fi
popd
## Grabbing and applying the patch in the PR we are testing
pushd ~/src/${ODHREPO}
if [ -z "$PULL_NUMBER" ]; then
  echo "No pull number, assuming nightly run"
else
  curl -O -L https://github.com/${REPO_OWNER}/${REPO_NAME}/pull/${PULL_NUMBER}.patch
  echo "Applying following patch:"
  cat ${PULL_NUMBER}.patch > ${ARTIFACT_DIR}/github-pr-${PULL_NUMBER}.patch
  git am ${PULL_NUMBER}.patch
fi
popd
## Point datasciencecluster_openshift.yaml to the manifests in the PR
pushd ~/datasciencecluster
if [ -z "$PULL_NUMBER" ]; then
  echo "No pull number, not modifying datasciencecluster_openshift.yaml"
else
  IMAGE_TAG=${IMAGE_TAG:-"quay.io/opendatahub/data-science-pipelines-operator:pr-$PULL_NUMBER"}
  sed -i "s#value: quay.io/opendatahub/data-science-pipelines-operator:latest#value: $IMAGE_TAG#" ./datasciencecluster_openshift.yaml
  if [ $REPO_NAME == $ODHREPO ]; then
    echo "Setting manifests in datasciencecluster_openshift to use pull number: $PULL_NUMBER"
    sed -i "s#uri: https://github.com/opendatahub-io/${ODHREPO}/tarball/main#uri: https://api.github.com/repos/opendatahub-io/${ODHREPO}/tarball/pull/${PULL_NUMBER}/head#" ./datasciencecluster_openshift.yaml
  fi
fi

if ! [ -z "${SKIP_DATASCIENCECLUSTER_INSTALL}" ]; then
  ## SKIP_DATASCIENCECLUSTER_INSTALL is useful in an instance where the
  ## operator install comes with an init container to handle
  ## the DataScienceCluster creation
  echo "Relying on existing DataScienceCluster because SKIP_DATASCIENCECLUSTER_INSTALL was set"
else
  echo "Creating the following DataScienceCluster"
  cat ./datasciencecluster_openshift.yaml > ${ARTIFACT_DIR}/datasciencecluster_openshift.yaml
  oc apply -f ./datasciencecluster_openshift.yaml
  datasciencecluster_result=$?
  if [ "$datasciencecluster_result" -ne 0 ]; then
    echo "The installation failed"
    exit $datasciencecluster_result
  fi
fi
set +x
popd
