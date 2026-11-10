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
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestWaitForDeploymentAvailable(t *testing.T) {
	available := appsv1.DeploymentCondition{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, Reason: "AnyAvailableReason"}
	unavailable := appsv1.DeploymentCondition{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse, Reason: "MinimumReplicasAvailable"}
	unknown := appsv1.DeploymentCondition{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionUnknown}
	progressing := appsv1.DeploymentCondition{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue}
	notFound := apierrs.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "scheduledworkflow")
	forbidden := apierrs.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "scheduledworkflow", errors.New("access denied"))

	type observation struct {
		conditions []appsv1.DeploymentCondition
		err        error
	}
	ready := observation{conditions: []appsv1.DeploymentCondition{available}}
	for _, scenario := range []struct {
		name            string
		observations    []observation
		timeout         bool
		expectedError   error
		cancelBefore    bool
		cancelAfterRead bool
		diagnostic      string
	}{
		{name: "already available with any reason", observations: []observation{ready}},
		{name: "transient unavailability", observations: []observation{{conditions: []appsv1.DeploymentCondition{unavailable}}, ready}},
		{name: "unknown availability", observations: []observation{{conditions: []appsv1.DeploymentCondition{unknown}}, ready}},
		{name: "no conditions yet", observations: []observation{{}, ready}},
		{name: "other condition is true", observations: []observation{{conditions: []appsv1.DeploymentCondition{progressing}}, ready}},
		{name: "deployment appears later", observations: []observation{{err: notFound}, ready}},
		{name: "availability timeout", observations: []observation{{conditions: []appsv1.DeploymentCondition{unavailable}}}, timeout: true, diagnostic: "MinimumReplicasAvailable"},
		{name: "deployment never appears", observations: []observation{{err: notFound}}, timeout: true, diagnostic: "not found"},
		{name: "API error", observations: []observation{{err: forbidden}}, expectedError: forbidden},
		{name: "already canceled", cancelBefore: true, expectedError: context.Canceled},
		{name: "canceled while waiting", observations: []observation{{}}, cancelAfterRead: true, expectedError: context.Canceled},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario.cancelBefore {
				cancel()
			}
			reads := 0
			kubernetesClient := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
					require.Equal(t, client.ObjectKey{Namespace: "test-k8s-dspa", Name: "scheduledworkflow"}, key)
					index := reads
					reads++
					if index >= len(scenario.observations) {
						index = len(scenario.observations) - 1
					}
					require.GreaterOrEqual(t, index, 0, "canceled wait must not fetch deployment")
					observed := scenario.observations[index]
					object.(*appsv1.Deployment).Status.Conditions = observed.conditions
					if scenario.cancelAfterRead {
						cancel()
					}
					return observed.err
				},
			}).Build()
			timeout := time.Second
			if scenario.timeout {
				timeout = 20 * time.Millisecond
			}
			err := WaitForDeploymentAvailable(ctx, kubernetesClient, "test-k8s-dspa", "scheduledworkflow", timeout, time.Millisecond)
			if scenario.timeout || scenario.expectedError != nil {
				require.Error(t, err)
				require.Contains(t, err.Error(), "test-k8s-dspa/scheduledworkflow")
				if scenario.expectedError != nil {
					require.ErrorIs(t, err, scenario.expectedError)
				}
				if scenario.diagnostic != "" {
					require.Contains(t, err.Error(), scenario.diagnostic)
				}
			} else {
				require.NoError(t, err)
			}
			if scenario.timeout {
				require.Positive(t, reads)
			} else {
				require.Equal(t, len(scenario.observations), reads)
			}
		})
	}
}

func TestWaitForDeploymentAvailableBoundsAPIRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	parentDeadline, _ := ctx.Deadline()
	kubernetesClient := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
			requestDeadline, hasDeadline := ctx.Deadline()
			require.True(t, hasDeadline)
			require.True(t, requestDeadline.Before(parentDeadline), "API request must use the deployment wait timeout")
			<-ctx.Done()
			return ctx.Err()
		},
	}).Build()
	err := WaitForDeploymentAvailable(ctx, kubernetesClient, "test-k8s-dspa", "scheduledworkflow", 20*time.Millisecond, time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Contains(t, err.Error(), "test-k8s-dspa/scheduledworkflow")
}
