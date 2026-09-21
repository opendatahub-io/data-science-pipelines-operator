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

package argo //nolint:testpackage // asset count intentionally validates the private embedded manifest list

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestObjects(t *testing.T) {
	objects, err := Objects("opendatahub")
	require.NoError(t, err)
	require.Len(t, objects, len(assetNames))

	foundWorkflowCRD := false
	foundArgoRole := false
	for _, object := range objects {
		require.NotEmpty(t, object.GetAPIVersion())
		require.NotEmpty(t, object.GetKind())
		require.NotEmpty(t, object.GetName())
		if isNamespacedAsset(object.GetKind()) {
			require.Equal(t, "opendatahub", object.GetNamespace(), "%s/%s", object.GetKind(), object.GetName())
		} else {
			require.Empty(t, object.GetNamespace(), "%s/%s", object.GetKind(), object.GetName())
		}
		if object.GetName() == "workflows.argoproj.io" {
			foundWorkflowCRD = true
		}
		if object.GetName() == "argo-role" {
			foundArgoRole = true
		}
	}
	require.True(t, foundWorkflowCRD)
	require.True(t, foundArgoRole)
}

func TestModuleOverlaysInstallArgoCRDs(t *testing.T) {
	for _, overlay := range []string{"odh", "rhoai"} {
		t.Run(overlay, func(t *testing.T) {
			data, err := os.ReadFile("../overlays/" + overlay + "/dspo/kustomization.yaml")
			require.NoError(t, err)
			require.Contains(t, strings.Split(string(data), "\n"), "- ../../../argo/crds")
		})
	}
}

func TestArgoCRDBootstrapIncludesOwnershipMarkers(t *testing.T) {
	data, err := os.ReadFile("crds/kustomization.yaml")
	require.NoError(t, err)

	var kustomization struct {
		Labels []struct {
			Pairs map[string]string `json:"pairs"`
		} `json:"labels"`
		CommonAnnotations map[string]string `json:"commonAnnotations"`
	}
	require.NoError(t, yaml.Unmarshal(data, &kustomization))

	labelPairs := make(map[string]string)
	for _, label := range kustomization.Labels {
		for key, value := range label.Pairs {
			labelPairs[key] = value
		}
	}
	require.Equal(t, "true", labelPairs["app.opendatahub.io/data-science-pipelines-operator"])
	require.Equal(t, "v2", labelPairs["dsp-version"])
	require.Equal(t, "true", kustomization.CommonAnnotations["aipipelines.components.platform.opendatahub.io/argo-managed"])
}
