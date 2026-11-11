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
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestWaitForDeploymentAvailable covers readiness transitions, missing deployments,
// timeout diagnostics, API errors, and cancellation.
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

// TestWaitForDeploymentAvailableBoundsAPIRequest verifies that the deployment
// wait timeout also bounds a blocked Kubernetes API request.
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

// TestForDeploymentAbsenceUsesStatus verifies that condition reasons cannot
// turn an unavailable deployment into an available one.
func TestForDeploymentAbsenceUsesStatus(t *testing.T) {
	for _, status := range []corev1.ConditionStatus{corev1.ConditionFalse, corev1.ConditionUnknown} {
		t.Run(string(status), func(t *testing.T) {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "workflow-controller", Namespace: "test-dspa"},
				Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentAvailable, Status: status, Reason: "MinimumReplicasAvailable",
				}}},
			}
			kubernetesClient := fake.NewClientBuilder().WithObjects(deployment).Build()
			TestForDeploymentAbsence(t, context.Background(), "test-dspa", deployment.Name, kubernetesClient)
		})
	}
}

// TestForDeploymentAbsenceRejectsAvailable verifies that Available=True fails
// the absence assertion regardless of the condition reason.
// A subprocess isolates the expected testing.T failure from the parent test.
func TestForDeploymentAbsenceRejectsAvailable(t *testing.T) {
	const childProcess = "DSPO_TEST_AVAILABLE_DEPLOYMENT"
	if os.Getenv(childProcess) == "1" {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "workflow-controller", Namespace: "test-dspa"},
			Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, Reason: "AnyAvailableReason",
			}}},
		}
		kubernetesClient := fake.NewClientBuilder().WithObjects(deployment).Build()
		TestForDeploymentAbsence(t, context.Background(), "test-dspa", deployment.Name, kubernetesClient)
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestForDeploymentAbsenceRejectsAvailable$")
	command.Env = append(os.Environ(), childProcess+"=1")
	output, err := command.CombinedOutput()
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError, "an available deployment must fail the absence assertion: %s", output)
	require.Equal(t, 1, exitError.ExitCode(), "%s", output)
	require.Contains(t, string(output), "Should be false", "expected the absence assertion to reject availability, not an unrelated failure")
}

// TestWaitForInterruptsPolling verifies cancellation interrupts a polling delay
// and remains distinguishable from a deadline expiration.
func TestWaitForInterruptsPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstCheck := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		result <- WaitFor(ctx, 2*time.Second, 2*time.Second, func() (bool, error) {
			select {
			case firstCheck <- struct{}{}:
			default:
			}
			return false, nil
		})
	}()
	select {
	case <-firstCheck:
	case <-time.After(time.Second):
		t.Fatal("wait did not check the condition immediately")
	}
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt the polling interval")
	}
}

// TestWaitForDeadlineAndCanceledContext covers a deadline shorter than the
// interval and cancellation before the first condition check.
func TestWaitForDeadlineAndCanceledContext(t *testing.T) {
	t.Run("zero timeout", func(t *testing.T) {
		err := WaitFor(context.Background(), 0, time.Millisecond, func() (bool, error) {
			t.Error("an expired wait must not call the condition")
			return true, nil
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
	t.Run("deadline shorter than interval", func(t *testing.T) {
		result := make(chan error, 1)
		go func() {
			result <- WaitFor(context.Background(), 20*time.Millisecond, 5*time.Second, func() (bool, error) {
				return false, nil
			})
		}()
		// Allow generous scheduling slack while remaining well below the
		// polling interval: the right error alone does not prove a bound.
		select {
		case err := <-result:
			require.ErrorIs(t, err, context.DeadlineExceeded)
		case <-time.After(time.Second):
			t.Fatal("deadline did not interrupt the polling interval")
		}
	})
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := WaitFor(ctx, time.Second, time.Millisecond, func() (bool, error) {
			t.Error("a canceled wait must not call the condition")
			return true, nil
		})
		require.ErrorIs(t, err, context.Canceled)
	})
}
