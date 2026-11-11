//go:build test_integration

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

package integration

import (
	"context"
	"flag"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestOperatorNamespaceFlag covers the default and explicit namespace override
// exposed to make integrationtest.
func TestOperatorNamespaceFlag(t *testing.T) {
	namespaceFlag := flag.Lookup("operatorNamespace")
	require.NotNil(t, namespaceFlag)
	require.Equal(t, "opendatahub", namespaceFlag.DefValue)
	previous := namespaceFlag.Value.String()
	t.Cleanup(func() { require.NoError(t, namespaceFlag.Value.Set(previous)) })
	require.NoError(t, namespaceFlag.Value.Set("rhods"))
	require.Equal(t, "rhods", OperatorNamespace)
}

// TestWebhookPodUsesOperatorNamespace exercises the suite with a decoy webhook
// in the default namespace so a hardcoded lookup fails without a polling delay.
func TestWebhookPodUsesOperatorNamespace(t *testing.T) {
	apiServer := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-server", Namespace: "test-dspa", Labels: map[string]string{"app": "ds-pipeline-test"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "ds-pipeline-api-server", Args: []string{"--pipelinesStoreKubernetes=true"}}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	webhook := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "webhook", Namespace: "rhods", Labels: map[string]string{"app": "ds-pipelines-webhook"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	decoy := webhook.DeepCopy()
	decoy.Namespace = "opendatahub"
	var queriedNamespaces []string
	kubernetesClient := fake.NewClientBuilder().WithObjects(apiServer, webhook, decoy).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, underlying client.WithWatch, list client.ObjectList, options ...client.ListOption) error {
			listOptions := &client.ListOptions{}
			listOptions.ApplyOptions(options)
			queriedNamespaces = append(queriedNamespaces, listOptions.Namespace)
			return underlying.List(ctx, list, options...)
		},
	}).Build()
	suite := &IntegrationTestSuite{
		Ctx: context.Background(), Clientmgr: ClientManager{k8sClient: kubernetesClient},
		DSPANamespace: "test-dspa", OperatorNamespace: "rhods",
		DSPA: &dspav1.DataSciencePipelinesApplication{ObjectMeta: metav1.ObjectMeta{Name: "test"}, Spec: dspav1.DSPASpec{APIServer: &dspav1.APIServer{PipelineStore: "kubernetes"}}},
	}
	suite.SetT(t)
	suite.TestDSPADeploymentWithK8sNativeApi()
	require.Contains(t, queriedNamespaces, "rhods")
	require.NotContains(t, queriedNamespaces, "opendatahub")
}
