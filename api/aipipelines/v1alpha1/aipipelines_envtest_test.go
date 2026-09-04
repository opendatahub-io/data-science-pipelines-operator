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

package v1alpha1_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestCRDValidationAndDefaulting(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	config, err := testEnv.Start()
	if err != nil {
		t.Fatalf("starting envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Errorf("stopping envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("registering AIPipelines API: %v", err)
	}
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("creating Kubernetes client: %v", err)
	}
	ctx := context.Background()

	t.Run("rejects a non-singleton name", func(t *testing.T) {
		resource := &v1alpha1.AIPipelines{
			ObjectMeta: metav1.ObjectMeta{Name: "another-aipipelines"},
			Spec:       v1alpha1.AIPipelinesSpec{},
		}
		if err := k8sClient.Create(ctx, resource); !apierrors.IsInvalid(err) {
			t.Fatalf("Create() error = %v, want Invalid", err)
		}
	})

	t.Run("rejects an invalid management state", func(t *testing.T) {
		resource := &v1alpha1.AIPipelines{
			ObjectMeta: metav1.ObjectMeta{Name: v1alpha1.AIPipelinesInstanceName},
			Spec: v1alpha1.AIPipelinesSpec{
				ArgoWorkflowsControllers: &v1alpha1.ArgoWorkflowsControllersSpec{
					ManagementState: common.ManagementState("Invalid"),
				},
			},
		}
		if err := k8sClient.Create(ctx, resource); !apierrors.IsInvalid(err) {
			t.Fatalf("Create() error = %v, want Invalid", err)
		}
	})

	t.Run("defaults management state to Managed", func(t *testing.T) {
		resource := &v1alpha1.AIPipelines{
			ObjectMeta: metav1.ObjectMeta{Name: v1alpha1.AIPipelinesInstanceName},
			Spec: v1alpha1.AIPipelinesSpec{
				ArgoWorkflowsControllers: &v1alpha1.ArgoWorkflowsControllersSpec{},
			},
		}
		if err := k8sClient.Create(ctx, resource); err != nil {
			t.Fatalf("creating AIPipelines: %v", err)
		}

		created := &v1alpha1.AIPipelines{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: resource.Name}, created); err != nil {
			t.Fatalf("getting AIPipelines: %v", err)
		}
		if created.Spec.ArgoWorkflowsControllers == nil {
			t.Fatal("argoWorkflowsControllers is nil after defaulting")
		}
		if got := created.Spec.ArgoWorkflowsControllers.ManagementState; got != common.Managed {
			t.Errorf("defaulted management state = %q, want %q", got, common.Managed)
		}
	})
}
