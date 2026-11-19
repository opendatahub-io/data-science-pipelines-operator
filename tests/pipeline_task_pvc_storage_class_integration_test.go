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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	TestUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// taskPVCStorageClassName is a non-default StorageClass created by this
	// test from the cluster default's provisioner, then configured as the
	// DSPA pipeline-task PVC default.
	taskPVCStorageClassName = "dspa-task-pvc-sc"
	// explicitTaskPVCStorageClassName is a second non-default StorageClass,
	// distinct from the configured DSPA default, used by the pipeline's
	// explicit-class task so preservation can be distinguished from fallback.
	explicitTaskPVCStorageClassName = "dspa-task-pvc-sc-explicit"
	pipelineCreatePVCName           = "test-pipeline-createpvc"
	// createPVCOwnerAnnotation* is stamped by the CreatePVC pipeline's tasks
	// onto every PVC they create. The upload templates the value with a
	// run-unique suffix, so lookup and cleanup match only the current
	// run's claims, never another workload's or another run's.
	createPVCOwnerAnnotationKey   = "test-dspo-owner"
	createPVCOwnerAnnotationValue = "createpvc-storage-class"
	// maxHTTPResponseBytes bounds response reads from the DSP API server.
	maxHTTPResponseBytes = 1 << 20
)

// createStorageClassFromDefault creates a StorageClass with the given name
// reusing the cluster default's provisioner and parameters, so volumes can
// bind on any provider. The reclaim policy is forced to Delete even when the
// default uses Retain, so provisioned volumes release their backing storage
// when cleanup removes the test's claims. StorageClasses this call creates
// are cluster-scoped and outlive the suite's namespace, so they are
// registered for deletion in the test's cleanup; pre-existing ones
// (AlreadyExists) are left alone.
func (suite *IntegrationTestSuite) createStorageClassFromDefault(t *testing.T, name string) {
	scList := &storagev1.StorageClassList{}
	require.NoError(t, suite.Clientmgr.k8sClient.List(suite.Ctx, scList))

	var def *storagev1.StorageClass
	for i := range scList.Items {
		if scList.Items[i].Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			def = &scList.Items[i]
			break
		}
	}
	require.NotNil(t, def, "cluster must have a default StorageClass")

	reclaimDelete := corev1.PersistentVolumeReclaimDelete
	sc := &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: name},
		Provisioner:       def.Provisioner,
		Parameters:        def.Parameters,
		ReclaimPolicy:     &reclaimDelete,
		VolumeBindingMode: def.VolumeBindingMode,
	}
	err := suite.Clientmgr.k8sClient.Create(suite.Ctx, sc)
	if apierrors.IsAlreadyExists(err) {
		return
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := suite.Clientmgr.k8sClient.Delete(suite.Ctx, sc); err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("failed to delete StorageClass %s: %v", name, err)
		}
	})
}

// verifyTaskPVCDefaultStorageClassPropagated asserts that the DSPA field,
// which this suite's DSPA is deployed with, reached the API server
// deployment and its running pods as DEFAULT_PVC_STORAGE_CLASS set to
// expected. expected is the DSPA's configured value, whatever manifest
// supplied it, not a hardcoded class name.
func (suite *IntegrationTestSuite) verifyTaskPVCDefaultStorageClassPropagated(t *testing.T, expected string) {
	deploymentName := fmt.Sprintf("ds-pipeline-%s", suite.DSPA.Name)
	require.Eventually(t, func() bool {
		deployment := &appsv1.Deployment{}
		err := suite.Clientmgr.k8sClient.Get(suite.Ctx,
			types.NamespacedName{Name: deploymentName, Namespace: suite.DSPANamespace}, deployment)
		if err != nil {
			return false
		}
		for i := range deployment.Spec.Template.Spec.Containers {
			c := &deployment.Spec.Template.Spec.Containers[i]
			if c.Name != "ds-pipeline-api-server" {
				continue
			}
			for _, env := range c.Env {
				if env.Name == "DEFAULT_PVC_STORAGE_CLASS" && env.Value == expected {
					return true
				}
			}
		}
		return false
	}, DeployTimeout, PollInterval, "DEFAULT_PVC_STORAGE_CLASS should reach the API server deployment")

	// The run must be served by a pod that has the env var, so require every
	// Running API server pod to carry it.
	require.Eventually(t, func() bool {
		podList := &corev1.PodList{}
		err := suite.Clientmgr.k8sClient.List(suite.Ctx, podList,
			client.InNamespace(suite.DSPANamespace),
			client.MatchingLabels{"app": deploymentName})
		if err != nil {
			return false
		}
		if len(podList.Items) == 0 {
			return false
		}
		for i := range podList.Items {
			if podList.Items[i].Status.Phase != corev1.PodRunning {
				return false
			}
			if !podHasEnv(&podList.Items[i], "DEFAULT_PVC_STORAGE_CLASS", expected) {
				return false
			}
		}
		return true
	}, DeployTimeout, PollInterval, "running API server pods should carry DEFAULT_PVC_STORAGE_CLASS")
}

// podHasEnv reports whether the ds-pipeline-api-server container of the pod
// has the named env var set to value.
func podHasEnv(pod *corev1.Pod, name, value string) bool {
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		if c.Name != "ds-pipeline-api-server" {
			continue
		}
		for _, env := range c.Env {
			if env.Name == name && env.Value == value {
				return true
			}
		}
	}
	return false
}

// getRunPVC returns the storageClassName pointer of the current run's PVC
// whose name ends with suffix, retrying until the driver task has created it.
// Claims are matched by the run-unique owner annotation, so claims of other
// workloads or runs are never picked up. The pointer distinguishes an unset
// class (nil) from an explicit empty one ("").
func (suite *IntegrationTestSuite) getRunPVC(t *testing.T, suffix, ownerValue string) *string {
	var storageClass *string
	require.Eventually(t, func() bool {
		pvcList := &corev1.PersistentVolumeClaimList{}
		err := suite.Clientmgr.k8sClient.List(suite.Ctx, pvcList,
			client.InNamespace(suite.DSPANamespace))
		if err != nil {
			return false
		}
		for i := range pvcList.Items {
			pvc := pvcList.Items[i]
			if pvc.Annotations[createPVCOwnerAnnotationKey] != ownerValue {
				continue
			}
			if strings.HasSuffix(pvc.Name, suffix) {
				storageClass = pvc.Spec.StorageClassName
				return true
			}
		}
		return false
	}, DeployTimeout, PollInterval, "PVC with suffix %s should be created", suffix)
	return storageClass
}

// uploadCreatePVCPipeline uploads the compiled CreatePVC pipeline with a
// run-unique owner annotation stamped on the tasks, and returns the pipeline
// ID and the annotation value identifying this run's claims. A run-unique
// pipeline name avoids a 409 on the upload when the suite reuses an existing
// DSPA (skipCleanup) that already holds this pipeline.
func (suite *IntegrationTestSuite) uploadCreatePVCPipeline(t *testing.T) (pipelineID, ownerValue string) {
	unique := time.Now().UnixNano()
	pipelineName := fmt.Sprintf("%s-%d", pipelineCreatePVCName, unique)
	ownerValue = fmt.Sprintf("%s-%d", createPVCOwnerAnnotationValue, unique)

	// Template the manifest so each run's claims carry a unique owner value.
	manifest, err := os.ReadFile("resources/test-pipeline-createpvc.yaml")
	require.NoError(t, err)
	manifestPath := filepath.Join(t.TempDir(), "test-pipeline-createpvc.yaml")
	require.NoError(t, os.WriteFile(manifestPath,
		[]byte(strings.ReplaceAll(string(manifest), createPVCOwnerAnnotationValue, ownerValue)), 0o644))

	postUrl := fmt.Sprintf("%s/apis/v2beta1/pipelines/upload?name=%s", APIServerURL, url.QueryEscape(pipelineName))
	vals := map[string]string{
		"uploadfile": "@" + manifestPath,
	}
	body, contentType := TestUtil.FormFromFile(t, vals)

	response, err := suite.Clientmgr.httpClient.Post(postUrl, contentType, body)
	require.NoError(t, err)
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, maxHTTPResponseBytes))
	require.NoError(t, err)
	loggr.Info(fmt.Sprintf("pipeline %s upload response status: %d", pipelineName, response.StatusCode))
	require.Equal(t, 200, response.StatusCode)

	pipelineID, err = TestUtil.RetrievePipelineId(t, suite.Clientmgr.httpClient, APIServerURL, pipelineName)
	require.NoError(t, err)
	return pipelineID, ownerValue
}

// runCreatePVCPipeline creates a run of the CreatePVC pipeline and waits for
// that specific run's completion. The shared WaitForPipelineRunCompletion
// helper observes the state of an arbitrary run, which can pass or fail on a
// stale run in a reused namespace, so this wait polls the run by its ID via
// GET /apis/v2beta1/runs/{run_id} rather than scanning the paginated runs
// list, where the run could fall outside the first page.
func (suite *IntegrationTestSuite) runCreatePVCPipeline(t *testing.T, pipelineID string) {
	postUrl := fmt.Sprintf("%s/apis/v2beta1/runs", APIServerURL)
	body := TestUtil.FormatRequestBody(t, pipelineID, pipelineCreatePVCName)
	response, err := suite.Clientmgr.httpClient.Post(postUrl, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer response.Body.Close()
	responseData, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPResponseBytes))
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	runID := TestUtil.RetrieveRunID(t, responseData)
	loggr.Info(fmt.Sprintf("created CreatePVC pipeline run %s", runID))

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		runResponse, err := suite.Clientmgr.httpClient.Get(fmt.Sprintf("%s/apis/v2beta1/runs/%s", APIServerURL, runID))
		if !assert.NoError(ct, err) {
			return
		}
		defer runResponse.Body.Close()
		runData, err := io.ReadAll(io.LimitReader(runResponse.Body, maxHTTPResponseBytes))
		if !assert.NoError(ct, err) {
			return
		}
		if !assert.Equal(ct, 200, runResponse.StatusCode) {
			return
		}
		var run struct {
			State string `json:"state"`
		}
		if !assert.NoError(ct, json.Unmarshal(runData, &run)) {
			return
		}
		switch run.State {
		case "SUCCEEDED":
			return
		case "SKIPPED", "FAILED", "CANCELING", "CANCELED", "PAUSED":
			ct.Errorf("pipeline run %s status: %s", runID, run.State)
			return
		}
		ct.Errorf("run %s has no terminal state yet: %q", runID, run.State)
	}, 6*time.Minute, 6*time.Second, "run %s should complete", runID)
}

// cleanupRunPVCs registers deletion of every PVC the current run created,
// matched by the run-unique owner annotation, so claims of other workloads
// or runs are never touched. It is registered before the run starts so the
// claims are deleted even when the run fails or an assertion aborts the
// subtest early; the suite deletes the DSPA but not the namespace, so they
// would otherwise outlive the suite.
func (suite *IntegrationTestSuite) cleanupRunPVCs(t *testing.T, ownerValue string) {
	t.Cleanup(func() {
		pvcList := &corev1.PersistentVolumeClaimList{}
		err := suite.Clientmgr.k8sClient.List(suite.Ctx, pvcList,
			client.InNamespace(suite.DSPANamespace))
		if err != nil {
			t.Errorf("failed to list PVCs for cleanup: %v", err)
			return
		}
		for i := range pvcList.Items {
			pvc := pvcList.Items[i]
			if pvc.Annotations[createPVCOwnerAnnotationKey] != ownerValue {
				continue
			}
			if err := suite.Clientmgr.k8sClient.Delete(suite.Ctx, &pvc); err != nil && !apierrors.IsNotFound(err) {
				t.Errorf("failed to delete PVC %s: %v", pvc.Name, err)
			}
		}
	})
}

// clusterDefaultStorageClass returns the name of the cluster's default
// StorageClass. When several classes carry the is-default-class annotation,
// Kubernetes picks the newest, tie-broken by ascending name
// (k8s.io/kubernetes/pkg/volume/util GetDefaultClass), so the same rule is
// applied here.
func (suite *IntegrationTestSuite) clusterDefaultStorageClass(t *testing.T) string {
	scList := &storagev1.StorageClassList{}
	require.NoError(t, suite.Clientmgr.k8sClient.List(suite.Ctx, scList))
	var defaults []*storagev1.StorageClass
	for i := range scList.Items {
		if scList.Items[i].Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaults = append(defaults, &scList.Items[i])
		}
	}
	if len(defaults) == 0 {
		t.Fatal("cluster must have a default StorageClass")
	}
	sort.Slice(defaults, func(i, j int) bool {
		if !defaults[i].CreationTimestamp.Equal(&defaults[j].CreationTimestamp) {
			return defaults[i].CreationTimestamp.After(defaults[j].CreationTimestamp.Time)
		}
		return defaults[i].Name < defaults[j].Name
	})
	return defaults[0].Name
}

// TestPipelineTaskPVCDefaultStorageClass verifies the RHOAIENG-101484
// end-to-end flow: a configured DSPA default StorageClass reaches the API
// server as DEFAULT_PVC_STORAGE_CLASS and the backend applies the precedence
// pipeline-supplied class (including explicit "") > DSPA default > Kubernetes
// default. Suites whose DSPA is deployed without the field (see
// tests/resources/dspa-*.yaml) exercise the unset leg instead: an omitted
// class falls back to the Kubernetes default. Mutating a running DSPA is not
// an option because the resulting API server rollout breaks the suite's
// pod-level port-forward.
func (suite *IntegrationTestSuite) TestPipelineTaskPVCDefaultStorageClass() {
	// Read the live DSPA rather than the fixture: with -skipDeploy the
	// fixture need not match the deployed object, and the expected classes
	// below must follow what is actually deployed.
	live := &dspav1.DataSciencePipelinesApplication{}
	require.NoError(suite.T(), suite.Clientmgr.k8sClient.Get(suite.Ctx,
		types.NamespacedName{Name: suite.DSPA.Name, Namespace: suite.DSPANamespace}, live))
	// The pipeline is uploaded through the API, which the Kubernetes
	// pipeline store does not serve runs for (see TestPipelineSuccessfulRun).
	// Check the live object: a database fixture can be reused against a live
	// DSPA that actually uses the Kubernetes store.
	if live.Spec.APIServer.PipelineStore == "kubernetes" {
		suite.T().Log("PipelineStore is set to kubernetes, skipping CreatePVC default verification")
		suite.T().SkipNow()
	}
	configuredDefault := live.Spec.APIServer.PipelineTaskPVCDefaultStorageClass
	configured := configuredDefault != ""

	suite.T().Run("should apply DSPA default and task precedence to CreatePVC volumes", func(t *testing.T) {
		suite.createStorageClassFromDefault(t, taskPVCStorageClassName)
		suite.createStorageClassFromDefault(t, explicitTaskPVCStorageClassName)
		if configured {
			suite.verifyTaskPVCDefaultStorageClassPropagated(t, configuredDefault)
		}

		pipelineID, ownerValue := suite.uploadCreatePVCPipeline(t)
		suite.cleanupRunPVCs(t, ownerValue)
		suite.runCreatePVCPipeline(t, pipelineID)

		// Omitted class: with a configured DSPA default it must be selected
		// over the Kubernetes default; without one the Kubernetes default
		// must apply. Either way admission resolves it to a concrete class,
		// so the pointer must be set.
		expectedOmitted := suite.clusterDefaultStorageClass(t)
		if configured {
			expectedOmitted = configuredDefault
		}
		omitted := suite.getRunPVC(t, "-sc-omitted", ownerValue)
		require.NotNil(t, omitted)
		assert.Equal(t, expectedOmitted, *omitted,
			"omitted storage class must use the DSPA-configured default when set, else the Kubernetes default")

		// Explicit named class: must be preserved and distinct from the
		// configured default, proving the backend does not override it.
		explicit := suite.getRunPVC(t, "-sc-explicit", ownerValue)
		require.NotNil(t, explicit)
		assert.Equal(t, explicitTaskPVCStorageClassName, *explicit,
			"explicit storage class must be preserved")

		// Explicit empty class: opt-out of defaulting must be preserved as
		// an explicit empty value, not an unset one.
		empty := suite.getRunPVC(t, "-sc-empty", ownerValue)
		require.NotNil(t, empty, "explicit empty storage class must be set, not unset")
		assert.Equal(t, "", *empty,
			"explicit empty storage class must opt out of defaulting")
	})
}
