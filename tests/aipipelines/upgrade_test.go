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
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Run explicitly with make aipipelines-upgrade-test. Missing version/image
// inputs are errors, never a green skipped upgrade test.
func TestAIPipelinesUpgradeDowngrade(t *testing.T) {
	baselineImage := os.Getenv("AIPIPELINES_BASELINE_IMAGE")
	baselineVersion := os.Getenv("AIPIPELINES_BASELINE_VERSION")
	candidateVersion := os.Getenv("AIPIPELINES_CANDIDATE_VERSION")
	baselineRelatedImagesJSON := os.Getenv("AIPIPELINES_BASELINE_RELATED_IMAGES")
	require.NotEmpty(t, baselineImage)
	require.NotEmpty(t, baselineRelatedImagesJSON)
	baselineRelatedImages := map[string]string{}
	require.NoError(t, json.Unmarshal([]byte(baselineRelatedImagesJSON), &baselineRelatedImages), "AIPIPELINES_BASELINE_RELATED_IMAGES must be a JSON object")
	require.NotEmpty(t, baselineRelatedImages)
	for name, image := range baselineRelatedImages {
		require.True(t, strings.HasPrefix(name, "RELATED_IMAGE_"), "unsupported baseline image variable %q", name)
		require.NotEmpty(t, image, "baseline image %s is empty", name)
	}
	oldVersion, err := version.ParseSemantic(baselineVersion)
	require.NoError(t, err, "set AIPIPELINES_BASELINE_VERSION")
	newVersion, err := version.ParseSemantic(candidateVersion)
	require.NoError(t, err, "set AIPIPELINES_CANDIDATE_VERSION")
	require.True(t, oldVersion.LessThan(newVersion), "candidate must be newer than baseline")
	f := newFixture(t)
	operatorKey := client.ObjectKey{Name: operatorName, Namespace: f.applications}
	original := &appsv1.Deployment{}
	f.get(operatorKey, original)
	candidateImage := ""
	for _, container := range original.Spec.Template.Spec.Containers {
		if container.Name == "manager" {
			candidateImage = container.Image
		}
	}
	require.NotEmpty(t, candidateImage)
	require.NotEqual(t, baselineImage, candidateImage, "upgrade must roll between distinct operator images")
	candidateRelatedImages := make(map[string]string, len(baselineRelatedImages))
	for _, container := range original.Spec.Template.Spec.Containers {
		if container.Name != "manager" {
			continue
		}
		for _, env := range container.Env {
			if _, configured := baselineRelatedImages[env.Name]; configured {
				require.Empty(t, env.ValueFrom, "candidate related image %s must be a literal value", env.Name)
				candidateRelatedImages[env.Name] = env.Value
			}
		}
	}
	for name, baseline := range baselineRelatedImages {
		candidate, found := candidateRelatedImages[name]
		require.True(t, found, "candidate operator Deployment has no %s", name)
		require.NotEmpty(t, candidate, "candidate image %s is empty", name)
		require.NotEqual(t, baseline, candidate, "baseline and candidate %s must differ", name)
	}
	t.Cleanup(func() {
		if !assert.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			current := &appsv1.Deployment{}
			if err := f.client.Get(f.ctx, operatorKey, current); err != nil {
				return err
			}
			current.Spec.Template = *original.Spec.Template.DeepCopy()
			return f.client.Update(f.ctx, current)
		})) {
			return
		}
		f.waitDeployment(f.applications, operatorName)
	})
	roll := func(image, release string, relatedImages map[string]string) {
		t.Helper()
		f.setVersion(release)
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			current := &appsv1.Deployment{}
			if err := f.client.Get(f.ctx, operatorKey, current); err != nil {
				return err
			}
			for i := range current.Spec.Template.Spec.Containers {
				container := &current.Spec.Template.Spec.Containers[i]
				if container.Name != "manager" {
					continue
				}
				container.Image = image
				for name, relatedImage := range relatedImages {
					found := false
					for j := range container.Env {
						if container.Env[j].Name == name {
							container.Env[j] = corev1.EnvVar{Name: name, Value: relatedImage}
							found = true
						}
					}
					require.True(t, found, "manager container has no %s", name)
				}
				// Early modular releases read this fallback. Current releases must
				// also consume the live handshake (tested separately below).
				found := false
				for j := range container.Env {
					if container.Env[j].Name == "DSPO_PLATFORMVERSION" {
						container.Env[j] = corev1.EnvVar{Name: "DSPO_PLATFORMVERSION", Value: release}
						found = true
					}
				}
				if !found {
					container.Env = append(container.Env, corev1.EnvVar{Name: "DSPO_PLATFORMVERSION", Value: release})
				}
			}
			return f.client.Update(f.ctx, current)
		}))
		f.waitDeployment(f.applications, operatorName)
		f.waitModule(common.Managed, release)
	}

	t.Log("install baseline and write database, object-store, and workflow sentinels")
	roll(baselineImage, baselineVersion, baselineRelatedImages)
	f.deployDSPA()
	baselineOperandImages := f.operandImages()
	data := f.apiRequest("POST", "/experiments", strings.NewReader(`{"display_name":"module-upgrade-sentinel","description":"must survive upgrade and downgrade"}`))
	var experiment struct {
		ID   string `json:"experiment_id"`
		Name string `json:"display_name"`
	}
	require.NoError(t, json.Unmarshal(data, &experiment))
	require.NotEmpty(t, experiment.ID)
	run := f.runWorkflow("persisted-run")
	runUID := run.GetUID()
	pvcs := &corev1.PersistentVolumeClaimList{}
	require.NoError(t, f.client.List(f.ctx, pvcs, client.InNamespace(f.namespace)))
	require.GreaterOrEqual(t, len(pvcs.Items), 2)
	pvcUIDs := map[string]types.UID{}
	for _, pvc := range pvcs.Items {
		pvcUIDs[pvc.Name] = pvc.UID
	}
	secret := &corev1.Secret{}
	f.get(client.ObjectKey{Name: "ds-pipeline-s3-" + f.dspa.Name, Namespace: f.namespace}, secret)
	secretUID := secret.UID
	payload := []byte("aipipelines persistent object store sentinel\n")
	withStore := func(action func(*minio.Client)) {
		t.Helper()
		endpoint, closeForward := f.forward("minio-service", 9000)
		defer closeForward()
		store, err := minio.New(strings.TrimPrefix(endpoint, "http://"), &minio.Options{
			Creds:  credentials.NewStaticV4(string(secret.Data["accesskey"]), string(secret.Data["secretkey"]), ""),
			Secure: false,
		})
		require.NoError(t, err)
		action(store)
	}
	withStore(func(store *minio.Client) {
		require.NoError(t, store.MakeBucket(f.ctx, "module-upgrade-sentinel", minio.MakeBucketOptions{}))
		_, err := store.PutObject(f.ctx, "module-upgrade-sentinel", "data", bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{})
		require.NoError(t, err)
	})
	assertData := func() {
		t.Helper()
		f.waitOperands()
		var currentExperiment struct {
			ID   string `json:"experiment_id"`
			Name string `json:"display_name"`
		}
		require.NoError(t, json.Unmarshal(f.apiRequest("GET", "/experiments/"+experiment.ID, nil), &currentExperiment))
		require.Equal(t, experiment, currentExperiment)
		f.get(client.ObjectKeyFromObject(run), run)
		require.Equal(t, runUID, run.GetUID())
		for name, uid := range pvcUIDs {
			pvc := &corev1.PersistentVolumeClaim{}
			f.get(client.ObjectKey{Name: name, Namespace: f.namespace}, pvc)
			require.Equal(t, uid, pvc.UID, "PVC %s was replaced", name)
		}
		currentSecret := &corev1.Secret{}
		f.get(client.ObjectKeyFromObject(secret), currentSecret)
		require.Equal(t, secretUID, currentSecret.UID)
		require.True(t, bytes.Equal(secret.Data["accesskey"], currentSecret.Data["accesskey"]))
		require.True(t, bytes.Equal(secret.Data["secretkey"], currentSecret.Data["secretkey"]))
		withStore(func(store *minio.Client) {
			object, err := store.GetObject(f.ctx, "module-upgrade-sentinel", "data", minio.GetObjectOptions{})
			require.NoError(t, err)
			defer object.Close()
			actual, err := io.ReadAll(object)
			require.NoError(t, err)
			require.Equal(t, payload, actual)
		})
	}
	assertData()
	t.Logf("upgrade %s -> %s", baselineImage, candidateImage)
	roll(candidateImage, candidateVersion, candidateRelatedImages)
	f.waitSampleVersion(candidateVersion)
	assertData()
	candidateOperandImages := f.operandImages()
	require.NotEqual(t, baselineOperandImages, candidateOperandImages, "no DSPA Deployment image changed during upgrade")
	f.runWorkflow("run-after-upgrade")
	// A ConfigMap-only transition must reach both the module status and DSPA
	// metadata without a restart or a DSPA spec edit.
	f.setVersion(candidateVersion + "-handshake-test")
	f.waitModule(common.Managed, candidateVersion+"-handshake-test")
	f.waitSampleVersion(candidateVersion + "-handshake-test")
	t.Logf("downgrade %s -> %s", candidateImage, baselineImage)
	roll(baselineImage, baselineVersion, baselineRelatedImages)
	f.waitSampleVersion(baselineVersion)
	assertData()
	require.Equal(t, baselineOperandImages, f.operandImages(), "downgrade did not restore baseline DSPA Deployment images")
	f.runWorkflow("run-after-downgrade")
}
