//go:build test_all || test_unit

/*

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

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func NewFakeController() *DSPAReconciler {
	// Setup Fake Client Builder
	FakeBuilder := fake.NewClientBuilder()

	// Create Scheme
	FakeScheme := scheme.Scheme
	utilruntime.Must(clientgoscheme.AddToScheme(FakeScheme))
	utilruntime.Must(buildv1.Install(FakeScheme))
	utilruntime.Must(imagev1.Install(FakeScheme))
	utilruntime.Must(routev1.Install(FakeScheme))
	utilruntime.Must(dspav1alpha1.AddToScheme(FakeScheme))
	FakeBuilder.WithScheme(FakeScheme)

	// Build Fake Client
	FakeClient := FakeBuilder.Build()

	// Generate DSPAReconciler using Fake Client
	r := &DSPAReconciler{
		Client:        FakeClient,
		Log:           ctrl.Log.WithName("controllers").WithName("ds-pipelines-controller"),
		Scheme:        FakeScheme,
		TemplatesPath: "../config/internal/",
	}

	return r
}

func CreateNewTestObjects() (context.Context, *DSPAParams, *DSPAReconciler) {
	return context.Background(), &DSPAParams{}, NewFakeController()
}

func (r *DSPAReconciler) IsResourceCreated(ctx context.Context, obj client.Object, name, namespace string) (bool, error) {
	// Fake Request for verification
	nn := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	// Fetch
	err := r.Get(ctx, nn, obj)

	// Err shouldnt be thrown if resource exists
	// TODO: implement better verification
	if err != nil {
		if apierrs.IsNotFound(err) {
			return false, nil
		} else {
			return false, err
		}
	}
	return true, nil
}
