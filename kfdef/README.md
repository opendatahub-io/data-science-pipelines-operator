# Deploy latest DSPO via ODH 

To deploy the latest DSPO using the changes within this repo via Open Data Hub you can follow these steps


## Pre-requisites
1. An OpenShift cluster that is 4.10 or higher.
2. You will need to be logged into this cluster as [cluster admin] via [oc client].
3. The OpenShift Cluster must have OpenShift Pipelines 1.9 or higher installed. Instructions [here][OCP Pipelines Operator].
4. The Open Data Hub operator needs to be installed. You can install it via [OperatorHub][installodh].


## Deploy Kfdef

Clone this repository then run the following commands: 

```bash 
# If this namespace does not exist
oc new-project odh-applications 

# Then run
oc apply -f https://raw.githubusercontent.com/opendatahub-io/data-science-pipelines-operator/main/kfdef/kfdef.yaml -n odh-applications
```

Once done, follow the steps outlined [here][dspa] to get started with deploying your own 
`DataSciencePipelinesApplication` with the latest changes found within this repository.

[kfdef]: https://github.com/opendatahub-io/data-science-pipelines-operator/blob/main/kfdef/kfdef.yaml
[cluster admin]: https://docs.openshift.com/container-platform/4.12/authentication/using-rbac.html#creating-cluster-admin_using-rbac
[oc client]: https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest/openshift-client-linux.tar.gz
[OCP Pipelines Operator]: https://docs.openshift.com/container-platform/4.12/cicd/pipelines/installing-pipelines.html#op-installing-pipelines-operator-in-web-console_installing-pipelines
[installodh]: https://opendatahub.io/docs/getting-started/quick-installation.html
[dspa]: https://github.com/opendatahub-io/data-science-pipelines-operator#deploy-dsp-instance
