#!/bin/bash

source $TEST_DIR/common

MY_DIR=$(readlink -f `dirname "${BASH_SOURCE[0]}"`)

source ${MY_DIR}/../util
RESOURCEDIR="${MY_DIR}/../resources/dsp-operator"
DSPAPROJECT=${DSPAPROJECT:-"data-science-pipelines-test"}

os::test::junit::declare_suite_start "$MY_SCRIPT"


function verify_data_science_pipelines_operator_install() {
    header "Testing Data Science Pipelines operator installation"

    os::cmd::expect_success_and_text "oc get deployment -n openshift-operators openshift-pipelines-operator" "openshift-pipelines-operator"
    runningpods=($(oc get pods -n openshift-operators -l name=openshift-pipelines-operator --field-selector="status.phase=Running" -o jsonpath="{$.items[*].metadata.name}"))
    os::cmd::expect_success_and_text "echo ${#runningpods[@]}" "1"

    os::cmd::expect_success_and_text "oc get deployment -n ${ODHPROJECT} data-science-pipelines-operator-controller-manager" "data-science-pipelines-operator-controller-manager"
    runningpods=($(oc get pods -n ${ODHPROJECT} --field-selector="status.phase=Running" | grep data-science-pipelines-operator | wc -l))
    os::cmd::expect_success_and_text "echo $runningpods" "3"
}

function create_and_verify_data_science_pipelines_resources() {
    header "Testing Data Science Pipelines installation with help of DSPO CR"

    os::cmd::expect_success "oc new-project ${DSPAPROJECT} || oc project ${DSPAPROJECT};"

    os::cmd::expect_success "oc apply -n ${DSPAPROJECT} -f ${RESOURCEDIR}/test-dspo-cr.yaml"
    os::cmd::try_until_text "oc get crd -n ${DSPAPROJECT} datasciencepipelinesapplications.datasciencepipelinesapplications.opendatahub.io" "datasciencepipelinesapplications.datasciencepipelinesapplications.opendatahub.io" $odhdefaulttimeout $odhdefaultinterval
    os::cmd::try_until_text "oc -n ${DSPAPROJECT} get pods -l app=ds-pipeline-sample -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}'" "True" $odhdefaulttimeout $odhdefaultinterval
    running_pods=$(oc get pods -n ${DSPAPROJECT} -l component=data-science-pipelines --field-selector='status.phase=Running' -o jsonpath='{$.items[*].metadata.name}' | wc -w)
    echo "Sleeping for 5 minutes for the DSPO CR settle up "
    sleep 5m
    os::cmd::expect_success "if [ "$running_pods" -gt "0" ]; then exit 0; else exit 1; fi"
}

function check_custom_resource_conditions() {
    header "Testing Data Science Pipelines Application CR conditions"

    # Check if all CR conditions are good
    os::cmd::try_until_text "oc get -n ${DSPAPROJECT} datasciencepipelinesapplication sample -o jsonpath='{.status.conditions[6].status}'" "True" $odhdefaulttimeout $odhdefaultinterval

    ## Given general condition is good, is expected that other component conditions are good
    # DataBaseReady
    os::cmd::try_until_text "oc get -n ${DSPAPROJECT} datasciencepipelinesapplication sample -o jsonpath='{.status.conditions[0].status}'" "True" $odhdefaulttimeout $odhdefaultinterval
    # ObjectStorageReady
    os::cmd::try_until_text "oc get -n ${DSPAPROJECT} datasciencepipelinesapplication sample -o jsonpath='{.status.conditions[1].status}'" "True" $odhdefaulttimeout $odhdefaultinterval
    # ApiServerReady
    os::cmd::try_until_text "oc get -n ${DSPAPROJECT} datasciencepipelinesapplication sample -o jsonpath='{.status.conditions[2].status}'" "True" $odhdefaulttimeout $odhdefaultinterval
    # PersistenceAgentReady
    os::cmd::try_until_text "oc get -n ${DSPAPROJECT} datasciencepipelinesapplication sample -o jsonpath='{.status.conditions[3].status}'" "True" $odhdefaulttimeout $odhdefaultinterval
    # ScheduledWorkflowReady
    os::cmd::try_until_text "oc get -n ${DSPAPROJECT} datasciencepipelinesapplication sample -o jsonpath='{.status.conditions[4].status}'" "True" $odhdefaulttimeout $odhdefaultinterval
    # UserInterfaceReady
    os::cmd::try_until_text "oc get -n ${DSPAPROJECT} datasciencepipelinesapplication sample -o jsonpath='{.status.conditions[5].status}'" "True" $odhdefaulttimeout $odhdefaultinterval
}

function check_data_science_pipeline_route() {
    header "Checking Routes of Data Science Pipeline availability"

    os::cmd::try_until_text "oc get pods -n ${DSPAPROJECT}  -l app=ds-pipeline-sample --field-selector='status.phase=Running' -o jsonpath='{$.items[*].metadata.name}' | wc -w" "1" $odhdefaulttimeout $odhdefaultinterval
}

function setup_monitoring() {
    header "Enabling User Workload Monitoring on the cluster"

    os::cmd::expect_success "oc apply -n openshift-monitoring -f ${RESOURCEDIR}/enable-uwm.yaml"
}

function test_metrics() {
    header "Checking metrics for total number of runs, should be 1 since we have spun up 1 run"

    cluster_version=$(oc get -o json clusterversion | jq '.items[0].status.desired.version')
    monitoring_token=$(oc create token thanos-querier -n openshift-monitoring)
    monitoring_route=$(oc get route thanos-querier -n openshift-monitoring --template={{.spec.host}})
    os::cmd::try_until_text "oc -n openshift-monitoring exec -c prometheus prometheus-k8s-0 -- curl -k -H \"Authorization: Bearer $monitoring_token\" 'https://thanos-querier.openshift-monitoring:9091/api/v1/query' -d 'query=controller_runtime_max_concurrent_reconciles{namespace=\"opendatahub\"}' | jq '.data.result[0].value[1]'" "1" $odhdefaulttimeout $odhdefaultinterval
}

function fetch_runs() {
    header "Fetch the dsp route and verify it works"

    ROUTE=$(oc get route -n ${DSPAPROJECT}  ds-pipeline-sample --template={{.spec.host}})
    SA_TOKEN=$(oc create token ds-pipeline-sample -n ${DSPAPROJECT})

    os::cmd::try_until_text "curl -s -k -H \"Authorization: Bearer ${SA_TOKEN}\" 'https://${ROUTE}/apis/v1beta1/runs'" "{}" $odhdefaulttimeout $odhdefaultinterval
}

function create_pipeline() {
    header "Creating a pipeline from data science pipelines stack"

    PIPELINE_ID=$(curl -s -k -H "Authorization: Bearer ${SA_TOKEN}" -F "uploadfile=@${RESOURCEDIR}/test-pipeline-run.yaml" "https://${ROUTE}/apis/v1beta1/pipelines/upload" | jq -r .id)
    os::cmd::try_until_not_text "curl -s -k -H \"Authorization: Bearer ${SA_TOKEN}\" 'https://${ROUTE}/apis/v1beta1/pipelines/${PIPELINE_ID}' | jq" "null" $odhdefaulttimeout $odhdefaultinterval
}

function verify_pipeline_availabilty() {
    header "verify the pipelines exists"

    os::cmd::try_until_text "curl -s -k -H 'Authorization: Bearer ${SA_TOKEN}' https://${ROUTE}/apis/v1beta1/pipelines | jq '.total_size'" "2" $odhdefaulttimeout $odhdefaultinterval
}

function create_run() {
    header "Creating the run from uploaded pipeline"

    RUN_ID=$(( curl -k -H "Authorization: Bearer ${SA_TOKEN}" \
        -H "Content-Type: application/json" \
        -X POST "https://${ROUTE}/apis/v1beta1/runs" \
        -d @- << EOF
        {
            "name":"test-pipeline-run",
            "pipeline_spec":{
                "pipeline_id":"${PIPELINE_ID}"
            }
        }
EOF
        ) | jq -r .run.id)

    os::cmd::try_until_not_text "curl -s -k -H 'Authorization: Bearer ${SA_TOKEN}' https://${ROUTE}/apis/v1beta1/runs/${RUN_ID} | jq '" "null" $odhdefaulttimeout $odhdefaultinterval
}

function verify_run_availabilty() {
    header "verify the run exists"

    os::cmd::try_until_text "curl -s -k -H 'Authorization: Bearer ${SA_TOKEN}' https://${ROUTE}/apis/v1beta1/runs | jq '.total_size'" "1" $odhdefaulttimeout $odhdefaultinterval
}

function check_run_status() {
    header "Checking run status"

    os::cmd::try_until_text "curl -s -k -H 'Authorization: Bearer ${SA_TOKEN}' https://${ROUTE}/apis/v1beta1/runs/${RUN_ID} | jq '.run.status'" "Completed" $odhdefaulttimeout $odhdefaultinterval
}

function delete_runs() {
    header "Deleting the runs"

    os::cmd::try_until_text "curl -s -k -H 'Authorization: Bearer ${SA_TOKEN}' -X DELETE https://${ROUTE}/apis/v1beta1/runs/${RUN_ID} | jq" "" $odhdefaulttimeout $odhdefaultinterval
    os::cmd::try_until_text "curl -s -k -H 'Authorization: Bearer ${SA_TOKEN}' https://${ROUTE}/apis/v1beta1/runs/${RUN_ID} | jq '.code'" "5" $odhdefaulttimeout $odhdefaultinterval
}

function delete_pipeline() {
    header "Deleting the pipeline"

    os::cmd::try_until_text "curl -s -k -H 'Authorization: Bearer ${SA_TOKEN}' -X DELETE https://${ROUTE}/apis/v1beta1/pipelines/${PIPELINE_ID} | jq" "" $odhdefaulttimeout $odhdefaultinterval
}


echo "Testing Data Science Pipelines Operator functionality"

verify_data_science_pipelines_operator_install
create_and_verify_data_science_pipelines_resources
check_custom_resource_conditions
check_data_science_pipeline_route
setup_monitoring
test_metrics
fetch_runs
create_pipeline
verify_pipeline_availabilty
create_run
verify_run_availabilty
check_run_status
delete_runs
delete_pipeline

os::test::junit::declare_suite_end
