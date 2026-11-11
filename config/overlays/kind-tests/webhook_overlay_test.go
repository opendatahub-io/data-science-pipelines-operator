//go:build test_all || test_unit

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kindtests_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/yaml"
)

// TestWebhookCertificateUsesOperatorNamespace verifies that the Kind overlay
// renders webhook certificates and CA-injection settings for the operator namespace.
func TestWebhookCertificateUsesOperatorNamespace(t *testing.T) {
	for _, namespace := range []string{"opendatahub", "rhods"} {
		t.Run(namespace, func(t *testing.T) {
			// Match make deploy-kind: select the namespace on the Kind overlay
			// before rendering all of its resources together.
			configDirectory := filepath.Join(t.TempDir(), "config")
			require.NoError(t, os.CopyFS(configDirectory, os.DirFS("../..")))
			overlayDirectory := filepath.Join(configDirectory, "overlays", "kind-tests")
			kustomizationPath := filepath.Join(overlayDirectory, "kustomization.yaml")
			data, err := os.ReadFile(kustomizationPath)
			require.NoError(t, err)
			var kustomization map[string]interface{}
			require.NoError(t, yaml.Unmarshal(data, &kustomization))
			kustomization["namespace"] = namespace
			data, err = yaml.Marshal(kustomization)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(kustomizationPath, data, 0600))

			kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
			resources, err := kustomizer.Run(filesys.MakeFsOnDisk(), overlayDirectory)
			require.NoError(t, err)
			var certificate, issuer, deployment *resource.Resource
			for _, manifest := range resources.Resources() {
				switch {
				case manifest.GetKind() == "Certificate" && manifest.GetName() == "dspa-webhook-cert":
					certificate = manifest
				case manifest.GetKind() == "Issuer" && manifest.GetName() == "dspa-webhook-selfsigned-issuer":
					issuer = manifest
				case manifest.GetKind() == "Deployment" && manifest.GetName() == "data-science-pipelines-operator-controller-manager":
					deployment = manifest
				}
			}
			require.NotNil(t, certificate, "Kind deployment must include the webhook certificate")
			require.NotNil(t, issuer)
			require.Equal(t, namespace, certificate.GetNamespace())
			require.Equal(t, namespace, issuer.GetNamespace())
			dnsNames, err := certificate.GetFieldValue("spec.dnsNames")
			require.NoError(t, err)
			require.Equal(t, []interface{}{
				"ds-pipelines-webhook",
				"ds-pipelines-webhook." + namespace,
				"ds-pipelines-webhook." + namespace + ".svc",
				"ds-pipelines-webhook." + namespace + ".svc.cluster.local",
			}, dnsNames)
			secretName, err := certificate.GetFieldValue("spec.secretName")
			require.NoError(t, err)
			require.Equal(t, "ds-pipelines-webhook-tls", secretName)
			issuerName, err := certificate.GetFieldValue("spec.issuerRef.name")
			require.NoError(t, err)
			require.Equal(t, issuer.GetName(), issuerName)

			require.NotNil(t, deployment)
			require.Equal(t, namespace, deployment.GetNamespace())
			data, err = deployment.AsYAML()
			require.NoError(t, err)
			var operatorDeployment appsv1.Deployment
			require.NoError(t, yaml.Unmarshal(data, &operatorDeployment))
			namespaceIndex, annotationsIndex := -1, -1
			var annotationsValue string
			for _, container := range operatorDeployment.Spec.Template.Spec.Containers {
				if container.Name != "manager" {
					continue
				}
				for index, variable := range container.Env {
					switch variable.Name {
					case "DSPO_NAMESPACE":
						namespaceIndex = index
						require.NotNil(t, variable.ValueFrom)
						require.NotNil(t, variable.ValueFrom.FieldRef)
						require.Equal(t, "metadata.namespace", variable.ValueFrom.FieldRef.FieldPath)
					case "WEBHOOK_ANNOTATIONS":
						annotationsIndex = index
						annotationsValue = variable.Value
					}
				}
			}
			require.NotEqual(t, -1, namespaceIndex)
			require.Greater(t, annotationsIndex, namespaceIndex, "Kubernetes expands only preceding variables")
			// The placeholder must remain in the manifest: kubelet expands it at
			// startup. TestKindWebhookTLS checks the resulting live annotation.
			require.JSONEq(t, `{"cert-manager.io/inject-ca-from":"$(DSPO_NAMESPACE)/dspa-webhook-cert"}`, annotationsValue)
		})
	}
}
