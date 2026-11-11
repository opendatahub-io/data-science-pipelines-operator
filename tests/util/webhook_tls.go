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

package testUtil

import (
	"bytes"
	"context"
	"fmt"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckKindWebhookTLS verifies the annotations written by the running operator
// and the CA bundles injected by cert-manager against the certificate Secret.
func CheckKindWebhookTLS(ctx context.Context, kubernetesClient client.Client, operatorNamespace string) error {
	secret := &corev1.Secret{}
	if err := kubernetesClient.Get(ctx, client.ObjectKey{Namespace: operatorNamespace, Name: "ds-pipelines-webhook-tls"}, secret); err != nil {
		return fmt.Errorf("get webhook certificate Secret: %w", err)
	}
	certificateAuthority := secret.Data["ca.crt"]
	if len(certificateAuthority) == 0 {
		return fmt.Errorf("webhook certificate Secret has no ca.crt")
	}
	for _, configuration := range []client.Object{&admissionv1.ValidatingWebhookConfiguration{}, &admissionv1.MutatingWebhookConfiguration{}} {
		if err := kubernetesClient.Get(ctx, client.ObjectKey{Name: "pipelineversions.pipelines.kubeflow.org"}, configuration); err != nil {
			return fmt.Errorf("get %T: %w", configuration, err)
		}
		if configuration.GetAnnotations()["cert-manager.io/inject-ca-from"] != operatorNamespace+"/dspa-webhook-cert" {
			return fmt.Errorf("%T CA injection annotation does not reference the operator namespace certificate", configuration)
		}
		var clientConfigurations []admissionv1.WebhookClientConfig
		switch webhook := configuration.(type) {
		case *admissionv1.ValidatingWebhookConfiguration:
			for _, entry := range webhook.Webhooks {
				clientConfigurations = append(clientConfigurations, entry.ClientConfig)
			}
		case *admissionv1.MutatingWebhookConfiguration:
			for _, entry := range webhook.Webhooks {
				clientConfigurations = append(clientConfigurations, entry.ClientConfig)
			}
		}
		if len(clientConfigurations) == 0 {
			return fmt.Errorf("%T has no webhooks", configuration)
		}
		for _, webhook := range clientConfigurations {
			if webhook.Service == nil || webhook.Service.Namespace != operatorNamespace || webhook.Service.Name != "ds-pipelines-webhook" {
				return fmt.Errorf("%T points to the wrong webhook service", configuration)
			}
			if !bytes.Equal(webhook.CABundle, certificateAuthority) {
				return fmt.Errorf("%T CA bundle does not match the certificate Secret", configuration)
			}
		}
	}
	return nil
}
