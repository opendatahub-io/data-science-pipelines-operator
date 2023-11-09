kubectl create namespace ${DSPA_NS}
cd ${GITHUB_WORKSPACE}/config/samples
kustomize build . | kubectl -n ${DSPA_NS} apply -f -
