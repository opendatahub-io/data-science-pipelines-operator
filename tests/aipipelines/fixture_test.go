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

package aipipelines_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	argoassets "github.com/opendatahub-io/data-science-pipelines-operator/config/argo"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	fixtureLabel = "testing.opendatahub.io/aipipelines"
	operatorName = "data-science-pipelines-operator-controller-manager"
	configName   = "odh-aipipelines-config"
	pollInterval = 2 * time.Second
	deadline     = 8 * time.Minute
)

type fixture struct {
	t            *testing.T
	ctx          context.Context
	client       client.Client
	restConfig   *rest.Config
	applications string
	namespace    string
	dspa         *dspav1.DataSciencePipelinesApplication
}

// The singleton and its handshake must be explicitly marked as test fixtures.
// These tests change module-wide settings and must run on a dedicated cluster.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	require.NotEmpty(t, kubeconfig, "set KUBECONFIG to a dedicated test cluster's config file")
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	restConfig.Timeout = 30 * time.Second
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	require.NoError(t, dspav1.AddToScheme(scheme))
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	require.NoError(t, err)
	applications := os.Getenv("APPLICATIONS_NAMESPACE")
	if applications == "" {
		applications = "opendatahub"
	}
	f := &fixture{t: t, ctx: context.Background(), client: k8sClient, restConfig: restConfig, applications: applications}
	module := f.module()
	require.Equal(t, "true", module.Labels[fixtureLabel], "the module must be a dedicated test fixture")
	handshake := &corev1.ConfigMap{}
	f.get(client.ObjectKey{Name: configName, Namespace: applications}, handshake)
	require.Equal(t, "true", handshake.Labels[fixtureLabel])
	require.Empty(t, module.OwnerReferences, "run directly against DSPO, without a platform controller overwriting the fixture")
	f.waitModule(common.Managed, handshake.Data["platformVersion"])

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "aipipelines-e2e-", Labels: map[string]string{fixtureLabel: "true"}}}
	require.NoError(t, f.client.Create(f.ctx, namespace))
	f.namespace = namespace.Name
	t.Cleanup(func() { assert.NoError(t, client.IgnoreNotFound(f.client.Delete(f.ctx, namespace))) })
	// Restore only the fixture fields this suite changes, even after a failure.
	t.Cleanup(func() {
		assert.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			current := &corev1.ConfigMap{}
			if err := f.client.Get(f.ctx, client.ObjectKeyFromObject(handshake), current); err != nil {
				return err
			}
			current.Data = handshake.Data
			return f.client.Update(f.ctx, current)
		}))
		assert.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			current := &aipipelinesv1alpha1.AIPipelines{}
			err := f.client.Get(f.ctx, client.ObjectKeyFromObject(module), current)
			if apierrors.IsNotFound(err) {
				return f.client.Create(f.ctx, &aipipelinesv1alpha1.AIPipelines{
					ObjectMeta: metav1.ObjectMeta{Name: module.Name, Labels: module.Labels}, Spec: module.Spec,
				})
			}
			if err != nil {
				return err
			}
			current.Spec = module.Spec
			return f.client.Update(f.ctx, current)
		}))
	})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		current := &aipipelinesv1alpha1.AIPipelines{}
		if err := f.client.Get(f.ctx, client.ObjectKeyFromObject(module), current); err == nil {
			t.Logf("module status: %+v", current.Status)
		}
		pods := &corev1.PodList{}
		if err := f.client.List(f.ctx, pods, client.InNamespace(f.namespace)); err == nil {
			for _, pod := range pods.Items {
				t.Logf("pod %s status: %+v", pod.Name, pod.Status)
			}
		}
		events := &corev1.EventList{}
		if err := f.client.List(f.ctx, events, client.InNamespace(f.namespace)); err == nil {
			for _, event := range events.Items {
				t.Logf("%s/%s %s: %s", event.InvolvedObject.Kind, event.InvolvedObject.Name, event.Reason, event.Message)
			}
		}
	})
	return f
}

func (f *fixture) get(key client.ObjectKey, object client.Object) {
	f.t.Helper()
	require.NoError(f.t, f.client.Get(f.ctx, key, object))
}

func (f *fixture) module() *aipipelinesv1alpha1.AIPipelines {
	f.t.Helper()
	module := &aipipelinesv1alpha1.AIPipelines{}
	f.get(client.ObjectKey{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}, module)
	return module
}

func (f *fixture) waitModule(state common.ManagementState, version string) {
	f.t.Helper()
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		module := &aipipelinesv1alpha1.AIPipelines{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKey{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}, module)) {
			return
		}
		var contract common.PlatformObject = module
		assert.Equal(c, state, module.Spec.ArgoWorkflowsControllersManagementState())
		assert.Equal(c, module.Generation, contract.GetStatus().ObservedGeneration)
		assert.Equal(c, common.PhaseReady, contract.GetStatus().Phase)
		assert.Equal(c, version, contract.GetReleaseStatus().GetPlatformRelease())
		for _, kind := range []string{"Ready", "ProvisioningSucceeded", "DSPOReady", "ArgoWorkflowsControllersReady"} {
			found := false
			for _, condition := range contract.GetConditions() {
				if condition.Type == kind {
					found = true
					assert.Equal(c, metav1.ConditionTrue, condition.Status, "%s: %s", kind, condition.Message)
					assert.Equal(c, module.Generation, condition.ObservedGeneration)
				}
			}
			assert.True(c, found, "missing %s", kind)
		}
	}, deadline, pollInterval)
}

func (f *fixture) setState(state common.ManagementState) {
	f.t.Helper()
	require.NoError(f.t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		module := &aipipelinesv1alpha1.AIPipelines{}
		if err := f.client.Get(f.ctx, client.ObjectKey{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}, module); err != nil {
			return err
		}
		module.Spec.ArgoWorkflowsControllers = &aipipelinesv1alpha1.ArgoWorkflowsControllersSpec{ManagementState: state}
		return f.client.Update(f.ctx, module)
	}))
}

func (f *fixture) waitModuleCondition(kind string, status metav1.ConditionStatus, release string) {
	f.t.Helper()
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		module := &aipipelinesv1alpha1.AIPipelines{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKey{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}, module)) {
			return
		}
		assert.Equal(c, module.Generation, module.Status.ObservedGeneration)
		assert.Equal(c, release, module.Status.GetPlatformRelease())
		found := false
		for _, condition := range module.Status.Conditions {
			if condition.Type == kind {
				found = true
				assert.Equal(c, status, condition.Status, condition.Message)
				assert.Equal(c, module.Generation, condition.ObservedGeneration)
			}
		}
		assert.True(c, found, "missing %s", kind)
	}, deadline, pollInterval)
}

func (f *fixture) setVersion(version string) {
	f.t.Helper()
	require.NoError(f.t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		cm := &corev1.ConfigMap{}
		if err := f.client.Get(f.ctx, client.ObjectKey{Name: configName, Namespace: f.applications}, cm); err != nil {
			return err
		}
		cm.Data["platformVersion"] = version
		return f.client.Update(f.ctx, cm)
	}))
}

func (f *fixture) deployDSPA() {
	f.t.Helper()
	data, err := os.ReadFile("../resources/dspa-lite.yaml")
	require.NoError(f.t, err)
	f.dspa = &dspav1.DataSciencePipelinesApplication{}
	require.NoError(f.t, yaml.Unmarshal(data, f.dspa))
	f.dspa.Namespace = f.namespace
	// This fixture does not use the custom PyPI server from the user-flow suite.
	f.dspa.Spec.APIServer.CABundle = nil
	require.NoError(f.t, f.client.Create(f.ctx, f.dspa))
	f.waitOperands()
}

func (f *fixture) waitDeployment(namespace, name string) {
	f.t.Helper()
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		deployment := &appsv1.Deployment{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKey{Name: name, Namespace: namespace}, deployment)) {
			return
		}
		assert.GreaterOrEqual(c, deployment.Status.ObservedGeneration, deployment.Generation)
		assert.NotNil(c, deployment.Spec.Replicas)
		if deployment.Spec.Replicas == nil {
			return
		}
		assert.Positive(c, *deployment.Spec.Replicas)
		assert.Equal(c, *deployment.Spec.Replicas, deployment.Status.UpdatedReplicas)
		assert.Equal(c, *deployment.Spec.Replicas, deployment.Status.AvailableReplicas)
		assert.Equal(c, *deployment.Spec.Replicas, deployment.Status.Replicas)
		available := false
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
				available = true
			}
		}
		assert.True(c, available, "%s is unavailable: %+v", name, deployment.Status)
	}, deadline, pollInterval)
}

func (f *fixture) waitOperands() {
	f.t.Helper()
	for _, prefix := range []string{"ds-pipeline-", "ds-pipeline-persistenceagent-", "ds-pipeline-scheduledworkflow-", "ds-pipeline-workflow-controller-", "mariadb-", "minio-", "ds-pipeline-metadata-grpc-", "ds-pipeline-metadata-envoy-"} {
		f.waitDeployment(f.namespace, prefix+f.dspa.Name)
	}
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		current := &dspav1.DataSciencePipelinesApplication{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKeyFromObject(f.dspa), current)) {
			return
		}
		found := false
		for _, condition := range current.Status.Conditions {
			if condition.Type == "Ready" {
				found = true
				assert.Equal(c, metav1.ConditionTrue, condition.Status, condition.Message)
			}
		}
		assert.True(c, found)
	}, deadline, pollInterval)
}

func (f *fixture) forward(service string, port int) (string, func()) {
	f.t.Helper()
	svc := &corev1.Service{}
	f.get(client.ObjectKey{Name: service, Namespace: f.namespace}, svc)
	require.NotEmpty(f.t, svc.Spec.Selector)
	pods := &corev1.PodList{}
	require.NoError(f.t, f.client.List(f.ctx, pods, client.InNamespace(f.namespace), client.MatchingLabels(svc.Spec.Selector)))
	podName := ""
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				podName = pod.Name
				break
			}
		}
		if podName != "" {
			break
		}
	}
	require.NotEmpty(f.t, podName, "no ready pod for Service %s", service)
	clientset, err := kubernetes.NewForConfig(f.restConfig)
	require.NoError(f.t, err)
	endpoint := clientset.CoreV1().RESTClient().Post().Namespace(f.namespace).Resource("pods").Name(podName).SubResource("portforward").URL()
	transport, upgrader, err := spdy.RoundTripperFor(f.restConfig)
	require.NoError(f.t, err)
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, endpoint)
	stop, ready := make(chan struct{}), make(chan struct{})
	forward, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{fmt.Sprintf("0:%d", port)}, stop, ready, io.Discard, io.Discard)
	require.NoError(f.t, err)
	done := make(chan error, 1)
	go func() { done <- forward.ForwardPorts(); close(done) }()
	select {
	case <-ready:
	case err := <-done:
		close(stop)
		f.t.Fatalf("port forward to %s failed: %v", service, err)
	case <-time.After(30 * time.Second):
		close(stop)
		f.t.Fatalf("port forward to %s did not become ready", service)
	}
	ports, err := forward.GetPorts()
	if err != nil {
		close(stop)
		f.t.Fatal(err)
	}
	require.Len(f.t, ports, 1)
	return fmt.Sprintf("http://127.0.0.1:%d", ports[0].Local), func() {
		close(stop)
		select {
		case err := <-done:
			assert.NoError(f.t, err)
		case <-time.After(10 * time.Second):
			f.t.Error("port forward did not stop")
		}
	}
}

func (f *fixture) apiRequest(method, path string, body io.Reader) []byte {
	f.t.Helper()
	endpoint, closeForward := f.forward("ds-pipeline-"+f.dspa.Name, 8888)
	defer closeForward()
	request, err := http.NewRequestWithContext(f.ctx, method, endpoint+"/apis/v2beta1"+path, body)
	require.NoError(f.t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	require.NoError(f.t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(f.t, err)
	require.Equal(f.t, http.StatusOK, response.StatusCode, "%s %s: %s", method, path, data)
	return data
}

func (f *fixture) assertSharedAssets(removed bool) {
	f.t.Helper()
	assets, err := argoassets.Objects(f.applications)
	require.NoError(f.t, err)
	module := &aipipelinesv1alpha1.AIPipelines{}
	if !removed {
		module = f.module()
	}
	for _, asset := range assets {
		current := asset.DeepCopy()
		err := f.client.Get(f.ctx, client.ObjectKeyFromObject(asset), current)
		if removed && asset.GetKind() != "CustomResourceDefinition" {
			require.True(f.t, apierrors.IsNotFound(err), "%s/%s: %v", asset.GetKind(), asset.GetName(), err)
			continue
		}
		require.NoError(f.t, err)
		if asset.GetKind() == "CustomResourceDefinition" {
			require.Empty(f.t, current.GetOwnerReferences())
		} else {
			require.True(f.t, metav1.IsControlledBy(current, module))
		}
	}
}

func (f *fixture) waitSampleVersion(version string) {
	f.t.Helper()
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		cm := &corev1.ConfigMap{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKey{Name: "sample-config-" + f.dspa.Name, Namespace: f.namespace}, cm)) {
			return
		}
		var samples struct {
			Pipelines []struct {
				VersionName string `json:"versionName"`
			} `json:"pipelines"`
		}
		if !assert.NoError(c, json.Unmarshal([]byte(cm.Data["sample_config.json"]), &samples)) {
			return
		}
		if assert.NotEmpty(c, samples.Pipelines) {
			name := samples.Pipelines[0].VersionName
			assert.True(c, name == version || strings.HasSuffix(name, " - "+version), "sample version %q does not report %q", name, version)
		}
	}, deadline, pollInterval)
}

func workflow(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1", "kind": "Workflow",
		"metadata": map[string]interface{}{"name": name, "namespace": namespace},
		"spec": map[string]interface{}{
			"entrypoint": "main", "serviceAccountName": "pipeline-runner-test-dspa",
			"podMetadata": map[string]interface{}{"labels": map[string]interface{}{"pipelines.kubeflow.org/v2_component": "true"}},
			"templates": []interface{}{map[string]interface{}{
				"name": "main", "container": map[string]interface{}{
					"image":   "quay.io/opendatahub/ds-pipelines-ci-executor-image:v1.1",
					"command": []interface{}{"python", "-c"},
					"args":    []interface{}{fmt.Sprintf("import urllib.request; urls = ['http://ds-pipeline-test-dspa.%s.svc:8888/apis/v2beta1/healthz', 'http://minio-service.%s.svc:9000/minio/health/live']; [urllib.request.urlopen(url, timeout=30).read() for url in urls]; print('aipipelines-e2e services reachable')", namespace, namespace)},
				},
			}},
		},
	}}
}

func (f *fixture) runWorkflow(name string) *unstructured.Unstructured {
	f.t.Helper()
	run := workflow(f.namespace, name)
	require.NoError(f.t, f.client.Create(f.ctx, run))
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKeyFromObject(run), run)) {
			return
		}
		phase, _, err := unstructured.NestedString(run.Object, "status", "phase")
		assert.NoError(c, err)
		assert.Equal(c, "Succeeded", phase, "workflow status: %v", run.Object["status"])
	}, deadline, pollInterval)
	return run
}
