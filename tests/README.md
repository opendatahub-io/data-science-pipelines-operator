# DSP Integration tests

In this folder you will find the DSP Integration tests. These tests are intended to be run against a live Kubernetes or OCP 
cluster. They are also utilized in our KinD GitHub workflow (e.g. [kind-workflow])

The tests are scoped to an individual namespace and require the testing namespace to be created beforehand.

### Pre-requisites

* Logged into an OCP/K8s cluster
* Valid kubeconfig file (default is `$HOME/.kube/config`)
* DSPO is already installed (either via ODH or manual)
* An empty namespace to run the tests in


### Run tests locally


#### Full test suite

The full test suite will install a DSPA, wait for it to reach ready status, run the test suite, then clean up the DSPA 
afterwards.

```bash

# Adjust the following as needed
KUBECONFIG_PATH=$HOME/.kube/config # this is usually the default
TARGET_CLUSTER=...(e.g. https://api.hukhan.dev.datahub.redhat.com:6443, you can retrieve this via `oc whoami --show-server`)
TARGET_NAMESPACE=dspa # Do not use the same namespace as where DSPO is deployed (otherwise you will encounter some failed tests that verify DSPA deployment).

git clone git@github.com:opendatahub-io/data-science-pipelines-operator.git ${DSPO_REPO}

# Make sure DSPO is already deployed, if not then run: 
oc new-project opendatahub
make deploy

make integrationtest \
 K8SAPISERVERHOST=${TARGET_CLUSTER} \
 DSPANAMESPACE=${TARGET_NAMESPACE} \
 KUBECONFIGPATH=${KUBECONFIG_PATH}
```

#### Use existing DSPA install

For the impatient developer, you can use the following flag to skip DSPA install. This is useful when you want to make 
changes to a live environment and run the tests against it: 

```bash
go test ./... --tags=test_integration -v \
  -kubeconfig=${KUBECONFIG_PATH} \
  -k8sApiServerHost=${TARGET_CLUSTER} \
  -DSPANamespace=${TARGET_NAMESPACE} \
  -DSPAPath=resources/dspa-lite.yaml
  -skipDeploy=true \
  -skipCleanup=true
```

The `skipDeploy` and `skipCleanup` flags are independent, and can be added/left out as needed for your use case.

[kind-workflow]: ../.github/workflows/kind-integration.yml
