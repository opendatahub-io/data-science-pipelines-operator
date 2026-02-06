# How to Contribute

## Contribution Guidelines
See the below guidelines for contributing to DSPO.

## Deploy DSPO on Kind
Run integration tests on a local [kind](https://kind.sigs.k8s.io/) cluster.

### Prerequisites
- The [kind CLI](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) is installed.

### 1. Create Kind cluster and deploy DSPO
#### Optional: Deploy DSPO with non-main changes
By default, the commands above deploy an image built from the DSPO `main` branch. To deploy DSPO with local changes, run the following command to build and push the custom image to your image repository.
>Note: Set `IMG_REGISTRY` to your preferred image registry before running this command.

```shell
make image-dspo
```
Then update `config/overlays/kind-tests/kustomization.yaml` with the DSPO image you built and pushed above. For example:
```yaml
images:
- name: controller
  newName: quay.io/username/dspo
  newTag: 1
```


#### Deploy DSPO with default settings
The following command creates a kind cluster, sets up DSPO requirements and deploys DSPO on the cluster.
```shell
make kind-setup
```

#### OR Deploy DSPO with external Argo Workflows enabled
To deploy DSPO with external Argo Workflows enabled instead, execute the command below.
```shell
make kind-setup-externalargo
```

### 2. Run integration tests
`tests/suite_test.go` deploys DSPA and runs the integration tests, with different configurations in the following Make targets:
- `kind-integrationtest` deploys and runs integration tests with default settings.
- `kind-integrationtest-k8s` deploys and runs integration tests with Kubernetes pipeline storage enabled.
- `kind-integrationtest-extconnection` deploys and runs integration tests with external connections enabled.

#### Optional: Deploy DSPA with non-main changes
- Build and push DSP images to `IMG_REGISTRY`. You'll need to change directory to the `data-science-pipelines` project root.
>Note that the first time you push these images to Quay (if using Quay), you will need to set each image as public.

```shell
make -C backend image_apiserver
${CONTAINER_ENGINE} push quay.io/username/apiserver:1
```
```shell
make -C backend image_driver
${CONTAINER_ENGINE} push quay.io/username/driver:1
```
```shell
make -C backend image_launcher
${CONTAINER_ENGINE} push quay.io/username/launcher:1
```
- Update the appropriate test manifest. The table below lists each integration test target and its corresponding manifest file.

| Test Makefile target                 | Manifest path                             |
|--------------------------------------|-------------------------------------------|
| `kind-integrationtest`               | `tests/resources/dspa-lite.yaml`          |
| `kind-integrationtest-k8s`           | `tests/resources/dspa-k8s.yaml`           |
| `kind-integrationtest-extconnection` | `tests/resources/dspa-external-lite.yaml` |

For example:
```yaml
  apiServer:
    image:  quay.io/username/apiserver:1
    argoLauncherImage:  quay.io/username/launcher:1
    argoDriverImage:  quay.io/username/driver:1
```
