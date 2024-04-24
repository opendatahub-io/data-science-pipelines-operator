#!/usr/bin/env bash

set -ex

kfp_directory=kfp
boto_directory=boto3

mkdir -p "$kfp_directory"
mkdir -p "$boto_directory"

# Download kfp package
pip download kfp==2.7.0 -d "$kfp_directory"

# Download boto3 package
pip download boto3 -d "$boto_directory"


# Print the pods in the namespace
oc -n test-pypiserver get pods

pod_name=$(oc -n test-pypiserver get pod | grep pypi | awk '{print $1}')

# Copy kfp packages
for kfp_entry in "$kfp_directory"/*; do
    echo oc -n test-pypiserver cp "$kfp_entry" $pod_name:/opt/app-root/packages
    oc -n test-pypiserver cp "$kfp_entry" $pod_name:/opt/app-root/packages
done

# Copy boto3 packages
for boto_entry in "$boto_directory"/*; do
    echo oc -n test-pypiserver cp "$boto_entry" $pod_name:/opt/app-root/packages
    oc -n test-pypiserver cp "$boto_entry" $pod_name:/opt/app-root/packages
done
