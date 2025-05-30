#!/bin/bash

set -ex

CONTAINER_CLI="${CONTAINER_CLI:-docker}"
RUN_PKG_UPLOADER_IN_CONTAINER="${RUN_PKG_UPLOADER_IN_CONTAINER:-true}"

mkdir -p /tmp/packages

if [ "$RUN_PKG_UPLOADER_IN_CONTAINER" = "true" ]; then
  echo "Running uploader in container..."
  $CONTAINER_CLI rm package_upload_run || true
  $CONTAINER_CLI build -t package_upload .
  $CONTAINER_CLI run --name package_upload_run -v /tmp/packages:/app/packages package_upload
else
  echo "Running uploader..."
  ./package_download.sh
  mv packages /tmp
fi

# Print the pods in the namespace
kubectl -n test-pypiserver get pods

pod_name=$(kubectl -n test-pypiserver get pod | grep pypi | awk '{print $1}')

# Copy packages
for entry in /tmp/packages/*; do
    kubectl -n test-pypiserver cp "$entry" $pod_name:/opt/app-root/packages
done
