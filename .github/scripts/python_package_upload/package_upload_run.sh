#!/bin/bash

set -ex

mkdir -p /tmp/packages
docker rm package_upload_run || true
docker build -t package_upload .
docker run --name package_upload_run -v /tmp/packages:/app/packages package_upload

# Print the pods in the namespace
kubectl -n test-pypiserver get pods

pod_name=$(kubectl -n test-pypiserver get pod | grep pypi | awk '{print $1}')

# Copy packages
for entry in /tmp/packages/*; do
    kubectl -n test-pypiserver cp "$entry" $pod_name:/opt/app-root/packages
done
