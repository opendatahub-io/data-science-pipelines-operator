# Deploy DSPO on KinD

## 1. Create KinD cluster and deploy DSPO
```shell
make dspo-kind-cluster
```
To deploy DSPO with external Argo, run the command below instead.
```shell
make dspo-kind-cluster-externalargo
```
### Using a custom DSPO image
By default, the commands above deploy an image built from DSPO `main`. To use a different branch, check out the branch and run the command below.

*Note: update Makefile IMG_REGISTRY with your preferred image registry.*
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

## 2. Run integration tests (creates DSPA)
DSPA is deployed when the integration tests are executed, in file `tests/suite_test.go`. There are three integration test Make targets:
- `dspo-integrationtest` runs the tests with default settings.
- `dspo-integrationtest-extconnection` runs the tests with external connections enabled.
- `dspo-integrationtest-k8s` runs the tests with Kubernetes pipeline storage enabled.

### Create DSPA with non-master DSP images
Build and push DSP images to your local image registry and update the test manifests in the table with the corresponding YAML block. Fill in your own image registry and tag.

*Note that the first time you push these images to Quay (if using Quay), you will need to set each image as public.*
```shell
make -C backend image_apiserver
podman push quay.io/username/apiserver:1
```
```shell
make -C backend image_driver
podman push quay.io/username/driver:1
```
```shell
make -C backend image_launcher
podman push quay.io/username/launcher:1
```

| Test Makefile target               | Manifest path                             |
|------------------------------------|-------------------------------------------|
| `dspo-integrationtest`               | `tests/resources/dspa-lite.yaml`          |
| `dspo-integrationtest-extconnection` | `tests/resources/dspa-external-lite.yaml` |
| `dspo-integrationtest-k8s`           | `tests/resources/dspa-k8s.yaml`           |
For example:
```yaml
  apiServer:
    image:  quay.io/username/apiserver:1
    argoLauncherImage:  quay.io/username/launcher:1
    argoDriverImage:  quay.io/username/driver:1
```
