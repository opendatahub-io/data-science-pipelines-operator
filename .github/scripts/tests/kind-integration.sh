#!/bin/bash
set -e

if [ "$GIT_WORKSPACE" = "" ]; then
    echo "GIT_WORKSPACE variable not definied. Should be the root of the source code. Example GIT_WORKSPACE=/home/dev/git/data-science-pipelines-operator" && exit
fi

if [ "$REGISTRY_ADDRESS" = "" ]; then
    echo "REGISTRY_ADDRESS variable not definied." && exit
fi

# Env vars
IMAGE_REPO_DSPO="data-science-pipelines-operator"
DSPA_NAMESPACE="test-dspa"
DSPA_EXTERNAL_NAMESPACE="dspa-ext"
MINIO_NAMESPACE="test-minio"
MARIADB_NAMESPACE="test-mariadb"
PYPISERVER_NAMESPACE="test-pypiserver"
DSPA_NAME="test-dspa"
DSPA_EXTERNAL_NAME="dspa-ext"
DSPA_DEPLOY_WAIT_TIMEOUT="300"
INTEGRATION_TESTS_DIR="${GIT_WORKSPACE}/tests"
DSPA_PATH="${GIT_WORKSPACE}/tests/resources/dspa-lite.yaml"
DSPA_EXTERNAL_PATH="${GIT_WORKSPACE}/tests/resources/dspa-external-lite.yaml"
CONFIG_DIR="${GIT_WORKSPACE}/config"
RESOURCES_DIR_CRD="${GIT_WORKSPACE}/.github/resources"
DSPO_IMAGE="${REGISTRY_ADDRESS}/data-science-pipelines-operator"
OPENDATAHUB_NAMESPACE="opendatahub"
RESOURCES_DIR_PYPI="${GIT_WORKSPACE}/.github/resources/pypiserver/base"

# TODO: Consolidate testing CRDS (2 locations)
# Apply OCP CRDs
kubectl apply -f ${RESOURCES_DIR_CRD}/crds
kubectl apply -f "${CONFIG_DIR}/crd/external/route.openshift.io_routes.yaml"

# Build image
( cd $GIT_WORKSPACE && make podman-build -e IMG="${DSPO_IMAGE}" )

# Create opendatahub namespace
kubectl create namespace $OPENDATAHUB_NAMESPACE

# Deploy Argo Lite
( cd "${GIT_WORKSPACE}/.github/resources/argo-lite" && kustomize build . | kubectl -n $OPENDATAHUB_NAMESPACE apply -f - )

# Deploy DSPO
( cd $GIT_WORKSPACE && make podman-push -e IMG="${DSPO_IMAGE}" )
( cd $GIT_WORKSPACE && make deploy-kind -e IMG="${DSPO_IMAGE}" )

# Create Minio Namespace
kubectl create namespace $MINIO_NAMESPACE

# Deploy Minio
( cd "${GIT_WORKSPACE}/.github/resources/minio" && kustomize build . | kubectl -n $MINIO_NAMESPACE apply -f - )

# Create MariaDB Namespace
kubectl create namespace $MARIADB_NAMESPACE

# Deploy MariaDB
( cd "${GIT_WORKSPACE}/.github/resources/mariadb" && kustomize build . | kubectl -n $MARIADB_NAMESPACE apply -f - )

# Create Pypiserver Namespace
kubectl create namespace $PYPISERVER_NAMESPACE

# Deploy pypi-server
( cd "${GIT_WORKSPACE}/.github/resources/pypiserver/base" && kustomize build . | kubectl -n $PYPISERVER_NAMESPACE apply -f - )

# Wait for Dependencies (DSPO, Minio, Mariadb, Pypi server)
kubectl wait -n $OPENDATAHUB_NAMESPACE --timeout=60s --for=condition=Available=true deployment data-science-pipelines-operator-controller-manager
kubectl wait -n $MARIADB_NAMESPACE --timeout=60s --for=condition=Available=true deployment mariadb
kubectl wait -n $MINIO_NAMESPACE --timeout=60s --for=condition=Available=true deployment minio
kubectl wait -n $PYPISERVER_NAMESPACE --timeout=60s --for=condition=Available=true deployment pypi-server

# Upload Python Packages to pypi-server
( cd "${GIT_WORKSPACE}/.github/scripts/python_package_upload" && sh package_upload.sh )

# Create DSPA Namespace
kubectl create namespace $DSPA_NAMESPACE

# Create Namespace for DSPA with External connections
kubectl create namespace $DSPA_EXTERNAL_NAMESPACE

# Apply MariaDB and Minio Secrets and Configmaps in the External Namespace
( cd "${GIT_WORKSPACE}/.github/resources/external-pre-reqs" && kustomize build . |  oc -n $DSPA_EXTERNAL_NAMESPACE apply -f - )

# Apply PIP Server ConfigMap
( cd "${GIT_WORKSPACE}/.github/resources/pypiserver/base" && kubectl apply -f $RESOURCES_DIR_PYPI/nginx-tls-config.yaml -n $DSPA_NAMESPACE )

# Run tests
( cd $GIT_WORKSPACE && make integrationtest K8SAPISERVERHOST=$(oc whoami --show-server) DSPANAMESPACE=${DSPA_NAMESPACE} DSPAPATH=${DSPA_PATH} )

# Run tests for DSPA with External Connections
( cd $GIT_WORKSPACE && make integrationtest K8SAPISERVERHOST=$(oc whoami --show-server) DSPANAMESPACE=${DSPA_EXTERNAL_NAMESPACE} DSPAPATH=${DSPA_EXTERNAL_PATH} )

# Clean up
( cd $GIT_WORKSPACE && make undeploy-kind )
