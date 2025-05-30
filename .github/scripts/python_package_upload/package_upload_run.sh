#!/bin/bash

set -ex

CONTAINER_CLI="${CONTAINER_CLI:-docker}"
PACKAGE_UPLOADER_IMAGE="${PACKAGE_UPLOADER_IMAGE:-}"

mkdir -p /tmp/packages

if [ -z "$PACKAGE_UPLOADER_IMAGE" ]; then
  echo "No prebuilt package uploader image specified, buildling..."
  $CONTAINER_CLI rm package_upload_run || true
  $CONTAINER_CLI build -t package_upload .
  $CONTAINER_CLI run --name package_upload_run -v /tmp/packages:/app/packages package_upload
else
  echo "Using prebuilt package uploader image ${PACKAGE_UPLOADER_IMAGE}"
  $CONTAINER_CLI run --name $PACKAGE_UPLOADER_IMAGE -v /tmp/packages:/app/packages package_upload
fi

# Print the pods in the namespace
kubectl -n test-pypiserver get pods

pod_name=$(kubectl -n test-pypiserver get pod | grep pypi | awk '{print $1}')

# Copy packages
for entry in /tmp/packages/*; do
    kubectl -n test-pypiserver cp "$entry" $pod_name:/opt/app-root/packages
done
