#!/bin/bash
set -euo pipefail

DSPA_NAMESPACE="${DSPA_NAMESPACE:?DSPA_NAMESPACE must be set}"
DSPA_NAME="${DSPA_NAME:?DSPA_NAME must be set}"
DSPA_MANIFEST="${1:-tests/resources/dspa-lite.yaml}"

kubectl get namespace "$DSPA_NAMESPACE" 2>/dev/null || kubectl create namespace "$DSPA_NAMESPACE"
kubectl -n "$DSPA_NAMESPACE" apply -f "$DSPA_MANIFEST"

echo "Waiting for operator to create the APIServer deployment..."
kubectl -n "$DSPA_NAMESPACE" wait --for=create --timeout=300s \
  deployment/ds-pipeline-"$DSPA_NAME"

echo "Waiting for DSPA to become ready..."
kubectl -n "$DSPA_NAMESPACE" wait --timeout=300s \
  --for=condition=Available deployment/ds-pipeline-"$DSPA_NAME"

echo "DSPA deployment is ready."
