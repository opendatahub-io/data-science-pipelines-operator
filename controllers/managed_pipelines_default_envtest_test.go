//go:build test_all || test_functional

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

package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

func TestManagedPipelinesAPIServerDefaulting(t *testing.T) {
	data, err := os.ReadFile("../config/crd/bases/datasciencepipelinesapplications.opendatahub.io_datasciencepipelinesapplications.yaml")
	require.NoError(t, err)
	var upgradedCRD apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(data, &upgradedCRD))
	require.Nil(t, upgradedCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["apiServer"].Properties["managedPipelines"].Default)
	legacyCRD := upgradedCRD.DeepCopy()
	// Model the released schema: no managed-pipeline default, and null is pruned.
	legacySpec := legacyCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	legacyAPIServer := legacySpec.Properties["apiServer"]
	legacyManaged := legacyAPIServer.Properties["managedPipelines"]
	legacyManaged.Default, legacyManaged.Nullable = nil, false
	delete(legacyManaged.Properties, "enabled")
	legacyAPIServer.Properties["managedPipelines"] = legacyManaged
	legacySpec.Properties["apiServer"] = legacyAPIServer
	legacyCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"] = legacySpec
	testEnv := &envtest.Environment{CRDs: []*apiextensionsv1.CustomResourceDefinition{legacyCRD}}
	config, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testEnv.Stop()) })
	scheme := runtime.NewScheme()
	require.NoError(t, dspav1.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	require.NoError(t, err)
	ctx := context.Background()
	const optOutAnnotation = "datasciencepipelinesapplications.opendatahub.io/disable-managed-pipelines"

	legacyCases := []struct {
		name       string
		apiServer  map[string]interface{}
		optOut     bool
		configured bool
	}{
		{name: "legacy-omitted"},
		{name: "legacy-empty", apiServer: map[string]interface{}{}},
		{name: "legacy-enabled", apiServer: map[string]interface{}{"managedPipelines": map[string]interface{}{"image": "legacy:test"}}, configured: true},
		{name: "legacy-new-field-pruned", apiServer: map[string]interface{}{"managedPipelines": map[string]interface{}{"image": "legacy:test", "enabled": false}}, configured: true},
		{name: "legacy-opt-out", optOut: true},
		{name: "legacy-null-pruned", apiServer: map[string]interface{}{"managedPipelines": nil}},
	}
	for _, tt := range legacyCases {
		spec := map[string]interface{}{"objectStorage": map[string]interface{}{}}
		if tt.apiServer != nil {
			spec["apiServer"] = tt.apiServer
		}
		object := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": dspav1.GroupVersion.String(), "kind": "DataSciencePipelinesApplication",
			"metadata": map[string]interface{}{"name": tt.name, "namespace": "default"}, "spec": spec,
		}}
		require.NoError(t, k8sClient.Create(ctx, object))
		if !tt.configured {
			_, present, err := unstructured.NestedFieldNoCopy(object.Object, "spec", "apiServer", "managedPipelines")
			require.NoError(t, err)
			require.False(t, present, "the old schema must not default or retain null")
		}
		if tt.name == "legacy-new-field-pruned" {
			_, present, err := unstructured.NestedFieldNoCopy(object.Object, "spec", "apiServer", "managedPipelines", "enabled")
			require.NoError(t, err)
			require.False(t, present, "the old CRD cannot retain the new spec opt-out")
		}
		if tt.optOut {
			// This is the documented pre-upgrade annotation operation.
			patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:"true"}}}`, optOutAnnotation)
			require.NoError(t, k8sClient.Patch(ctx, object, client.RawPatch(types.MergePatchType, []byte(patch))))
		}
	}

	var installedCRD apiextensionsv1.CustomResourceDefinition
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: upgradedCRD.Name}, &installedCRD))
	installedCRD.Spec = upgradedCRD.Spec
	require.NoError(t, k8sClient.Update(ctx, &installedCRD))
	// Wait until the API server accepts nullable fields through the new schema.
	require.Eventually(t, func() bool {
		probe := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": dspav1.GroupVersion.String(), "kind": "DataSciencePipelinesApplication",
			"metadata": map[string]interface{}{"name": "schema-probe", "namespace": "default"},
			"spec":     map[string]interface{}{"objectStorage": map[string]interface{}{}, "apiServer": map[string]interface{}{"managedPipelines": nil}},
		}}
		if k8sClient.Create(ctx, probe, client.DryRunAll) != nil {
			return false
		}
		_, present, err := unstructured.NestedFieldNoCopy(probe.Object, "spec", "apiServer", "managedPipelines")
		return err == nil && present
	}, 10*time.Second, 20*time.Millisecond)
	t.Run("CRD upgrade does not persist operator defaults", func(t *testing.T) {
		for _, tt := range legacyCases {
			key := client.ObjectKey{Name: tt.name, Namespace: "default"}
			var dspa dspav1.DataSciencePipelinesApplication
			require.NoError(t, k8sClient.Get(ctx, key, &dspa))
			if tt.configured {
				require.NotNil(t, dspa.Spec.APIServer.ManagedPipelines)
				require.Equal(t, "legacy:test", dspa.Spec.APIServer.ManagedPipelines.Image)
			} else {
				require.Nil(t, dspa.Spec.APIServer.ManagedPipelines)
			}
			if tt.optOut {
				require.Equal(t, "true", dspa.Annotations[optOutAnnotation])
			}
			require.Equal(t, !tt.optOut, dspa.ManagedPipelinesEnabled())
			dspa.Labels = map[string]string{"test": "metadata-update"}
			require.NoError(t, k8sClient.Update(ctx, &dspa))
			require.NoError(t, k8sClient.Get(ctx, key, &dspa))
			require.Equal(t, !tt.configured, dspa.Spec.APIServer.ManagedPipelines == nil)
			if tt.optOut {
				require.Equal(t, "true", dspa.Annotations[optOutAnnotation])
			}
			require.Equal(t, !tt.optOut, dspa.ManagedPipelinesEnabled())
		}
	})

	for _, tt := range []struct {
		name      string
		apiServer map[string]interface{}
		disabled  bool
		optOut    bool
	}{
		{name: "new-omitted-apiserver"},
		{name: "new-omitted-managed", apiServer: map[string]interface{}{}},
		{name: "new-null-follows-default", apiServer: map[string]interface{}{"managedPipelines": nil}},
		{name: "new-annotation-opt-out", disabled: true, optOut: true},
		{name: "new-spec-opt-out", apiServer: map[string]interface{}{"managedPipelines": map[string]interface{}{"enabled": false}}, disabled: true},
		{name: "new-spec-enabled", apiServer: map[string]interface{}{"managedPipelines": map[string]interface{}{"enabled": true}}},
		{name: "new-null-enabled", apiServer: map[string]interface{}{"managedPipelines": map[string]interface{}{"enabled": nil}}},
		{name: "new-annotation-overrides-enabled", apiServer: map[string]interface{}{"managedPipelines": map[string]interface{}{"enabled": true}}, disabled: true, optOut: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := map[string]interface{}{"objectStorage": map[string]interface{}{}}
			if tt.apiServer != nil {
				spec["apiServer"] = tt.apiServer
			}
			object := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": dspav1.GroupVersion.String(), "kind": "DataSciencePipelinesApplication",
				"metadata": map[string]interface{}{"name": tt.name, "namespace": "default"}, "spec": spec,
			}}
			if tt.optOut {
				object.SetAnnotations(map[string]string{optOutAnnotation: "true"})
			}
			require.NoError(t, k8sClient.Create(ctx, object))
			key := client.ObjectKeyFromObject(object)
			var dspa dspav1.DataSciencePipelinesApplication
			require.NoError(t, k8sClient.Get(ctx, key, &dspa))
			configured := tt.apiServer != nil && tt.apiServer["managedPipelines"] != nil
			require.Equal(t, !configured, dspa.Spec.APIServer.ManagedPipelines == nil)
			require.Equal(t, !tt.disabled, dspa.ManagedPipelinesEnabled())
			// The controller adds its finalizer through a typed update.
			dspa.Finalizers = []string{finalizerName}
			require.NoError(t, k8sClient.Update(ctx, &dspa))
			require.NoError(t, k8sClient.Get(ctx, key, object))
			_, present, err := unstructured.NestedFieldNoCopy(object.Object, "spec", "apiServer", "managedPipelines")
			require.NoError(t, err)
			require.Equal(t, configured, present, "typed updates must preserve explicit configuration without adding reconciliation defaults")
			require.NoError(t, k8sClient.Get(ctx, key, &dspa))
			require.Equal(t, !tt.disabled, dspa.ManagedPipelinesEnabled())
		})
	}

	t.Run("invalid enabled type rejected", func(t *testing.T) {
		object := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": dspav1.GroupVersion.String(), "kind": "DataSciencePipelinesApplication",
			"metadata": map[string]interface{}{"name": "invalid-enabled", "namespace": "default"},
			"spec":     map[string]interface{}{"objectStorage": map[string]interface{}{}, "apiServer": map[string]interface{}{"managedPipelines": map[string]interface{}{"enabled": "false"}}},
		}}
		require.True(t, apierrors.IsInvalid(k8sClient.Create(ctx, object)))
	})

	t.Run("patch and apply transitions", func(t *testing.T) {
		object := &dspav1.DataSciencePipelinesApplication{
			ObjectMeta: metav1.ObjectMeta{Name: "patch-transitions", Namespace: "default"},
			Spec:       dspav1.DSPASpec{ObjectStorage: &dspav1.ObjectStorage{}},
		}
		require.NoError(t, k8sClient.Create(ctx, object))
		require.Nil(t, object.Spec.APIServer.ManagedPipelines, "API server must leave managed configuration omitted")
		for _, tt := range []struct {
			name      string
			patchType types.PatchType
			patch     string
			disabled  bool
		}{
			{name: "JSON patch null follows default", patchType: types.JSONPatchType, patch: `[{"op":"add","path":"/spec/apiServer/managedPipelines","value":null}]`},
			{name: "merge patch null follows default", patchType: types.MergePatchType, patch: `{"spec":{"apiServer":{"managedPipelines":null}}}`},
			{name: "server-side apply null follows default", patchType: types.ApplyPatchType, patch: fmt.Sprintf(`{"apiVersion":%q,"kind":"DataSciencePipelinesApplication","metadata":{"name":"patch-transitions","namespace":"default"},"spec":{"apiServer":{"managedPipelines":null}}}`, dspav1.GroupVersion.String())},
			{name: "JSON patch spec disables", patchType: types.JSONPatchType, patch: `[{"op":"add","path":"/spec/apiServer/managedPipelines","value":{"enabled":false}}]`, disabled: true},
			{name: "merge patch spec enables", patchType: types.MergePatchType, patch: `{"spec":{"apiServer":{"managedPipelines":{"enabled":true}}}}`},
			{name: "server-side apply spec disables", patchType: types.ApplyPatchType, patch: fmt.Sprintf(`{"apiVersion":%q,"kind":"DataSciencePipelinesApplication","metadata":{"name":"patch-transitions","namespace":"default"},"spec":{"apiServer":{"managedPipelines":{"enabled":false}}}}`, dspav1.GroupVersion.String()), disabled: true},
			{name: "JSON patch remove enabled follows default", patchType: types.JSONPatchType, patch: `[{"op":"remove","path":"/spec/apiServer/managedPipelines/enabled"}]`},
			{name: "annotation disables", patchType: types.MergePatchType, patch: fmt.Sprintf(`{"metadata":{"annotations":{%q:"true"}}}`, optOutAnnotation), disabled: true},
			{name: "server-side apply empty cannot override annotation", patchType: types.ApplyPatchType, patch: fmt.Sprintf(`{"apiVersion":%q,"kind":"DataSciencePipelinesApplication","metadata":{"name":"patch-transitions","namespace":"default"},"spec":{"apiServer":{"managedPipelines":{}}}}`, dspav1.GroupVersion.String()), disabled: true},
			{name: "explicit enabled cannot override annotation", patchType: types.MergePatchType, patch: `{"spec":{"apiServer":{"managedPipelines":{"enabled":true}}}}`, disabled: true},
			{name: "JSON patch remove cannot override annotation", patchType: types.JSONPatchType, patch: `[{"op":"remove","path":"/spec/apiServer/managedPipelines"}]`, disabled: true},
			{name: "false annotation enables", patchType: types.MergePatchType, patch: fmt.Sprintf(`{"metadata":{"annotations":{%q:"false"}}}`, optOutAnnotation)},
			{name: "annotation removed enables", patchType: types.MergePatchType, patch: fmt.Sprintf(`{"metadata":{"annotations":{%q:null}}}`, optOutAnnotation)},
		} {
			t.Run(tt.name, func(t *testing.T) {
				var options []client.PatchOption
				if tt.patchType == types.ApplyPatchType {
					options = []client.PatchOption{client.FieldOwner("managed-pipelines-test"), client.ForceOwnership}
				}
				require.NoError(t, k8sClient.Patch(ctx, object, client.RawPatch(tt.patchType, []byte(tt.patch)), options...))
				require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(object), object))
				require.Equal(t, !tt.disabled, object.ManagedPipelinesEnabled())
			})
		}
	})
}
