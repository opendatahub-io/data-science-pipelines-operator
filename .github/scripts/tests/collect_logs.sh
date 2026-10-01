#!/usr/bin/env bash

set -e

DSPA_NS=""
DSPO_NS=""
DEPENDENCY_NAMESPACES=()

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --dspa-ns) DSPA_NS="$2"; shift ;;
        --dspo-ns) DSPO_NS="$2"; shift ;;
        --dependency-ns) DEPENDENCY_NAMESPACES+=("$2"); shift ;;
        *) echo "Unknown parameter passed: $1"; exit 1 ;;
    esac
    shift
done

if [[ -z "$DSPA_NS" || -z "$DSPO_NS" ]]; then
    echo "Both --dspa-ns and --dspo-ns parameters are required."
    exit 1
fi

function check_namespace {
    if ! kubectl get namespace "$1" &>/dev/null; then
        echo "Namespace '$1' is unavailable; skipping diagnostics for it."
        return 1
    fi
}

function display_pod_info {
    local NAMESPACE=$1
    local POD_NAMES

    if ! POD_NAMES=$(kubectl -n "${NAMESPACE}" get pods --no-headers -o custom-columns=":metadata.name"); then
        echo "Failed to list pods in namespace '${NAMESPACE}'."
        return
    fi

    if [[ -z "${POD_NAMES}" ]]; then
        echo "No pods found in namespace '${NAMESPACE}'."
        return
    fi

    for POD_NAME in ${POD_NAMES}; do
        echo "===== Pod: ${POD_NAME} in ${NAMESPACE} ====="

        echo "----- EVENTS -----"
        kubectl describe pod "${POD_NAME}" -n "${NAMESPACE}" | grep -A 100 Events || echo "No events found for pod ${POD_NAME}."

        echo "----- LOGS -----"
        kubectl logs "${POD_NAME}" -n "${NAMESPACE}" --all-containers=true || echo "No logs found for pod ${POD_NAME}."

        echo "----- PREVIOUS LOGS -----"
        kubectl logs "${POD_NAME}" -n "${NAMESPACE}" --all-containers=true --previous || echo "No previous logs found for pod ${POD_NAME}."

        echo "==========================="
        echo ""
    done
}

function get_pods {
    local NAMESPACE=$1
    echo "===== List of pods in the '${NAMESPACE}' namespace ====="
    kubectl get pods -n "${NAMESPACE}" || echo "Failed to list pods in namespace '${NAMESPACE}'."
    echo "==========================="
}

function collect_workflow_info {
    local NAMESPACE=$1

    echo "===== Collecting Argo Workflows in ${NAMESPACE} "
    # List all workflows
    kubectl -n "${NAMESPACE}" get workflows || echo "No workflows found in namespace '${NAMESPACE}'."

    # Display detailed workflow YAML
    kubectl -n "${NAMESPACE}" get workflow -o yaml || echo "Failed to retrieve workflows in '${NAMESPACE}'."

    echo "====================================================="
    echo ""
}

for NAMESPACE in "$DSPA_NS" "$DSPO_NS" "${DEPENDENCY_NAMESPACES[@]}"; do
    if ! check_namespace "$NAMESPACE"; then
        continue
    fi

    get_pods "$NAMESPACE"

    echo "===== Events in namespace '${NAMESPACE}' ====="
    kubectl get events -n "$NAMESPACE" --sort-by=.metadata.creationTimestamp || echo "Failed to retrieve events in namespace '${NAMESPACE}'."

    display_pod_info "$NAMESPACE"

    if [[ "$NAMESPACE" == "$DSPA_NS" ]]; then
        collect_workflow_info "$NAMESPACE"
    fi
done
