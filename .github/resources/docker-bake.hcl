variable "TAG" {
  default = "latest"
}

variable "REGISTRY" {
  default = "localhost:5000"
}

variable "KFP_REPO" {
  default = "."
}

variable "ARGO_REPO" {
  default = ""
}

group "default" {
  targets = ["api-server", "driver", "launcher", "persistence-agent", "scheduled-workflow", "tests"]
}

group "argo" {
  targets = ["argo-workflowcontroller", "argo-argoexec"]
}

target "argo-workflowcontroller" {
  context = "${ARGO_REPO}"
  dockerfile = "Dockerfile"
  target = "workflow-controller"
  tags = ["${REGISTRY}/ds-pipelines-argo-workflowcontroller:${TAG}"]
}

target "argo-argoexec" {
  context = "${ARGO_REPO}"
  dockerfile = "Dockerfile"
  target = "argoexec-nonroot"
  tags = ["${REGISTRY}/ds-pipelines-argo-argoexec:${TAG}"]
}

target "api-server" {
  context = "${KFP_REPO}"
  dockerfile = "backend/Dockerfile"
  tags = ["${REGISTRY}/ds-pipelines-apiserver:${TAG}"]
}

target "driver" {
  context = "${KFP_REPO}"
  dockerfile = "backend/Dockerfile.driver"
  tags = ["${REGISTRY}/ds-pipelines-driver:${TAG}"]
}

target "launcher" {
  context = "${KFP_REPO}"
  dockerfile = "backend/Dockerfile.launcher"
  tags = ["${REGISTRY}/ds-pipelines-launcher:${TAG}"]
}

target "persistence-agent" {
  context = "${KFP_REPO}"
  dockerfile = "backend/Dockerfile.persistenceagent"
  tags = ["${REGISTRY}/ds-pipelines-persistenceagent:${TAG}"]
}

target "scheduled-workflow" {
  context = "${KFP_REPO}"
  dockerfile = "backend/Dockerfile.scheduledworkflow"
  tags = ["${REGISTRY}/ds-pipelines-scheduledworkflow:${TAG}"]
}

target "tests" {
  context = "${KFP_REPO}"
  dockerfile = "backend/test/images/Dockerfile.test"
  tags = ["${REGISTRY}/ds-pipelines-tests:${TAG}"]
}
