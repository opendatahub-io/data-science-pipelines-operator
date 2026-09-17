#!/usr/bin/env bash
# This script is defined as the following:
# 1 - We declare the required environment variables
# 2 - Has the functions defined
# 3 - Port-forward to the API server and MLflow, then run ginkgo MLflow tests
#     from quay.io/opendatahub/ds-pipelines-tests (invoked by tests.sh run_tests_dspa_mlflow)

set -euo pipefail

# Env vars
DSPA_MLFLOW_NAMESPACE="${DSPA_MLFLOW_NAMESPACE:-test-dspa-mlflow}"
MLFLOW_NAMESPACE="${MLFLOW_NAMESPACE:-opendatahub}"
MLFLOW_CR_NAME="${MLFLOW_CR_NAME:-mlflow}"
DSPA_NAME="${DSPA_NAME:-test-dspa-mlflow}"
DSP_TESTS_IMAGE_TAG="${DSP_TESTS_IMAGE_TAG:-master}"
DSP_TESTS_IMAGE="${DSP_TESTS_IMAGE:-quay.io/opendatahub/ds-pipelines-tests:${DSP_TESTS_IMAGE_TAG}}"
CONTAINER_CLI="${CONTAINER_CLI:-docker}"
TEST_LABEL="${TEST_LABEL:-MLflow}"
NUM_PARALLEL_NODES="${NUM_PARALLEL_NODES:-1}"
API_LOCAL_PORT="${API_LOCAL_PORT:-8888}"
MLFLOW_LOCAL_PORT="${MLFLOW_LOCAL_PORT:-8080}"
KUBECONFIG_PATH="${KUBECONFIG:-${HOME}/.kube/config}"
KUBECTL_BIN="${KUBECTL_BIN:-$(command -v kubectl || true)}"
CONTAINER_KUBECONFIG="/dspa/backend/test/.kube/config"
CUSTOM_PIP_INDEX_URL="${CUSTOM_PIP_INDEX_URL:-https://pypi.org/simple}"
CUSTOM_PIP_TRUSTED_HOST="${CUSTOM_PIP_TRUSTED_HOST:-pypi.org}"
BASE_IMAGE="${BASE_IMAGE:-quay.io/opendatahub/ds-pipelines-ci-executor-image:v1.1}"

DEPLOYMENT_NAME="ds-pipeline-${DSPA_NAME}"
PIPELINE_RUNNER_SA="pipeline-runner-${DSPA_NAME}"
API_PF_PID=""
MLFLOW_PF_PID=""
GINKGO_BIN_DIR=""
GINKGO_BIN=""
MLFLOW_CA_FILE=""
MLFLOW_HOST=""
MLFLOW_HOSTS_ENTRY_ADDED=""
MLFLOW_TLS_SECRET_NAME="${MLFLOW_TLS_SECRET_NAME:-mlflow-tls}"
MLFLOW_TLS_SECRET_CERT_KEY="${MLFLOW_TLS_SECRET_CERT_KEY:-tls.crt}"
DRIVER_POD_NAME_PATTERN="-system-container-driver-"
DRIVER_LOG_WATCHER_PID=""
DRIVER_LOGGED_PODS_FILE=""

if [ ! -f "${KUBECONFIG_PATH}" ]; then
  echo "Kubeconfig not found at ${KUBECONFIG_PATH}" >&2
  exit 1
fi

if [ -z "${KUBECTL_BIN}" ] || [ ! -x "${KUBECTL_BIN}" ]; then
  echo "kubectl not found on host (set KUBECTL_BIN); ds-pipelines-tests needs it for workflow logs" >&2
  exit 1
fi

cleanup() {
  local exit_code=$?
  if [ -n "${API_PF_PID}" ] && kill -0 "${API_PF_PID}" 2>/dev/null; then
    kill "${API_PF_PID}" 2>/dev/null || true
  fi
  if [ -n "${MLFLOW_PF_PID}" ] && kill -0 "${MLFLOW_PF_PID}" 2>/dev/null; then
    kill "${MLFLOW_PF_PID}" 2>/dev/null || true
  fi
  pkill -f "kubectl port-forward.*${DEPLOYMENT_NAME}.*${API_LOCAL_PORT}" 2>/dev/null || true
  pkill -f "kubectl port-forward.*${MLFLOW_NAMESPACE}.*${MLFLOW_LOCAL_PORT}" 2>/dev/null || true
  if [ -n "${GINKGO_BIN_DIR}" ] && [ -d "${GINKGO_BIN_DIR}" ]; then
    rm -rf "${GINKGO_BIN_DIR}"
  fi
  if [ -n "${MLFLOW_CA_FILE}" ] && [ -f "${MLFLOW_CA_FILE}" ]; then
    rm -f "${MLFLOW_CA_FILE}"
  fi
  if [ -n "${MLFLOW_HOSTS_ENTRY_ADDED}" ] && [ -n "${MLFLOW_HOST}" ]; then
    sudo sed -i "\|127.0.0.1 ${MLFLOW_HOST}|d" /etc/hosts 2>/dev/null || true
  fi
  stop_driver_log_watcher
  exit "${exit_code}"
}
trap cleanup EXIT INT TERM

stop_conflicting_port_forwards() {
  pkill -f "kubectl port-forward.*${DEPLOYMENT_NAME}.*${API_LOCAL_PORT}" 2>/dev/null || true
  pkill -f "kubectl port-forward.*${MLFLOW_NAMESPACE}.*${MLFLOW_LOCAL_PORT}" 2>/dev/null || true
  sleep 2
}

resolve_mlflow_service() {
  local address_url
  if ! kubectl get mlflow "${MLFLOW_CR_NAME}" -n "${MLFLOW_NAMESPACE}" >/dev/null 2>&1; then
    echo "MLflow CR ${MLFLOW_CR_NAME} not found in ${MLFLOW_NAMESPACE}" >&2
    exit 1
  fi
  address_url="$(kubectl get mlflow "${MLFLOW_CR_NAME}" -n "${MLFLOW_NAMESPACE}" \
    -o jsonpath='{.status.address.url}' 2>/dev/null || true)"
  if [ -z "${address_url}" ]; then
    echo "MLflow CR ${MLFLOW_CR_NAME} has no status.address.url in ${MLFLOW_NAMESPACE}" >&2
    kubectl get mlflow "${MLFLOW_CR_NAME}" -n "${MLFLOW_NAMESPACE}" -o yaml || true
    exit 1
  fi
  # MLflow operator names the Service after the CR (must be "mlflow").
  MLFLOW_SVC="${MLFLOW_CR_NAME}"
  if ! kubectl get svc -n "${MLFLOW_NAMESPACE}" "${MLFLOW_SVC}" >/dev/null 2>&1; then
    echo "MLflow service ${MLFLOW_SVC} not found in ${MLFLOW_NAMESPACE}" >&2
    kubectl get svc -n "${MLFLOW_NAMESPACE}" || true
    exit 1
  fi
  MLFLOW_REMOTE_PORT="$(kubectl get svc -n "${MLFLOW_NAMESPACE}" "${MLFLOW_SVC}" -o jsonpath='{.spec.ports[0].port}')"
  # Port-forward uses localhost, but the TLS cert is issued for the in-cluster DNS name.
  local authority="${address_url#*://}"
  authority="${authority%%/*}"
  MLFLOW_HOST="${authority%%:*}"
  if [ -z "${MLFLOW_HOST}" ]; then
    echo "Failed to parse MLflow host from address URL: ${address_url}" >&2
    exit 1
  fi
  echo "MLflow service: ${MLFLOW_SVC}:${MLFLOW_REMOTE_PORT} (namespace ${MLFLOW_NAMESPACE}, address: ${address_url}, host: ${MLFLOW_HOST})"
}

ensure_mlflow_hosts_entry() {
  if grep -qE "[[:space:]]${MLFLOW_HOST}([[:space:]]|$)" /etc/hosts 2>/dev/null; then
    return 0
  fi
  echo "Mapping ${MLFLOW_HOST} -> 127.0.0.1 in /etc/hosts for port-forward TLS"
  echo "127.0.0.1 ${MLFLOW_HOST}" | sudo tee -a /etc/hosts >/dev/null
  MLFLOW_HOSTS_ENTRY_ADDED=1
}

resolve_mlflow_ca() {
  MLFLOW_CA_FILE="$(mktemp)"
  if kubectl get configmap mlflow-ca-bundle -n "${MLFLOW_NAMESPACE}" >/dev/null 2>&1; then
    if ! kubectl get configmap mlflow-ca-bundle -n "${MLFLOW_NAMESPACE}" \
      -o jsonpath='{.data.ca-bundle\.crt}' >"${MLFLOW_CA_FILE}" 2>/dev/null; then
      echo "Failed to read MLflow CA from configmap mlflow-ca-bundle in ${MLFLOW_NAMESPACE}" >&2
      exit 1
    fi
    echo "Using MLflow CA from ${MLFLOW_NAMESPACE}/mlflow-ca-bundle"
  elif kubectl get secret "${MLFLOW_TLS_SECRET_NAME}" -n "${MLFLOW_NAMESPACE}" >/dev/null 2>&1; then
    local jsonpath_key="${MLFLOW_TLS_SECRET_CERT_KEY//./\\.}"
    if ! kubectl get secret "${MLFLOW_TLS_SECRET_NAME}" -n "${MLFLOW_NAMESPACE}" \
      -o "jsonpath={.data.${jsonpath_key}}" | base64 -d >"${MLFLOW_CA_FILE}" 2>/dev/null; then
      echo "Failed to read ${MLFLOW_TLS_SECRET_CERT_KEY} from secret ${MLFLOW_TLS_SECRET_NAME} in ${MLFLOW_NAMESPACE}" >&2
      exit 1
    fi
    echo "Using MLflow CA from ${MLFLOW_NAMESPACE}/${MLFLOW_TLS_SECRET_NAME} (${MLFLOW_TLS_SECRET_CERT_KEY})"
  else
    echo "Neither mlflow-ca-bundle ConfigMap nor ${MLFLOW_TLS_SECRET_NAME} Secret found in ${MLFLOW_NAMESPACE}" >&2
    exit 1
  fi
  if [ ! -s "${MLFLOW_CA_FILE}" ]; then
    echo "MLflow CA material is empty" >&2
    exit 1
  fi
}

start_port_forwards() {
  echo "---------------------------------"
  echo "Port-forward API server and MLflow"
  echo "---------------------------------"
  stop_conflicting_port_forwards

  kubectl port-forward -n "${DSPA_MLFLOW_NAMESPACE}" "svc/${DEPLOYMENT_NAME}" "${API_LOCAL_PORT}:8888" >/dev/null 2>&1 &
  API_PF_PID=$!

  kubectl port-forward -n "${MLFLOW_NAMESPACE}" "svc/${MLFLOW_SVC}" "${MLFLOW_LOCAL_PORT}:${MLFLOW_REMOTE_PORT}" >/dev/null 2>&1 &
  MLFLOW_PF_PID=$!

  sleep 3
  if ! kill -0 "${API_PF_PID}" 2>/dev/null; then
    echo "API server port-forward (PID ${API_PF_PID}) died immediately" >&2
    exit 1
  fi
  if ! kill -0 "${MLFLOW_PF_PID}" 2>/dev/null; then
    echo "MLflow port-forward (PID ${MLFLOW_PF_PID}) died immediately" >&2
    exit 1
  fi
  ensure_mlflow_hosts_entry
}

wait_for_api_health() {
  echo "Waiting for API server health on http://127.0.0.1:${API_LOCAL_PORT}..."
  for _ in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:${API_LOCAL_PORT}/apis/v2beta1/healthz" >/dev/null 2>&1; then
      echo "API server is healthy"
      return 0
    fi
    sleep 2
  done
  echo "API server not healthy at http://127.0.0.1:${API_LOCAL_PORT}" >&2
  exit 1
}

wait_for_mlflow_health() {
  local health_url="https://${MLFLOW_HOST}:${MLFLOW_LOCAL_PORT}/mlflow/health"
  local curl_resolve=(--resolve "${MLFLOW_HOST}:${MLFLOW_LOCAL_PORT}:127.0.0.1")
  local curl_headers=(-H "X-MLflow-Workspace: ${DSPA_MLFLOW_NAMESPACE}")
  if [ -n "${MLFLOW_BEARER_TOKEN:-}" ]; then
    curl_headers+=(-H "Authorization: Bearer ${MLFLOW_BEARER_TOKEN}")
  fi

  echo "Waiting for MLflow health at ${health_url} (via port-forward on 127.0.0.1:${MLFLOW_LOCAL_PORT})..."
  for _ in $(seq 1 30); do
    status="$(curl --cacert "${MLFLOW_CA_FILE}" "${curl_resolve[@]}" -o /dev/null -w '%{http_code}' \
      --connect-timeout 5 --max-time 10 \
      "${curl_headers[@]}" "${health_url}" 2>/dev/null || echo "000")"
    if [ "${status}" = "200" ]; then
      echo "MLflow backend is healthy (status=${status})"
      return 0
    fi
    sleep 2
  done
  echo "MLflow backend not healthy at ${health_url} (last HTTP status: ${status})" >&2
  exit 1
}

configure_auth() {
  echo "---------------------------------"
  echo "Generate API and MLflow auth tokens"
  echo "---------------------------------"
  API_TOKEN="$(kubectl create token "${DEPLOYMENT_NAME}" --namespace "${DSPA_MLFLOW_NAMESPACE}" --duration=60m)"
  API_URL="http://127.0.0.1:${API_LOCAL_PORT}"
  MLFLOW_BEARER_TOKEN="$(kubectl create token "${DEPLOYMENT_NAME}" --namespace "${DSPA_MLFLOW_NAMESPACE}" --duration=60m)"
  export MLFLOW_BEARER_TOKEN
  export MLFLOW_TRACKING_URI="https://${MLFLOW_HOST}:${MLFLOW_LOCAL_PORT}/mlflow"
  export MLFLOW_WORKSPACE="${DSPA_MLFLOW_NAMESPACE}"
}

dump_failed_driver_pod_logs() {
  local pod="$1"

  if [ -z "${pod}" ] || [ -z "${DRIVER_LOGGED_PODS_FILE}" ]; then
    return 0
  fi
  if grep -qxF "${pod}" "${DRIVER_LOGGED_PODS_FILE}" 2>/dev/null; then
    return 0
  fi
  echo "${pod}" >>"${DRIVER_LOGGED_PODS_FILE}"

  echo "===== FAILED workflow driver pod: ${pod} (namespace ${DSPA_MLFLOW_NAMESPACE}) ====="
  echo "----- EVENTS -----"
  kubectl describe pod -n "${DSPA_MLFLOW_NAMESPACE}" "${pod}" 2>&1 | grep -A 100 Events || echo "No events found for pod ${pod}."
  echo "----- LOGS (all containers) -----"
  kubectl logs -n "${DSPA_MLFLOW_NAMESPACE}" "${pod}" --all-containers=true 2>&1 || echo "No logs found for pod ${pod}."
  echo "----- LOGS (main container) -----"
  kubectl logs -n "${DSPA_MLFLOW_NAMESPACE}" "${pod}" -c main 2>&1 || echo "No main container logs found for pod ${pod}."
  echo "----- PREVIOUS LOGS (main container) -----"
  kubectl logs -n "${DSPA_MLFLOW_NAMESPACE}" "${pod}" -c main --previous 2>&1 || echo "No previous main container logs found for pod ${pod}."
  echo "================================================================="
}

collect_failed_driver_logs_once() {
  local pods pod

  pods="$(kubectl get pods -n "${DSPA_MLFLOW_NAMESPACE}" \
    --field-selector=status.phase=Failed \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null \
    | grep -F -- "${DRIVER_POD_NAME_PATTERN}" || true)"
  if [ -z "${pods}" ]; then
    return 0
  fi

  for pod in ${pods}; do
    dump_failed_driver_pod_logs "${pod}"
  done
}

start_driver_log_watcher() {
  DRIVER_LOGGED_PODS_FILE="$(mktemp)"
  (
    while true; do
      collect_failed_driver_logs_once
      sleep 0.5
    done
  ) &
  DRIVER_LOG_WATCHER_PID=$!
  echo "Watching for failed workflow driver pods in ${DSPA_MLFLOW_NAMESPACE} (PID ${DRIVER_LOG_WATCHER_PID})"
}

stop_driver_log_watcher() {
  if [ -n "${DRIVER_LOG_WATCHER_PID}" ] && kill -0 "${DRIVER_LOG_WATCHER_PID}" 2>/dev/null; then
    kill "${DRIVER_LOG_WATCHER_PID}" 2>/dev/null || true
    wait "${DRIVER_LOG_WATCHER_PID}" 2>/dev/null || true
  fi
  DRIVER_LOG_WATCHER_PID=""
  collect_failed_driver_logs_once
  if [ -n "${DRIVER_LOGGED_PODS_FILE}" ] && [ -f "${DRIVER_LOGGED_PODS_FILE}" ]; then
    rm -f "${DRIVER_LOGGED_PODS_FILE}"
  fi
  DRIVER_LOGGED_PODS_FILE=""
}

build_ginkgo_binary() {
  echo "---------------------------------"
  echo "Pre-build ginkgo binary (${DSP_TESTS_IMAGE})"
  echo "---------------------------------"
  GINKGO_BIN_DIR="$(mktemp -d)"
  GINKGO_BIN="${GINKGO_BIN_DIR}/ginkgo"

  local ginkgo_mount="${GINKGO_BIN_DIR}:/out"
  if [ "${CONTAINER_CLI}" = "podman" ]; then
    ginkgo_mount="${ginkgo_mount}:Z"
  fi

  # shellcheck disable=SC2086
  ${CONTAINER_CLI} run --rm \
    --entrypoint bash \
    -v "${ginkgo_mount}" \
    --workdir=/dspa/backend/test/end2end \
    "${DSP_TESTS_IMAGE}" \
    -c 'go build -o /out/ginkgo github.com/onsi/ginkgo/v2/ginkgo'

  if [ ! -x "${GINKGO_BIN}" ]; then
    echo "Failed to build ginkgo binary at ${GINKGO_BIN}" >&2
    exit 1
  fi
}

run_dsp_tests() {
  echo "---------------------------------"
  echo "Run DSP MLflow tests (${DSP_TESTS_IMAGE})"
  echo "---------------------------------"

  local kube_mount="${KUBECONFIG_PATH}:${CONTAINER_KUBECONFIG}"
  local ginkgo_mount="${GINKGO_BIN_DIR}:/out:ro"
  local kubectl_mount="${KUBECTL_BIN}:/usr/local/bin/kubectl:ro"
  local mlflow_ca_mount="${MLFLOW_CA_FILE}:/mlflow-ca/ca-bundle.crt:ro"
  if [ "${CONTAINER_CLI}" = "podman" ]; then
    kube_mount="${kube_mount}:Z"
    ginkgo_mount="${GINKGO_BIN_DIR}:/out:ro,Z"
    kubectl_mount="${kubectl_mount},Z"
    mlflow_ca_mount="${mlflow_ca_mount},Z"
  fi

  local network_args=""
  if [ "$(uname -s)" = "Linux" ]; then
    network_args="--network host"
  fi

  start_driver_log_watcher

  set +e
  # shellcheck disable=SC2086
  ${CONTAINER_CLI} run --rm \
    --entrypoint bash \
    ${network_args} \
    -v "${kube_mount}" \
    -v "${ginkgo_mount}" \
    -v "${kubectl_mount}" \
    -v "${mlflow_ca_mount}" \
    -e KUBECONFIG="${CONTAINER_KUBECONFIG}" \
    -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-minio}" \
    -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-minio123}" \
    -e MLFLOW_TRACKING_URI \
    -e MLFLOW_WORKSPACE \
    -e MLFLOW_BEARER_TOKEN \
    -e MLFLOW_CA_BUNDLE_PATH="/mlflow-ca/ca-bundle.crt" \
    -e REQUESTS_CA_BUNDLE="/mlflow-ca/ca-bundle.crt" \
    -e SSL_CERT_FILE="/mlflow-ca/ca-bundle.crt" \
    -e DSP_TEST_NAMESPACE="${DSPA_MLFLOW_NAMESPACE}" \
    -e DSP_TEST_API_URL="${API_URL}" \
    -e DSP_TEST_API_TOKEN="${API_TOKEN}" \
    -e DSP_TEST_PIPELINE_RUNNER_SA="${PIPELINE_RUNNER_SA}" \
    -e DSP_TEST_LABEL="${TEST_LABEL}" \
    -e DSP_TEST_NUM_PARALLEL_NODES="${NUM_PARALLEL_NODES}" \
    -e DSP_TEST_CUSTOM_PIP_INDEX_URL="${CUSTOM_PIP_INDEX_URL}" \
    -e DSP_TEST_CUSTOM_PIP_TRUSTED_HOST="${CUSTOM_PIP_TRUSTED_HOST}" \
    -e DSP_TEST_BASE_IMAGE="${BASE_IMAGE}" \
    --workdir=/dspa/backend/test/end2end \
    "${DSP_TESTS_IMAGE}" \
    -c '/out/ginkgo -r -v -p \
      --nodes="${DSP_TEST_NUM_PARALLEL_NODES}" \
      --label-filter="${DSP_TEST_LABEL}" \
      -- -namespace="${DSP_TEST_NAMESPACE}" \
      -apiUrl="${DSP_TEST_API_URL}" \
      -authToken="${DSP_TEST_API_TOKEN}" \
      -disableTlsCheck=true \
      -mlflowEnabled=true \
      -customPipIndexURL="${DSP_TEST_CUSTOM_PIP_INDEX_URL}" \
      -customPipTrustedHost="${DSP_TEST_CUSTOM_PIP_TRUSTED_HOST}" \
      -serviceAccountName="${DSP_TEST_PIPELINE_RUNNER_SA}" \
      -baseImage="${DSP_TEST_BASE_IMAGE}" \
      -disconnectedCluster=false'
  local test_exit=$?
  set -e

  stop_driver_log_watcher
  return "${test_exit}"
}

resolve_mlflow_service
resolve_mlflow_ca
configure_auth
build_ginkgo_binary
start_port_forwards
wait_for_api_health
wait_for_mlflow_health
run_dsp_tests
