# Data Science Pipelines Operator

The Data Science Pipelines Operator (DSPO) is an OpenShift Operator that is used to deploy single namespace scoped
Data Science Pipeline stacks onto individual OCP namespaces.

## Table of Contents

- [Data Science Pipelines Operator](#data-science-pipelines-operator)
  - [Table of Contents](#table-of-contents)
  - [Overview](#overview)
  - [Quickstart](#quickstart)
  - [Pre-requisites](#pre-requisites)
  - [Deploy the Operator via ODH](#deploy-the-operator-via-odh)
    - [Using a development image](#using-a-development-image)
  - [Deploy the Operator standalone](#deploy-the-operator-standalone)
  - [Deploy DSP instance](#deploy-dsp-instance)
    - [Deploy another DSP instance](#deploy-another-dsp-instance)
    - [Deploy a DSP with custom credentials](#deploy-a-dsp-with-custom-credentials)
    - [Deploy a DSP with external Object Storage](#deploy-a-dsp-with-external-object-storage)
  - [DataSciencePipelinesApplication Component Overview](#datasciencepipelinesapplication-component-overview)
  - [Deploying Optional Components](#deploying-optional-components)
    - [MariaDB](#mariadb)
    - [Minio](#minio)
    - [ML Pipelines UI](#ml-pipelines-ui)
    - [ML Metadata](#ml-metadata)
  - [Using a DataSciencePipelinesApplication](#using-a-datasciencepipelinesapplication)
  - [Using the Graphical UI](#using-the-graphical-ui)
  - [Using the API](#using-the-api)
  - [Cleanup](#cleanup)
  - [Cleanup ODH Installation](#cleanup-odh-installation)
  - [Cleanup Standalone Installation](#cleanup-standalone-installation)
  - [Run tests](#run-tests)
  - [Metrics](#metrics)
  - [Configuring Log Levels for the Operator](#configuring-log-levels-for-the-operator)
  - [Deployment and Testing Guidelines for Developers](#deployment-and-testing-guidelines-for-developers)

## Overview

Data Science Pipelines (DSP) allows data scientists to track progress as they
iterate over development of ML models. With DSP, a data scientist can create
workflows for data preparation, model training, model validation, and more.
They can create and track experiements to arrive at the best version of of
training data, model hyperparameters, model code, etc., and repeatably
rerun these experiments.

Data Science Pipelines is based on the upstream [Kubeflow Pipelines (KFP)][kfp]
project. We leverage the [kfp-tekton][kfp] project to run pipelines backed
by the Tekton (rather than Argo, which is the default choice in KFP). We
currently distributed version 1.x of KFP, and are working to support v2.

Data Scientists can use tools like the
[kfp-tekton SDK](https://github.com/kubeflow/kfp-tekton/blob/master/sdk/README.md)
or [Elyra](https://github.com/elyra-ai/elyra) to author their workflows, and
interact with them in the
[ODH dashbard](https://github.com/opendatahub-io/odh-dashboard).

## Quickstart

To get started you will first need to satisfy the following pre-requisites:

## Pre-requisites

1. An OpenShift cluster that is 4.11 or higher.
2. You will need to be logged into this cluster as [cluster admin] via [oc client].
3. Based on which DSP version to install you will need to do the following:
   1. For DSPv1: The OpenShift Cluster must have OpenShift Pipelines 1.8 or higher installed. We recommend channel pipelines-1.8
      on OCP 4.10 and pipelines-1.9 or pipelines-1.10 for OCP 4.11, 4.12 and 4.13. Instructions [here][OCP Pipelines Operator].
   2. For DSPv2: The OpenShift Cluster must be Argo Workflows installed. You can follow the steps listed in the standalone deployment section [here](#deploy-the-operator-standalone).
4. Based on installation type you will need one of the following:
   1. For Standalone method: You will need to have [Kustomize] version 4.5+ installed
   2. For ODH method: The Open Data Hub operator needs to be installed. You can install it via [OperatorHub][installodh].

## Deploy the Operator via ODH

On a cluster with ODH installed, create a namespace where you would like to install DSPO:
Deploy the following `DataScienceCluster`:

```bash
ODH_NS=opendatahub
oc new-project ${ODH_NS}
```

Then deploy the following `DataScienceCluster` into the namespace created above:

```bash
cat <<EOF | oc apply -f -
kind: DataScienceCluster
apiVersion: datasciencecluster.opendatahub.io/v1
metadata:
  name: data-science-cluster
  namespace: ${ODH_NS}
spec:
  components:
    dashboard:
      managementState: Managed
    datasciencepipelines:
      managementState: Managed
EOF
```

> ℹ️ **Note:**
>
> You can also deploy other ODH components using DataScienceCluster`. See <https://github.com/opendatahub-io/opendatahub-operator#example-datasciencecluster> for more information.

Confirm the pods are successfully deployed and reach running state:

```bash
oc get pods -n ${ODH_NS}
```

Once all pods are ready, we can proceed to deploying the first Data Science Pipelines (DSP) instance. Instructions
[here](#deploy-dsp-instance).

### Using a development image

You can use custom manifests from a branch or tag to use a different Data Science Pipelines Operator image. To do so, modify the `IMAGES_DSPO` config in [config/base/params.env](config/base/params.env) and push the changes to a branch or tag.

Create a (or edit the existent) `DataSciencePipelines` adding `devFlags.manifests` with the URL of your branch or tag. For example, given the following repository and branch:

- Repository: `https://github.com/a_user/data-science-pipelines-operator`
- Branch: `my_branch`

The `DataSciencePipelines` YAML should look like:

```yaml
kind: DataScienceCluster
apiVersion: datasciencecluster.opendatahub.io/v1
metadata:
   name: data-science-cluster
   namespace: ${ODH_NS}
spec:
  components:
    dashboard:
      managementState: Managed
    datasciencepipelines:
      managementState: Managed
      devFlags:
        manifests:
          - uri: https://github.com/a_user/data-science-pipelines-operator/tarball/my_branch
            contextDir: config
            sourcePath: base
```

## Deploy the Operator standalone

First clone this repository:

```bash
WORKING_DIR=$(mktemp -d)
git clone https://github.com/opendatahub-io/data-science-pipelines-operator.git ${WORKING_DIR}
```

If you already have the repository cloned, set `WORKING_DIR=` to its absolute location accordingly.

DSPO can be installed in any namespace, we'll deploy it in the following namespace, you may update this environment
variable accordingly.

```bash
ODH_NS=opendatahub

# Create the namespace if it doesn't already exist
oc new-project ${ODH_NS}
```

Now we will navigate to the DSPO manifests then build and deploy them to this namespace.

Run the following to deploy the ful DSPO v2 stack.

```bash
cd ${WORKING_DIR}
make deploy OPERATOR_NS=${ODH_NS}
```

Confirm the pods are successfully deployed and reach running state:

```bash
oc get pods -n ${ODH_NS}
```

Once all pods are ready, we can proceed to deploying the first Data Science Pipelines (DSP) instance. Instructions
[here](#deploy-dsp-instance).

## Deploy DSP instance

We'll deploy the first instance in the following namespace.

```bash
# You may update this value to the namespace you would like to use.
DSP_Namespace=test-ds-project-1
oc new-project ${DSP_Namespace}
```

DSPO introduces a new custom resource to your cluster called `DataSciencePipelinesApplication`. This resource is where you configure your DSP
components, your DB and Storage configurations, and so forth. For now, we'll use the sample one provided, feel free to
inspect this sample resource to see other configurable options.

```bash
cd ${WORKING_DIR}/config/samples/v2/dspa-simple
kustomize build . | oc -n ${DSP_Namespace} apply -f -
```

> Note: the sample CR used here deploys a minio instance so DSP may work out of the box
> this is unsupported in production environments and we recommend to provide your own
> object storage connection details via spec.objectStorage.externalStorage
> see ${WORKING_DIR}/config/samples/v2/external-object-storage/dspa.yaml for an example.

Confirm all pods reach ready state by running:

```bash
oc get pods -n ${DSP_Namespace}
```

For instructions on how to use this DSP instance refer to these instructions: [here](#using-a-datasciencepipelinesapplication).

### Deploy another DSP instance

You can use the DSPO to deploy multiple `DataSciencePipelinesApplication` instances in different OpenShift namespaces, for example earlier we deployed a `DataSciencePipelinesApplication` resource named `sample`. We can use this again and deploy it to a different namespace:

```bash
DSP_Namespace_2=test-ds-project-2
oc new-project ${DSP_Namespace_2}
cd ${WORKING_DIR}/config/samples/v2/dspa-simple
kustomize build . | oc -n ${DSP_Namespace_2} apply -f -
```

### Deploy a DSP with custom credentials

Using DSPO you can specify custom credentials for Database and Object storage. If specifying external connections, this
is required. You can also provide secrets for the built in MariaDB and Minio deployments. To see a sample configuration
you can simply investigate and deploy the following path:

```bash
DSP_Namespace_3=test-ds-project-3
oc new-project ${DSP_Namespace_3}
cd ${WORKING_DIR}/config/samples/v2/custom-configs
kustomize build . | oc -n ${DSP_Namespace_3} apply -f -
```

Notice the introduction of 2 `secrets` `testdbsecret`, `teststoragesecret` and 2 `configmaps` `custom-ui-configmap` and
`custom-artifact-script`. The `secrets` allow you to provide your own credentials for the DB and MariaDB connections.

These can be configured by the end user as needed.

### Deploy a DSP with external Object Storage

To specify a custom Object Storage (example an AWS s3 bucket) you will need to provide DSPO with your S3 credentials in
the form of a k8s `Secret`, see an example of such a secret here `config/samples/v2/external-object-storage/storage-creds.yaml`.

DSPO can deploy a DSPA instance and use this S3 bucket for storing its metadata and pipeline artifacts. A sample
configuration for a DSPA that does this is found in `config/samples/v2/external-object-storage`, you can update this as
needed, and deploy this DSPA by running the following:

```bash
DSP_Namespace_3=test-ds-project-4
oc new-project ${DSP_Namespace_4}
cd ${WORKING_DIR}/config/samples/v2/external-object-storage
kustomize build . | oc -n ${DSP_Namespace_3} apply -f -
```

## DataSciencePipelinesApplication Component Overview

When a `DataSciencePipelinesApplication` is deployed, the following components are deployed in the target namespace:

- APIServer
- Persistence Agent
- Scheduled Workflow controller

If specified in the `DataSciencePipelinesApplication` resource, the following components may also be additionally deployed:

- MariaDB
- Minio
- MLPipelines UI
- MLMD (ML Metadata)

To understand how these components interact with each other please refer to the upstream
[Kubeflow Pipelines Architectural Overview] documentation.

## Deploying Optional Components

### MariaDB

To deploy a standalone MariaDB metadata database (rather than providing your own database connection details), simply add a `mariaDB` item under the `spec.database` in your DSPA definition with an `deploy` key set to `true`.  All other fields are defaultable/optional, see [All Fields DSPA Example](config/samples/v2/dspa-all-fields/dspa_all_fields.yaml) for full details.  Note that this component is mutually exclusive with externally-provided databases (defined by `spec.database.externalDB`).

```yaml
apiVersion: datasciencepipelinesapplications.opendatahub.io/v1alpha1
kind: DataSciencePipelinesApplication
metadata:
  name: sample
spec:
   ...
  database:
    mariaDB:   # mutually exclusive with externalDB
      deploy: true

```

### Minio

To deploy a Minio Object Storage component (rather than providing your own object storage connection details), simply add a `minio` item under the `spec.objectStorage` in your DSPA definition with an `image` key set to a valid minio component container image.  All other fields are defaultable/optional, see [All Fields DSPA Example](config/samples/v2/dspa-all-fields/dspa_all_fields.yaml) for full details.  Note that this component is mutually exclusive with externally-provided object stores (defined by `spec.objectStorage.externalStorage`).

```yaml
apiVersion: datasciencepipelinesapplications.opendatahub.io/v1alpha1
kind: DataSciencePipelinesApplication
metadata:
  name: sample
spec:
   ...
  objectStorage:
    minio:  # mutually exclusive with externalStorage
      deploy: true
      # Image field is required
      image: 'quay.io/opendatahub/minio:RELEASE.2019-08-14T20-37-41Z-license-compliance'
```

### ML Pipelines UI

To deploy the standalone DS Pipelines UI component, simply add a `spec.mlpipelineUI` item to your DSPA with an `image` key set to a valid ui component container image.  All other fields are defaultable/optional, see [All Fields DSPA Example](config/samples/v2/dspa-all-fields/dspa_all_fields.yaml) for full details.

```yaml
apiVersion: datasciencepipelinesapplications.opendatahub.io/v1alpha1
kind: DataSciencePipelinesApplication
metadata:
  name: sample
spec:
   ...
  mlpipelineUI:
    deploy: true
    # Image field is required
    image: 'quay.io/opendatahub/odh-ml-pipelines-frontend-container:beta-ui'
```

### ML Metadata

To deploy the ML Metadata artifact linage/metadata component, simply add a `spec.mlmd` item to your DSPA with `deploy` set to `true`.  All other fields are defaultable/optional, see [All Fields DSPA Example](config/samples/v2/dspa-all-fields/dspa_all_fields.yaml) for full details.

```yaml
apiVersion: datasciencepipelinesapplications.opendatahub.io/v1alpha1
kind: DataSciencePipelinesApplication
metadata:
  name: sample
spec:
   ...
   mlmd:
      deploy: true
```

## Using a DataSciencePipelinesApplication

When a `DataSciencePipelinesApplication` is deployed, use the MLPipelines UI endpoint to interact with DSP, either via a GUI or via API calls.

You can retrieve this route by running the following in your terminal:

```bash
DSP_CR_NAME=sample
DSP_Namespace=test-ds-project-1
echo https://$(oc get routes -n ${DSP_Namespace} ds-pipeline-ui-${DSP_CR_NAME} --template={{.spec.host}})
```

## Using the Graphical UI

> Note the UI presented below is the upstream Kubeflow Pipelines UI, this is not supported in DSP and will be replaced
> with the ODH Dashboard UI. Until then, this UI can be deployed via DSPO for experimentation/development purposes.
> Note however that this UI is not a supported feature of DSPO/ODH.

Navigate to the route retrieved in the last step. You will be presented with the MLPipelines UI. In this walkthrough we
will upload a pipeline and start a run based off it.

To start, click the "Upload Pipeline" button.

![pipeline](docs/images/upload_pipeline.png)

Choose a file, you can use the [flipcoin example]. Download this example and select it for the first step. Then click
"Create" for the second step.

![flipcoin](docs/images/upload_flipcoin.png)

Once done, you can now use this `Pipeline` to create a `Run`, do this by pressing "+ Create Run".

![create](docs/images/create_run.png)

On this page specify the `Pipeline` we just uploaded if it's not already auto selected. Similarly with the version,
with the example there will only be one version listed. Give this `Run` a name, or keep the default as is. Then click
"Start".

![start](docs/images/start_run.png)

Once you click start you will be navigated to the `Runs` page where you can see your previous runs that are have been
executed, or currently executing. You can click the most recently started Run to view it's execution graph.

![started](docs/images/started_run.png)

You should see something similar to the following once the `Run` completes.

![executed](docs/images/executed_run.png)

Click the first "flip-coin" step. This step produces an output message either "heads" or "tails", confirm that you can
see these logs after clicking this step and navigating to "Logs."

![logs](docs/images/logs.png)

## Using the API

> Note: By default we use kfp-tekton 1.5.x for this section so you will need [kfp-tekton v1.5.x sdk installed][kfp-tekton]
> in your environment

In the previous step we submitted a generated `Pipeline` yaml via the GUI. We can also submit the `Pipeline` code
directly either locally or via a notebook.

You can find the `Pipeline` code example [here][flipcoin code example]. We can submit this to the DSP API Server by
including this code in the following Python script:

```bash
cd ${WORKING_DIR}/docs/example_pipelines
touch execute_pipeline.py
```

We can utilize the flip coin example in this location and submit it directly to the API server by doing the following:

```python
# Add this to execute_pipeline.py we created earlier
import os
import kfp_tekton
from condition import flipcoin_pipeline
token = os.getenv("OCP_AUTH_TOKEN")
route = os.getenv("DSP_ROUTE")
client = kfp_tekton.TektonClient(host=route, existing_token=token)

client.create_run_from_pipeline_func(pipeline_func=flipcoin_pipeline, arguments={})
```

> Note: If you are in an unsecured cluster, you may encounter `CERTIFICATE_VERIFY_FAILED` error, to work around this
> you can pass in the self-signed certs to the kfp-tekton client. For instance, if running inside a notebook or pod
> on the same cluster, you can do the following:

```python
...
cert = "/run/secrets/kubernetes.io/serviceaccount/ca.crt"
client = kfp_tekton.TektonClient(host=route, existing_token=token, ssl_ca_cert=cert)
client.create_run_from_pipeline_func(pipeline_func=flipcoin_pipeline, arguments={})
```

Retrieve your token and DSP route:

```bash
# This is the namespace you deployed the DataSciencePipelinesApplication Custom Resource
DSP_Namespace=test-ds-project-1
# This is the metadata.name of that DataSciencePipelinesApplication Custom Resource
DSP_CR_NAME=sample
export DSP_ROUTE="https://$(oc get routes -n ${DSP_Namespace} ds-pipeline-ui-${DSP_CR_NAME} --template={{.spec.host}})"
export OCP_AUTH_TOKEN=$(oc whoami --show-token)
```

And finally execute this script and submit the flip coin example:

```bash
python execute_pipeline.py
```

You can navigate to the UI again and find your newly created run there, or you could amend the script above and list
the runs via `client.list_runs()`.

## Cleanup

To remove a `DataSciencePipelinesApplication` from your cluster, run:

```bash
# Replace environment variables accordingly
oc delete ${YOUR_DSPIPELINE_NAME} -n ${YOUR_DSPIPELINES_NAMESPACE}
```

The DSPO will clean up all manifests associated with each `DataSciencePipelinesApplication` instance.

Or you can remove all `DataSciencePipelinesApplication` instances in your whole cluster by running the following:

```bash
oc delete DataSciencePipelinesApplication --all -A
```

Depending on how you installed DSPO, follow the instructions below accordingly to remove the operator:

## Cleanup ODH Installation

To uninstall DSPO via ODH run the following:

```bash
DSC_NAME=$(oc get DataScienceCluster -o jsonpath='{.items[0].metadata.name}')
ODH_NS=$(oc get DataScienceCluster -o jsonpath='{.items[0].metadata.namespace}')
oc delete datasciencecluster ${DSC_NAME} -n "${ODH_NS}"
```

## Cleanup Standalone Installation

To clean up standalone DSPO deployment:

```bash
# WORKING_DIR must be the root of this repository's clone
cd ${WORKING_DIR}
make undeploy OPERATOR_NS=${ODH_NS}
oc delete project ${ODH_NS}
```

## Run tests

Simply clone the directory and execute `make test`.

To run it without `make` you can run the following:

```bash
tmpFolder=$(mktemp -d)
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
export KUBEBUILDER_ASSETS=$(${GOPATH}/bin/setup-envtest use 1.25.0 --bin-dir ${tmpFolder}/bin -p path)
go test ./... -coverprofile cover.out

# once $KUBEBUILDER_ASSETS you can also run the full test suite successfully by running:
pre-commit run --all-files
```

You can find a more permanent location to install `setup-envtest` into on your local filesystem and export
`KUBEBUILDER_ASSETS` into your `.bashrc` or equivalent. By doing this you can always run `pre-commit run --all-files`
without having to repeat these steps.

## Metrics

The Data Science Pipelines Operator exposes standard operator-sdk metrics for controller monitoring purposes.  
In addition to these metrics, DSPO also exposes several custom metrics for monitoring the status of the DataSciencePipelinesApplications that it owns.

They are as follows:

- `data_science_pipelines_application_apiserver_ready` - Gauge that indicates if the DSPA's APIServer is in a Ready state (1 => Ready, 0 => Not Ready)
- `data_science_pipelines_application_persistenceagent_ready` - Gauge that indicates if the DSPA's PersistenceAgent is in a Ready state (1 => Ready, 0 => Not Ready)
- `data_science_pipelines_application_scheduledworkflow_ready` - Gauge that indicates if the DSPA's ScheduledWorkflow manager is in a Ready state (1 => Ready, 0 => Not Ready)
- `data_science_pipelines_application_ready` - Gauge that indicates if the DSPA is in a fully Ready state (1 => Ready, 0 => Not Ready)

## Configuring Log Levels for the Operator

By default, the operator's log messages are set to `info` severity.
If you wish to adjust the log verbosity, you can do so by modifying the `ZAP_LOG_LEVEL` parameter in [params.env](config/base/params.env) file to your preferred severity level.

For a comprehensive list of available values, please consult the [Zap documentation](https://pkg.go.dev/go.uber.org/zap#pkg-constants).

## Deployment and Testing Guidelines for Developers

**To build the DSPO locally :**

Login oc using following command:

```bash
oc login                                                   
```

Install CRDs using the following command:

```bash
kubectl apply -k config/crd                                
```

Execute the following command:

```bash
go build main.go                                           
```

**To run the DSPO locally :**

Execute the following command:

```bash
go run main.go --config=$config
```

Below is the sample config file (the tags for the images can be edited as required):

```bash
Images:
  ApiServer: quay.io/opendatahub/ds-pipelines-api-server:latest
  Artifact: quay.io/opendatahub/ds-pipelines-artifact-manager:latest
  OAuthProxy: registry.redhat.io/openshift4/ose-oauth-proxy:v4.12.0
  PersistentAgent: quay.io/opendatahub/ds-pipelines-persistenceagent:latest
  ScheduledWorkflow: quay.io/opendatahub/ds-pipelines-scheduledworkflow:latest
  Cache: registry.access.redhat.com/ubi8/ubi-minimal
  MoveResultsImage: registry.access.redhat.com/ubi8/ubi-micro
  MariaDB: registry.redhat.io/rhel8/mariadb-103:1-188
  MlmdEnvoy: quay.io/opendatahub/ds-pipelines-metadata-envoy:latest
  MlmdGRPC: quay.io/opendatahub/ds-pipelines-metadata-grpc:latest
  MlmdWriter: quay.io/opendatahub/ds-pipelines-metadata-writer:latest  
```

**To build your own images :**

All the component images are available [here][component-images] and for thirdparty images [here][thirdparty-images]. Build these images from root as shown in the below example:

```bash
podman build . -f backend/Dockerfile -t quay.io/your_repo/dsp-apiserver:sometag
```

**To run the tests:**

Execute `make test` or `make unittest` or `make functest` based on the level of testing as mentioned below:

`make unittest` is a command that is often used to run only unit tests for individual units or components. These tests verify that each unit of code (e.g., functions or methods) behaves as expected. It is suggested to run this command often during the development process.

`make functest` is a command used to run functional tests which assess the overall functionality of the software by testing its features and user interactions. Functional tests help ensure that the software works as a whole and that its features are functioning correctly from a user's perspective. It is suggested to run make functest for every commit.

The specific tests that are executed when you run `make test` can include unit tests, functional tests and more. It is a test to check if the software behaves correctly and meets the desired quality standards. It helps identify and fix issues early in the development process. It is suggested to run make test before creating a PR.

**To deploy DSPO as a developer :**

Follow the instructions from [here](#deploy-the-operator-standalone) to deploy the operator standalone.

Follow the instructions from [here](#deploy-the-operator-via-odh) to deploy the operator via ODH.

**How to deploy with a custom image:**

Run the following command using the custom image:

```bash
make deploy IMG=my-registry/my-operator:v1
```

**How to regenerate manifests:**

After updating the Kubebuilder annotations in your code, run the following command to regenerate code and manifests:

```bash
make generate manifests
```

**How to regenerate crd on api changes:**

After making your API changes, run the following command to regenerate code and CRDs based on your updated API definitions:

```bash
make generate
```

Refer to kubebuilder docs [here][kubebuilder-docs] for more info.

**How to run pre-commit tests:**

Install pre-commit following the instructions [here][pre-commit-installation]. Before creating a PR, developers should run the following command which will auto fix any simple errors:

```bash
pre-commit run --all-files
```

**How to do disable health checks when dev testing:**

To disable the health checks set the values to true in the DSPA yaml file you apply. Refer to his sample file [here][dspa-yaml].

In certain scenarios, it may be necessary to disable health checks within our environment. When the DSPO is executed either locally or on a different cluster, the health checks can't reach the database and Object Store endpoints. Consequently, they remain unsuccessful, preventing the deployment of essential pipeline infrastructure components by the DSPA. To address this challenge, we have introduced the `disableHealthCheck` mechanism as a viable solution.

**How to enable kfp ui and minio:**

Refer to this [sample][sample-yaml] yaml file for enabling the upstream kubeflow pipelines ui and minio.

Refer to this [repo][kubeflow-pipelines-examples] to see examples of different pipelines for dev testing.

[cluster admin]: https://docs.openshift.com/container-platform/4.12/authentication/using-rbac.html#creating-cluster-admin_using-rbac
[oc client]: https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest/openshift-client-linux.tar.gz
[OCP Pipelines Operator]: https://docs.openshift.com/container-platform/4.12/cicd/pipelines/installing-pipelines.html#op-installing-pipelines-operator-in-web-console_installing-pipelines
[Kustomize]: https://kubectl.docs.kubernetes.io/installation/kustomize/
[Kubeflow Pipelines Architectural Overview]: https://www.kubeflow.org/docs/components/pipelines/v1/introduction/#architectural-overview
[flipcoin example]: https://github.com/opendatahub-io/data-science-pipelines-operator/blob/main/docs/example_pipelines/condition.yaml
[flipcoin code example]: https://github.com/opendatahub-io/data-science-pipelines-operator/blob/main/docs/example_pipelines/condition.py
[installodh]: https://opendatahub.io/docs/quick-installation
[kfp-tekton]: https://github.com/kubeflow/kfp-tekton
[kfp]: https://github.com/kubeflow/pipelines
[component-images]: https://github.com/opendatahub-io/data-science-pipelines/tree/master/backend
[thirdparty-images]: https://github.com/opendatahub-io/data-science-pipelines/tree/master/third-party
[pre-commit-installation]: https://pre-commit.com/
[kubebuilder-docs]: https://book.kubebuilder.io/
[dspa-yaml]: https://github.com/opendatahub-io/data-science-pipelines-operator/blob/main/config/samples/v2/dspa-all-fields/dspa_all_fields.yaml#L77
[sample-yaml]: https://github.com/opendatahub-io/data-science-pipelines-operator/blob/main/config/samples/v2/dspa-simple/dspa_simple.yaml
[kubeflow-pipelines-examples]: https://github.com/rh-datascience-and-edge-practice/kubeflow-pipelines-examples
