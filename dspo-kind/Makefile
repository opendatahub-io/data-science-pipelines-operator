MOD_ROOT=..
KIND_NAME ?= dspo
WORKSPACE=$(abspath $(CURDIR)/..)
TEST_RESOURCES=$(WORKSPACE)"/tests/resources"

# Container Build Params
CONTAINER_ENGINE ?= $(shell \
	if command -v docker >/dev/null 2>&1; then \
		echo docker; \
	elif command -v podman >/dev/null 2>&1; then \
		echo podman; \
	fi \
)
IMG_REGISTRY ?=
IMG_TAG_DSPO ?= dspo:1

# Integration test parameters
K8SAPISERVERHOST=
ENDPOINT_TYPE="service"
AWF_MANAGEMENT_STATE="Managed"
INTTEST_SKIP_CLEANUP ?= true
CLEAN_INFRA=false
SKIP_CLEANUP=false
SKIP_DEPLOY=false

# Build and push a custom DSPO image to IMG_REGISTRY
.PHONY: image-dspo
image-dspo:
	cd $(MOD_ROOT) && ${CONTAINER_ENGINE} build --platform linux/amd64 . -t $(IMG_REGISTRY)/$(IMG_TAG_DSPO)
	${CONTAINER_ENGINE} push $(IMG_REGISTRY)/$(IMG_TAG_DSPO)

# Deploy DSPO on a KinD cluster
.PHONY: dspo-kind-cluster
dspo-kind-cluster:
	# Deploy KinD cluster
	kind create cluster --name=$(KIND_NAME)
	# Set and deploy cluster requirements
	WORKSPACE=$(WORKSPACE) DSPO_IMAGE_REF=$(IMG_REGISTRY)/$(IMG_TAG_DSPO) ./dspo-kind-requirements.sh

# Deploy DSPO on a KinD cluster with external argo
.PHONY: dspo-kind-cluster-externalargo
dspo-kind-cluster-externalargo:
	# Deploy KinD cluster
	kind create cluster --name=$(KIND_NAME)
	# Set and deploy cluster requirements
	WORKSPACE=$(WORKSPACE) DSPO_IMAGE_REF=$(IMG_REGISTRY)/$(IMG_TAG_DSPO)  DEPLOY_EXTERNAL_ARGO="true" ./dspo-kind-requirements.sh

# Integration tests can only be run after deploying DSPO.
# Run integration tests with default settings.
.PHONY: dspo-integrationtest
dspo-integrationtest:
	cd $(MOD_ROOT) && \
	make integrationtest K8SAPISERVERHOST=$(K8SAPISERVERHOST) DSPANAMESPACE="test-dspa" DSPAPATH=$(TEST_RESOURCES)"/dspa-lite.yaml" ENDPOINT_TYPE=$(ENDPOINT_TYPE) INTTEST_AWF_MANAGEMENT_STATE=$(AWF_MANAGEMENT_STATE) INTTEST_SKIP_DEPLOY=$(SKIP_DEPLOY) INTTEST_SKIP_CLEANUP=$(SKIP_CLEANUP)

# Run integration tests with external connections enabled.
.PHONY: dspo-integrationtest-extconnection
dspo-integrationtest-extconnection:
	cd $(MOD_ROOT) && \
	make integrationtest K8SAPISERVERHOST=$(K8SAPISERVERHOST) DSPANAMESPACE="dspa-ext" DSPAPATH=$(TEST_RESOURCES)"/dspa-external-lite.yaml" ENDPOINT_TYPE=$(ENDPOINT_TYPE) MINIONAMESPACE="test-minio" INTTEST_AWF_MANAGEMENT_STATE=$(AWF_MANAGEMENT_STATE) INTTEST_SKIP_DEPLOY=$(SKIP_DEPLOY) INTTEST_SKIP_CLEANUP=$(SKIP_CLEANUP)

# Run integration tests with k8s storage enabled.
.PHONY: dspo-integrationtest-k8s
dspo-integrationtest-k8s:
	# Deploy cert manager
	kubectl create namespace "cert-manager"
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
	# Apply webhook certs
	kubectl wait -n "cert-manager" --timeout=90s --for=condition=Ready pods --all
	cd "$(WORKSPACE)/.github/resources/webhook" && kubectl -n "opendatahub" apply -k .
	# Run integration tests
	cd $(MOD_ROOT) && \
	make integrationtest K8SAPISERVERHOST=$(K8SAPISERVERHOST) DSPANAMESPACE="test-k8s-dspa" DSPAPATH=$(TEST_RESOURCES)"/dspa-k8s.yaml" ENDPOINT_TYPE=$(ENDPOINT_TYPE) INTTEST_AWF_MANAGEMENT_STATE=$(AWF_MANAGEMENT_STATE) INTTEST_SKIP_DEPLOY=$(SKIP_DEPLOY) INTTEST_SKIP_CLEANUP=$(SKIP_CLEANUP)
