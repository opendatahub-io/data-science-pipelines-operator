An operator that provisions a namespace installation of DSP within an OCP cluster.

# Quickstart

Deploy the operator
```bash
oc new-project ds-pipelines-controller
cd ${REPO}/config/default
kustomize build . | oc apply -f -
```

Deploy a DSP instance in a namespace
```bash
DSP_Namespace=test-ds-project
oc new-project ${DSP_Namespace}
cd ${REPO}/config/samples
kustomize build . | oc -n ${DSP_Namespace} apply -f -
```

Cleanup:

```bash
cd ${REPO}/config/default
kustomize build . | oc delete -f -
oc delete project ds-pipelines-controller
```