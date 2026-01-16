#!/bin/bash
# This script is a partial copy of .github/scripts/tests.sh, pared down to contain only KinD deployment logic.
# This script is defined as the following:
# 1 - We declare the required environment variables
# 2 - Has the functions defined
# 3 - Setup the environment and run the tests by using the appropriated functions

set -e

# Env vars
echo "WORKSPACE=$WORKSPACE"
if [ "$WORKSPACE" = "" ]; then
    echo "WORKSPACE variable not defined. Should be the root of the source code. Example WORKSPACE=/home/dev/git/data-science-pipelines-operator" && exit 1
fi

echo "DSPO_IMAGE_REF=${DSPO_IMAGE_REF:-}"
if [ "$DSPO_IMAGE_REF" = "" ]; then
  echo "DSPO_IMAGE_REF variable not defined. Example DSPO_IMAGE_REF=quay.io/username/dspo:1" && exit 1
fi

CLEAN_INFRA=false
DSPA_NAMESPACE="test-dspa"
DSPA_EXTERNAL_NAMESPACE="dspa-ext"
DSPA_K8S_NAMESPACE="test-k8s-dspa"
MINIO_NAMESPACE="test-minio"
MARIADB_NAMESPACE="test-mariadb"
PYPISERVER_NAMESPACE="test-pypiserver"
ARGO_NAMESPACE="argo"
ARGO_VERSION="v3.6.7"
DEPLOY_EXTERNAL_ARGO=false
AWF_MANAGEMENT_STATE="Managed"
CONFIG_DIR="${WORKSPACE}/config"
RESOURCES_DIR_CRD="${WORKSPACE}/.github/resources"
OPENDATAHUB_NAMESPACE="opendatahub"
RESOURCES_DIR_PYPI="${WORKSPACE}/.github/resources/pypiserver/base"
DSPO_IMAGE_REF="${DSPO_IMAGE_REF:-}"
CONTAINER_CLI="${CONTAINER_CLI:-docker}"
RUN_PKG_UPLOADER_IN_CONTAINER="${RUN_PKG_UPLOADER_IN_CONTAINER:-true}"

apply_crd() {
  echo "---------------------------------"
  echo "# Apply OCP CRDs"
  echo "---------------------------------"
  kubectl apply -f ${RESOURCES_DIR_CRD}/crds
  kubectl apply -f "${CONFIG_DIR}/crd/external/route.openshift.io_routes.yaml"
}

build_image() {
  cd $WORKSPACE && $CONTAINER_CLI build . -t $DSPO_IMAGE_REF
}

create_opendatahub_namespace() {
  echo "---------------------------------"
  echo "Create opendatahub namespace"
  echo "---------------------------------"
  kubectl get namespace $OPENDATAHUB_NAMESPACE >/dev/null 2>&1 || \
  kubectl create namespace $OPENDATAHUB_NAMESPACE
}

create_argo_namespace() {
  echo "---------------------------------"
  echo "Create Argo namespace"
  echo "---------------------------------"
  kubectl get namespace $ARGO_NAMESPACE >/dev/null 2>&1 || \
  kubectl create namespace $ARGO_NAMESPACE
}

deploy_argo_lite() {
  echo "---------------------------------"
  echo "Deploy Argo Lite"
  echo "---------------------------------"
  ( cd "${WORKSPACE}/.github/resources/argo-lite" && kubectl -n $OPENDATAHUB_NAMESPACE apply -k . )
}

deploy_argo_external() {
  echo "---------------------------------"
  echo "Deploy External Argo"
  echo "---------------------------------"
  kubectl apply -n $ARGO_NAMESPACE -f https://github.com/argoproj/argo-workflows/releases/download/$ARGO_VERSION/install.yaml
}

deploy_dspo_kind() {
  $CONTAINER_CLI push $DSPO_IMAGE_REF
  echo $DSPO_IMAGE_REF
  ( cd $WORKSPACE && make deploy-kind -e IMG="${DSPO_IMAGE_REF}" )
}


deploy_minio() {
  echo "---------------------------------"
  echo "Create Minio Namespace"
  echo "---------------------------------"
  kubectl get namespace $MINIO_NAMESPACE >/dev/null 2>&1 || \
  kubectl create namespace $MINIO_NAMESPACE
  echo "---------------------------------"
  echo "Deploy Minio"
  echo "---------------------------------"
  ( cd "${WORKSPACE}/.github/resources/minio" && kubectl -n $MINIO_NAMESPACE apply -k . )
}

deploy_mariadb() {
  echo "---------------------------------"
  echo "Create MariaDB Namespace"
  echo "---------------------------------"
  kubectl get namespace $MARIADB_NAMESPACE >/dev/null 2>&1 || \
  kubectl create namespace $MARIADB_NAMESPACE
  echo "---------------------------------"
  echo "Deploy MariaDB"
  echo "---------------------------------"
  ( cd "${WORKSPACE}/.github/resources/mariadb" && kubectl -n $MARIADB_NAMESPACE apply -k . )
}

deploy_pypi_server() {
  echo "---------------------------------"
  echo "Create Pypiserver Namespace"
  echo "---------------------------------"
  kubectl get namespace $PYPISERVER_NAMESPACE >/dev/null 2>&1 || \
  kubectl create namespace $PYPISERVER_NAMESPACE
  echo "---------------------------------"
  echo "Deploy pypi-server"
  echo "---------------------------------"
  ( cd "${WORKSPACE}/.github/resources/pypiserver/base" && kubectl -n $PYPISERVER_NAMESPACE apply -k . )
}

wait_for_dspo_dependencies() {
  echo "---------------------------------"
  echo "Wait for DSPO Dependencies"
  echo "---------------------------------"
  kubectl wait -n $OPENDATAHUB_NAMESPACE --timeout=60s --for=condition=Available=true deployment data-science-pipelines-operator-controller-manager
}

wait_for_dspo_redeploy() {
  echo "---------------------------------"
  echo "Wait for DSPO Redeploy"
  echo "---------------------------------"
  sleep_amount=10
  counter=0
  max_counter=20
  sleep $sleep_amount  # Initial sleep to allow for deployment to roll out new pod
  while [ $counter -lt $max_counter ]; do
    echo "Waiting for DSPO to redeploy, attempt $counter out of $max_counter..."
    num_pods=`kubectl get pods -n $OPENDATAHUB_NAMESPACE -l app.kubernetes.io/name=data-science-pipelines-operator --no-headers | wc -l`
    if [ $num_pods -eq 1 ]; then
      break
    fi
    counter=$((counter+1))
    sleep $sleep_amount
  done
  if [ $counter -eq $max_counter ]; then
    echo "Error: DSPO did not redeploy $(($counter * $sleep_amount)) seconds."
    exit 1
  fi
  echo "DSPO redeployed after $(($counter * $sleep_amount)) seconds."
}

wait_for_dependencies() {
  echo "---------------------------------"
  echo "Wait for Dependencies (Minio, Mariadb, Pypi server)"
  echo "---------------------------------"
  kubectl wait -n $MARIADB_NAMESPACE --timeout=60s --for=condition=Available=true deployment mariadb
  kubectl wait -n $MINIO_NAMESPACE --timeout=60s --for=condition=Available=true deployment minio
  kubectl wait -n $PYPISERVER_NAMESPACE --timeout=60s --for=condition=Available=true deployment pypi-server
}

upload_python_packages_to_pypi_server() {
  echo "---------------------------------"
  echo "Upload Python Packages to pypi-server"
  echo "---------------------------------"
  ( cd "${WORKSPACE}/.github/scripts/python_package_upload" && sh package_upload_run.sh)
}

create_dspa_namespace() {
  echo "---------------------------------"
  echo "Create DSPA Namespace"
  echo "---------------------------------"
  kubectl get namespace $DSPA_NAMESPACE >/dev/null 2>&1 || \
  kubectl create namespace $DSPA_NAMESPACE
}

create_namespace_dspa_external_connections() {
  echo "---------------------------------"
  echo "Create Namespace for DSPA with External connections"
  echo "---------------------------------"
  kubectl get namespace $DSPA_EXTERNAL_NAMESPACE >/dev/null 2>&1 || \
  kubectl create namespace $DSPA_EXTERNAL_NAMESPACE
}

create_dspa_k8s_namespace() {
  echo "---------------------------------"
  echo "Create DSPA Namespace with Kubernetes Pipeline Storage"
  echo "---------------------------------"
  kubectl get namespace $DSPA_K8S_NAMESPACE >/dev/null 2>&1 || \
  kubectl create namespace $DSPA_K8S_NAMESPACE
}

apply_mariadb_minio_secrets_configmaps_external_namespace() {
  echo "---------------------------------"
  echo "Apply MariaDB and Minio Secrets and Configmaps in the External Namespace"
  echo "---------------------------------"
  ( cd "${WORKSPACE}/.github/resources/external-pre-reqs" && kubectl -n $DSPA_EXTERNAL_NAMESPACE apply -k . )
}

apply_pip_server_configmap() {
  echo "---------------------------------"
  echo "Apply PIP Server ConfigMap"
  echo "---------------------------------"
  for ns in $DSPA_NAMESPACE $DSPA_K8S_NAMESPACE; do
    echo "Applying ConfigMap in namespace: $ns"
    ( cd "${WORKSPACE}/.github/resources/pypiserver/base" && kubectl apply -f "$RESOURCES_DIR_PYPI/nginx-tls-config.yaml" -n "$ns" )
  done
}

update_dspo_env() {
  echo "---------------------------------"
  echo "Update DSPO Environment Variable"
  echo "---------------------------------"
  envkey=$1
  envval=$2
  echo "Updating DSPO Environment Variable: $envkey to $envval"
  kubectl set env -n $OPENDATAHUB_NAMESPACE deployment/data-science-pipelines-operator-controller-manager $envkey="$envval"
}

undeploy_kind_resources() {
  echo "---------------------------------"
  echo "Clean up resources created for testing on kind"
  echo "---------------------------------"
  ( cd $WORKSPACE && make undeploy-kind )
}

setup_kind_requirements() {
  apply_crd
  build_image
  create_opendatahub_namespace
  deploy_argo_lite
  deploy_dspo_kind
  deploy_minio
  deploy_mariadb
  deploy_pypi_server
  wait_for_dspo_dependencies
  wait_for_dependencies
  upload_python_packages_to_pypi_server
  create_dspa_namespace
  create_namespace_dspa_external_connections
  create_dspa_k8s_namespace
  apply_mariadb_minio_secrets_configmaps_external_namespace
  apply_pip_server_configmap
}

setup_external_argo() {
  update_dspo_env "DSPO_ARGOWORKFLOWSCONTROLLERS" "{\"managementState\": \"$AWF_MANAGEMENT_STATE\"}"
  create_argo_namespace
  deploy_argo_external
  wait_for_dspo_redeploy
}

# Run
while [ "$#" -gt 0 ]; do
  case "$1" in
    # The clean-infra option is helpful when rerunning tests on the same target environment, as it eliminates
    # the need to manually delete the necessary infrastructure. By default, this setting is set to false.
    # If true, before running the test, it delete the necessary infrastructure.
    --clean-infra)
      CLEAN_INFRA=true
      shift
      ;;
    --deploy-external-argo)
      DEPLOY_EXTERNAL_ARGO=true
      AWF_MANAGEMENT_STATE=Removed
      shift
      ;;
    --external-argo-version)
      shift
      if [[ -n "$1" ]]; then
        ARGO_VERSION="$1"
        shift
      else
        echo "Error: --external-argo-version requires a value (in form of vX.Y.Z)"
	      exit 1
      fi
      ;;
    *)
      echo "Unknown command line switch: $1"
      exit 1
      ;;
  esac
done

if [ "$CLEAN_INFRA" = true ] ; then
  undeploy_kind_resources
fi
setup_kind_requirements

# Update to remove on-board Argo Workflow Controllers for BYOArgo test cases
if [ "$DEPLOY_EXTERNAL_ARGO" = true ]; then
  setup_external_argo
fi
