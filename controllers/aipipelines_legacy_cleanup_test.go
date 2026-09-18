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

package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCleanupLegacyDataSciencePipelines(t *testing.T) {
	tests := []struct {
		name       string
		owner      *metav1.OwnerReference
		wantDelete bool
	}{
		{
			name: "DSC v1 owner",
			owner: &metav1.OwnerReference{
				APIVersion: "datasciencecluster.opendatahub.io/v1",
				Kind:       "DataScienceCluster",
				Name:       "default-dsc",
			},
			wantDelete: true,
		},
		{
			name: "DSC v2 owner",
			owner: &metav1.OwnerReference{
				APIVersion: "datasciencecluster.opendatahub.io/v2",
				Kind:       "DataScienceCluster",
				Name:       "default-dsc",
			},
			wantDelete: true,
		},
		{
			name: "foreign owner",
			owner: &metav1.OwnerReference{
				APIVersion: "example.test/v1",
				Kind:       "Owner",
				Name:       "foreign",
			},
		},
		{name: "no owner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypeWithName(legacyDataSciencePipelinesGVK, &unstructured.Unstructured{})

			legacy := newLegacyDataSciencePipelines()
			if tt.owner != nil {
				legacy.SetOwnerReferences([]metav1.OwnerReference{*tt.owner})
			}
			cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()

			require.NoError(t, cleanupLegacyDataSciencePipelines(context.Background(), cli))

			actual := newLegacyDataSciencePipelines()
			err := cli.Get(context.Background(), client.ObjectKeyFromObject(actual), actual)
			if tt.wantDelete {
				require.True(t, apierrors.IsNotFound(err), "expected legacy CR deletion, got %v", err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCleanupLegacyDataSciencePipelinesIgnoresMissingCR(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(legacyDataSciencePipelinesGVK, &unstructured.Unstructured{})
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	require.NoError(t, cleanupLegacyDataSciencePipelines(context.Background(), cli))
}

func newLegacyDataSciencePipelines() *unstructured.Unstructured {
	legacy := &unstructured.Unstructured{}
	legacy.SetGroupVersionKind(legacyDataSciencePipelinesGVK)
	legacy.SetName(legacyDataSciencePipelinesName)
	return legacy
}
