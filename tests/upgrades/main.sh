kubectl create namespace ${DSPA_NS}
cd ${GITHUB_WORKSPACE}/config/samples/v1/dspa-simple
kustomize build . | kubectl -n ${DSPA_NS} apply -f -
