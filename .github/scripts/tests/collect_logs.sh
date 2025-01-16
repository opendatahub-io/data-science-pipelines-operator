#!/usr/bin/env bash

set -e

DSPA_NS=""
DSPO_NS=""

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --dspa-ns) DSPA_NS="$2"; shift ;;
        --dspo-ns) DSPO_NS="$2"; shift ;;
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
        echo "Namespace '$1' does not exist."
        exit 1
    fi
}

function display_pod_info {
    local NAMESPACE=$1
    local POD_NAMES

    POD_NAMES=$(kubectl -n "${DSPA_NS}" get pods -o custom-columns=":metadata.name")

    if [[ -z "${POD_NAMES}" ]]; then
        echo "No pods found in namespace '${NAMESPACE}'."
        return
    fi

    for POD_NAME in ${POD_NAMES}; do
        echo "===== Pod: ${POD_NAME} in ${NAMESPACE} ====="

        echo "----- EVENTS -----"
        kubectl describe pod "${POD_NAME}" -n "${NAMESPACE}" | grep -A 100 Events || echo "No events found for pod ${POD_NAME}."

        echo "----- LOGS -----"
        kubectl logs "${POD_NAME}" -n "${NAMESPACE}" || echo "No logs found for pod ${POD_NAME}."

        echo "==========================="
        echo ""
    done
}

check_namespace "$DSPA_NS"
check_namespace "$DSPO_NS"

display_pod_info "$DSPA_NS"
display_pod_info "$DSPO_NS"
