#!/bin/bash

set -ex

# Download packages
for package in "kfp==2.11.0" "numpy"; do
  pip download $package -d packages --only-binary=:none:
done
