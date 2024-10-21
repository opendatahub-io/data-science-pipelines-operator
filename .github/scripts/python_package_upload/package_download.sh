#!/bin/bash

set -ex

# Download packages
for python_version in "3.9" "3.10" "3.11" "3.12"; do
  for package in "kfp" "numpy"; do
    # If we don't set the --python it will use the one from the computer that may not be the one that the
    # pipeline is running
    pip download $package -d packages --only-binary=:none: --python $python_version
  done
done
