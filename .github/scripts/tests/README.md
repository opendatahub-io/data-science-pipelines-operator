# Setup the local environment

All the following commands must be executed in a single terminal instance.

## Increase inotify Limits
To prevent file monitoring issues in development environments (e.g., IDEs or file sync tools), increase inotify limits:
```bash
sudo sysctl fs.inotify.max_user_instances=2280
sudo sysctl fs.inotify.max_user_watches=1255360
```
## Create kind cluster
```bash
cat <<EOF | kind create cluster --name=kubeflow --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.31.0@sha256:53df588e04085fd41ae12de0c3fe4c72f7013bba32a20e7325357a1ac94ba865
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        "service-account-issuer": "kubernetes.default.svc"
        "service-account-signing-key-file": "/etc/kubernetes/pki/sa.key"
EOF
```

## kubeconfig
Instead of replacing your kubeconfig, we are going to set to a diff file
```bash
kind get kubeconfig --name kubeflow > /tmp/kubeflow-config
export KUBECONFIG=/tmp/kubeflow-config
```
# docker
In order to by pass the docker limit issue while downloading the images. Let's use your credentials
```bash
docker login
```

Upload the secret. The following command will return an error. You need to replace `to` with user `username`
```bash
kubectl create secret generic regcred \
--from-file=.dockerconfigjson=/home/to/.docker/config.json \
--type=kubernetes.io/dockerconfigjson
```

# Test environment variables
Replace the `/path/to` in order to match the `data-science-pipelines-operator` folder
```bash
export GIT_WORKSPACE=/path/to/data-science-pipelines-operator
```
The image registry is required because you are running the test locally.
It will build and upload the image to your repository.

Replace `username` with your quay user
```bash
export REGISTRY_ADDRESS=quay.io/username
```

# Run the test
```bash
sh .github/scripts/tests/tests.sh --kind
```
