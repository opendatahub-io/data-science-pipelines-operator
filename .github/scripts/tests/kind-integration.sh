#!/bin/bash
set -e

if [ "$GIT_WORKSPACE" = "" ]; then
    echo "GIT_WORKSPACE variable not defined. Should be the root of the source code. Example GIT_WORKSPACE=/home/dev/git/data-science-pipelines-operator" && exit
fi

if [ "$REGISTRY_ADDRESS" = "" ]; then
    echo "REGISTRY_ADDRESS variable not defined." && exit
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
echo "---------------------------------"
echo "# Apply OCP CRDs"
echo "---------------------------------"
kubectl apply -f ${RESOURCES_DIR_CRD}/crds
kubectl apply -f "${CONFIG_DIR}/crd/external/route.openshift.io_routes.yaml"

echo "---------------------------------"
echo "Build image"
echo "---------------------------------"
( cd $GIT_WORKSPACE && make podman-build -e IMG="${DSPO_IMAGE}" )

echo "---------------------------------"
echo "Create opendatahub namespace"
echo "---------------------------------"
kubectl create namespace $OPENDATAHUB_NAMESPACE

echo "---------------------------------"
echo "Deploy Argo Lite"
echo "---------------------------------"
( cd "${GIT_WORKSPACE}/.github/resources/argo-lite" && kustomize build . | kubectl -n $OPENDATAHUB_NAMESPACE apply -f - )

echo "---------------------------------"
echo "Deploy DSPO"
echo "---------------------------------"
( cd $GIT_WORKSPACE && make podman-push -e IMG="${DSPO_IMAGE}" )
( cd $GIT_WORKSPACE && make deploy-kind -e IMG="${DSPO_IMAGE}" )

echo "---------------------------------"
echo "Create Minio Namespace"
echo "---------------------------------"
kubectl create namespace $MINIO_NAMESPACE

echo "---------------------------------"
echo "Deploy Minio"
echo "---------------------------------"
( cd "${GIT_WORKSPACE}/.github/resources/minio" && kustomize build . | kubectl -n $MINIO_NAMESPACE apply -f - )

echo "---------------------------------"
echo "Create MariaDB Namespace"
echo "---------------------------------"
kubectl create namespace $MARIADB_NAMESPACE

echo "---------------------------------"
echo "Deploy MariaDB"
echo "---------------------------------"
( cd "${GIT_WORKSPACE}/.github/resources/mariadb" && kustomize build . | kubectl -n $MARIADB_NAMESPACE apply -f - )

echo "---------------------------------"
echo "Create Pypiserver Namespace"
echo "---------------------------------"
kubectl create namespace $PYPISERVER_NAMESPACE

echo "---------------------------------"
echo "Deploy pypi-server"
echo "---------------------------------"
( cd "${GIT_WORKSPACE}/.github/resources/pypiserver/base" && kustomize build . | kubectl -n $PYPISERVER_NAMESPACE apply -f - )

echo "---------------------------------"
echo "Wait for Dependencies (DSPO, Minio, Mariadb, Pypi server)"
echo "---------------------------------"
kubectl wait -n $OPENDATAHUB_NAMESPACE --timeout=60s --for=condition=Available=true deployment data-science-pipelines-operator-controller-manager
kubectl wait -n $MARIADB_NAMESPACE --timeout=60s --for=condition=Available=true deployment mariadb
kubectl wait -n $MINIO_NAMESPACE --timeout=60s --for=condition=Available=true deployment minio
kubectl wait -n $PYPISERVER_NAMESPACE --timeout=60s --for=condition=Available=true deployment pypi-server

echo "---------------------------------"
echo "Upload Python Packages to pypi-server"
echo "---------------------------------"
( cd "${GIT_WORKSPACE}/.github/scripts/python_package_upload" && sh package_upload.sh )

echo "---------------------------------"
echo "Create DSPA Namespace"
echo "---------------------------------"
kubectl create namespace $DSPA_NAMESPACE

echo "---------------------------------"
echo "Create Namespace for DSPA with External connections"
echo "---------------------------------"
kubectl create namespace $DSPA_EXTERNAL_NAMESPACE

echo "---------------------------------"
echo "Apply MariaDB and Minio Secrets and Configmaps in the External Namespace"
echo "---------------------------------"
( cd "${GIT_WORKSPACE}/.github/resources/external-pre-reqs" && kustomize build . |  oc -n $DSPA_EXTERNAL_NAMESPACE apply -f - )

echo "---------------------------------"
echo "Apply PIP Server ConfigMap"
echo "---------------------------------"
( cd "${GIT_WORKSPACE}/.github/resources/pypiserver/base" && kubectl apply -f $RESOURCES_DIR_PYPI/nginx-tls-config.yaml -n $DSPA_NAMESPACE )

echo "---------------------------------"
echo "Run tests"
echo "---------------------------------"
( cd $GIT_WORKSPACE && make integrationtest K8SAPISERVERHOST=$(oc whoami --show-server) DSPANAMESPACE=${DSPA_NAMESPACE} DSPAPATH=${DSPA_PATH} )

echo "---------------------------------"
echo "Run tests for DSPA with External Connections"
echo "---------------------------------"
( cd $GIT_WORKSPACE && make integrationtest K8SAPISERVERHOST=$(oc whoami --show-server) DSPANAMESPACE=${DSPA_EXTERNAL_NAMESPACE} DSPAPATH=${DSPA_EXTERNAL_PATH} )

echo "---------------------------------"
echo "Clean up"
echo "---------------------------------"
( cd $GIT_WORKSPACE && make undeploy-kind )
