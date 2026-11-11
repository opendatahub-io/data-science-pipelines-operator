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

package testUtil

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestCheckKindWebhookTLS verifies the runtime objects produced after kubelet
// expansion and cert-manager injection, including both webhook configurations.
func TestCheckKindWebhookTLS(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		mutate        func(*corev1.Secret, *admissionv1.ValidatingWebhookConfiguration, *admissionv1.MutatingWebhookConfiguration)
		expectedError string
	}{
		{name: "expanded annotation and injected CA"},
		{name: "unexpanded validating annotation", mutate: func(_ *corev1.Secret, validating *admissionv1.ValidatingWebhookConfiguration, _ *admissionv1.MutatingWebhookConfiguration) {
			validating.Annotations["cert-manager.io/inject-ca-from"] = "$(DSPO_NAMESPACE)/dspa-webhook-cert"
		}, expectedError: "annotation"},
		{name: "unexpanded mutating annotation", mutate: func(_ *corev1.Secret, _ *admissionv1.ValidatingWebhookConfiguration, mutating *admissionv1.MutatingWebhookConfiguration) {
			mutating.Annotations["cert-manager.io/inject-ca-from"] = "$(DSPO_NAMESPACE)/dspa-webhook-cert"
		}, expectedError: "annotation"},
		{name: "wrong certificate source", mutate: func(_ *corev1.Secret, validating *admissionv1.ValidatingWebhookConfiguration, _ *admissionv1.MutatingWebhookConfiguration) {
			validating.Annotations["cert-manager.io/inject-ca-from"] = "opendatahub/dspa-webhook-cert"
		}, expectedError: "annotation"},
		{name: "missing validating configuration", mutate: func(_ *corev1.Secret, validating *admissionv1.ValidatingWebhookConfiguration, _ *admissionv1.MutatingWebhookConfiguration) {
			validating.Name = "unrelated-configuration"
		}, expectedError: "not found"},
		{name: "missing mutating configuration", mutate: func(_ *corev1.Secret, _ *admissionv1.ValidatingWebhookConfiguration, mutating *admissionv1.MutatingWebhookConfiguration) {
			mutating.Name = "unrelated-configuration"
		}, expectedError: "not found"},
		{name: "wrong service name", mutate: func(_ *corev1.Secret, validating *admissionv1.ValidatingWebhookConfiguration, _ *admissionv1.MutatingWebhookConfiguration) {
			validating.Webhooks[0].ClientConfig.Service.Name = "unrelated-service"
		}, expectedError: "service"},
		{name: "later webhook has mismatched CA", mutate: func(_ *corev1.Secret, _ *admissionv1.ValidatingWebhookConfiguration, mutating *admissionv1.MutatingWebhookConfiguration) {
			additional := mutating.Webhooks[0].DeepCopy()
			additional.Name = "additional.example.com"
			additional.ClientConfig.CABundle = []byte("wrong-ca")
			mutating.Webhooks = append(mutating.Webhooks, *additional)
		}, expectedError: "CA bundle"},
		{name: "empty secret CA", mutate: func(secret *corev1.Secret, _ *admissionv1.ValidatingWebhookConfiguration, _ *admissionv1.MutatingWebhookConfiguration) {
			secret.Data = nil
		}, expectedError: "ca.crt"},
		{name: "validating CA not injected", mutate: func(_ *corev1.Secret, validating *admissionv1.ValidatingWebhookConfiguration, _ *admissionv1.MutatingWebhookConfiguration) {
			validating.Webhooks[0].ClientConfig.CABundle = nil
		}, expectedError: "CA bundle"},
		{name: "mutating CA does not match", mutate: func(_ *corev1.Secret, _ *admissionv1.ValidatingWebhookConfiguration, mutating *admissionv1.MutatingWebhookConfiguration) {
			mutating.Webhooks[0].ClientConfig.CABundle = []byte("wrong-ca")
		}, expectedError: "CA bundle"},
		{name: "wrong service namespace", mutate: func(_ *corev1.Secret, validating *admissionv1.ValidatingWebhookConfiguration, _ *admissionv1.MutatingWebhookConfiguration) {
			validating.Webhooks[0].ClientConfig.Service.Namespace = "opendatahub"
		}, expectedError: "service"},
		{name: "missing service", mutate: func(_ *corev1.Secret, _ *admissionv1.ValidatingWebhookConfiguration, mutating *admissionv1.MutatingWebhookConfiguration) {
			mutating.Webhooks[0].ClientConfig.Service = nil
		}, expectedError: "service"},
		{name: "no validating webhooks", mutate: func(_ *corev1.Secret, validating *admissionv1.ValidatingWebhookConfiguration, _ *admissionv1.MutatingWebhookConfiguration) {
			validating.Webhooks = nil
		}, expectedError: "no webhooks"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ds-pipelines-webhook-tls", Namespace: "rhods"}, Data: map[string][]byte{"ca.crt": []byte("test-ca")}}
			metadata := metav1.ObjectMeta{Name: "pipelineversions.pipelines.kubeflow.org", Annotations: map[string]string{"cert-manager.io/inject-ca-from": "rhods/dspa-webhook-cert"}}
			configuration := admissionv1.WebhookClientConfig{Service: &admissionv1.ServiceReference{Name: "ds-pipelines-webhook", Namespace: "rhods"}, CABundle: []byte("test-ca")}
			validating := &admissionv1.ValidatingWebhookConfiguration{ObjectMeta: *metadata.DeepCopy(), Webhooks: []admissionv1.ValidatingWebhook{{Name: "validating.example.com", ClientConfig: *configuration.DeepCopy()}}}
			mutating := &admissionv1.MutatingWebhookConfiguration{ObjectMeta: *metadata.DeepCopy(), Webhooks: []admissionv1.MutatingWebhook{{Name: "mutating.example.com", ClientConfig: *configuration.DeepCopy()}}}
			if scenario.mutate != nil {
				scenario.mutate(secret, validating, mutating)
			}
			kubernetesClient := fake.NewClientBuilder().WithObjects(secret, validating, mutating).Build()
			err := CheckKindWebhookTLS(context.Background(), kubernetesClient, "rhods")
			if scenario.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, scenario.expectedError)
			}
		})
	}
}

// TestCheckKindWebhookTLSAPIErrors preserves missing-resource and authorization
// failures rather than treating missing runtime objects as successful validation.
func TestCheckKindWebhookTLSAPIErrors(t *testing.T) {
	kubernetesClient := fake.NewClientBuilder().Build()
	require.True(t, apierrs.IsNotFound(CheckKindWebhookTLS(context.Background(), kubernetesClient, "rhods")))
	forbidden := apierrs.NewForbidden(schema.GroupResource{Resource: "secrets"}, "ds-pipelines-webhook-tls", errors.New("denied"))
	kubernetesClient = fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return forbidden
		},
	}).Build()
	require.ErrorIs(t, CheckKindWebhookTLS(context.Background(), kubernetesClient, "rhods"), forbidden)
}
