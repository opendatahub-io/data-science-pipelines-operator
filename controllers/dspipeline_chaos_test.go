//go:build test_chaos

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
	"errors"
	"fmt"
	"strings"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/opendatahub-io/operator-chaos/pkg/sdk"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func setupChaosTest(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
}

func newChaosReconciler(faults *sdk.FaultConfig) *DSPAReconciler {
	base := NewFakeController()
	if faults != nil {
		base.Client = sdk.NewChaosClient(base.Client, faults)
	}
	return base
}

func sanitizeTestName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, "/", "-")
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

func chaosReconcileRequest(dspa *dspav1.DataSciencePipelinesApplication) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      dspa.Name,
			Namespace: dspa.Namespace,
		},
	}
}

func createChaosTestDSPA(name string) *dspav1.DataSciencePipelinesApplication {
	dspa := testutil.CreateEmptyDSPA()
	dspa.Name = sanitizeTestName(name)
	dspa.Namespace = "default"
	dspa.Spec = dspav1.DSPASpec{
		PodToPodTLS: testutil.BoolPtr(false),
		DSPVersion:  "v2",
		APIServer: &dspav1.APIServer{
			Deploy: true,
		},
		PersistenceAgent: &dspav1.PersistenceAgent{
			Deploy: false,
		},
		ScheduledWorkflow: &dspav1.ScheduledWorkflow{
			Deploy: false,
		},
		WorkflowController: &dspav1.WorkflowController{
			Deploy: false,
		},
		MLMD: &dspav1.MLMD{
			Deploy: false,
		},
		Database: &dspav1.Database{
			DisableHealthCheck: true,
			MariaDB: &dspav1.MariaDB{
				Deploy: true,
			},
		},
		ObjectStorage: &dspav1.ObjectStorage{
			DisableHealthCheck: true,
			Minio: &dspav1.Minio{
				Deploy: true,
				Image:  "minio/minio:latest",
			},
		},
	}
	return dspa
}

func TestChaos_ReconcileHandlesGetErrors(t *testing.T) {
	setupChaosTest(t)
	cfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
		sdk.OpGet: {ErrorRate: 1.0, Error: "chaos: connection refused"},
	})

	reconciler := newChaosReconciler(cfg)
	ctx := context.Background()

	dspa := createChaosTestDSPA(t.Name())
	err := reconciler.Client.Create(ctx, dspa)
	require.NoError(t, err)

	_, err = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))

	require.Error(t, err, "expected a chaos error on Get")
	var chaosErr *sdk.ChaosError
	require.True(t, errors.As(err, &chaosErr), "error should be a ChaosError")
	assert.Equal(t, sdk.OpGet, chaosErr.Operation)
}

func TestChaos_ReconcileConvergesAfterTransientGetErrors(t *testing.T) {
	setupChaosTest(t)
	cfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
		sdk.OpGet: {ErrorRate: 1.0, Error: "chaos: transient connection refused"},
	})

	reconciler := newChaosReconciler(cfg)
	ctx := context.Background()

	dspa := createChaosTestDSPA(t.Name())
	err := reconciler.Client.Create(ctx, dspa)
	require.NoError(t, err)

	_, err = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
	assert.Error(t, err)

	cfg.Deactivate()

	_, err = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
	assert.NoError(t, err, "reconciler should converge once Get errors stop")
}

func TestChaos_ReconcileHandlesCreateErrors(t *testing.T) {
	setupChaosTest(t)
	cfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
		sdk.OpCreate: {ErrorRate: 1.0, Error: "chaos: quota exceeded"},
	})

	reconciler := newChaosReconciler(cfg)
	ctx := context.Background()

	dspa := createChaosTestDSPA(t.Name())
	cfg.Deactivate()
	err := reconciler.Client.Create(ctx, dspa)
	require.NoError(t, err)
	cfg.Activate()

	result, err := reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))

	if err != nil {
		var chaosErr *sdk.ChaosError
		require.True(t, errors.As(err, &chaosErr), "returned error should be a ChaosError")
	} else {
		assert.True(t, result.Requeue || result.RequeueAfter > 0,
			"reconciler should requeue when Creates fail")
	}
}

func TestChaos_ReconcileConvergesAfterTransientCreateErrors(t *testing.T) {
	setupChaosTest(t)
	cfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
		sdk.OpCreate: {ErrorRate: 1.0, Error: "chaos: quota exceeded"},
	})

	reconciler := newChaosReconciler(cfg)
	ctx := context.Background()

	dspa := createChaosTestDSPA(t.Name())
	cfg.Deactivate()
	err := reconciler.Client.Create(ctx, dspa)
	require.NoError(t, err)
	cfg.Activate()

	_, _ = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))

	cfg.Deactivate()

	_, err = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
	assert.NoError(t, err, "reconciler should converge once Create failures stop")
}

func TestChaos_ReconcileToleratesIntermittentErrors(t *testing.T) {
	setupChaosTest(t)

	cfg := sdk.NewFaultConfig(nil)
	reconciler := newChaosReconciler(cfg)
	ctx := context.Background()

	dspa := createChaosTestDSPA(t.Name())
	cfg.Deactivate()
	err := reconciler.Client.Create(ctx, dspa)
	require.NoError(t, err)

	faults := []struct {
		op   sdk.Operation
		spec sdk.FaultSpec
	}{
		{sdk.OpGet, sdk.FaultSpec{ErrorRate: 1.0, Error: "chaos: get timeout"}},
		{sdk.OpCreate, sdk.FaultSpec{ErrorRate: 1.0, Error: "chaos: create quota exceeded"}},
		{sdk.OpUpdate, sdk.FaultSpec{ErrorRate: 1.0, Error: "chaos: update conflict"}},
	}

	for _, f := range faults {
		cfg.SetFault(f.op, f.spec)
		cfg.Activate()
		result, faultErr := reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
		faulted := faultErr != nil || result.Requeue || result.RequeueAfter > 0
		assert.True(t, faulted, fmt.Sprintf("reconciler should error or requeue with %s fault active", f.op))
		cfg.RemoveFault(f.op)
		cfg.Deactivate()
	}

	_, err = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
	assert.NoError(t, err, "reconciler should converge after all faults are removed")
}

func TestChaos_ReconcilerResumesAfterClientRecovery(t *testing.T) {
	setupChaosTest(t)
	cfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
		sdk.OpGet:    {ErrorRate: 1.0, Error: "chaos: total outage"},
		sdk.OpCreate: {ErrorRate: 1.0, Error: "chaos: total outage"},
		sdk.OpUpdate: {ErrorRate: 1.0, Error: "chaos: total outage"},
	})

	reconciler := newChaosReconciler(cfg)
	ctx := context.Background()

	dspa := createChaosTestDSPA(t.Name())
	cfg.Deactivate()
	err := reconciler.Client.Create(ctx, dspa)
	require.NoError(t, err)
	cfg.Activate()

	_, err = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
	assert.Error(t, err, "reconciler should fail during total API outage")

	cfg.Deactivate()

	_, err = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
	assert.NoError(t, err, "reconciler should succeed after API recovery")

	updatedDSPA := &dspav1.DataSciencePipelinesApplication{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{
		Name:      dspa.Name,
		Namespace: dspa.Namespace,
	}, updatedDSPA)
	assert.NoError(t, err)
	assert.NotEmpty(t, updatedDSPA.Status.Conditions,
		"DSPA should have status conditions after successful reconcile")
}

func TestChaos_ReconcileHandlesUpdateConflict(t *testing.T) {
	setupChaosTest(t)

	cfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
		sdk.OpUpdate: {ErrorRate: 1.0, Error: "chaos: the object has been modified"},
	})
	cfg.Deactivate()

	reconciler := newChaosReconciler(cfg)
	ctx := context.Background()

	dspa := createChaosTestDSPA(t.Name())
	err := reconciler.Client.Create(ctx, dspa)
	require.NoError(t, err)

	// Run initial reconcile to create all resources without faults
	_, err = reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
	require.NoError(t, err, "initial reconcile should succeed")

	// Mutate the DSPA spec to force the reconciler into the update path
	err = reconciler.Client.Get(ctx, types.NamespacedName{Name: dspa.Name, Namespace: dspa.Namespace}, dspa)
	require.NoError(t, err)
	if dspa.Annotations == nil {
		dspa.Annotations = map[string]string{}
	}
	dspa.Annotations["chaos-test"] = "force-update"
	err = reconciler.Client.Update(ctx, dspa)
	require.NoError(t, err)

	// Activate update faults and reconcile again
	cfg.Activate()

	result, err := reconciler.Reconcile(ctx, chaosReconcileRequest(dspa))
	if err != nil {
		var chaosErr *sdk.ChaosError
		require.True(t, errors.As(err, &chaosErr), "returned error should be a ChaosError")
		assert.Equal(t, sdk.OpUpdate, chaosErr.Operation)
	} else {
		assert.True(t, result.Requeue || result.RequeueAfter > 0,
			"reconciler should requeue when Updates fail")
	}
}

func TestChaos_ReconcileWithWrapReconciler(t *testing.T) {
	setupChaosTest(t)
	faults := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
		sdk.OpReconcile: {ErrorRate: 1.0, Error: "chaos: reconcile blocked"},
	})

	base := NewFakeController()
	ctx := context.Background()

	dspa := createChaosTestDSPA(t.Name())
	err := base.Client.Create(ctx, dspa)
	require.NoError(t, err)

	wrapped := sdk.WrapReconciler(base, sdk.WithFaultConfig(faults))

	_, err = wrapped.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dspa.Name,
			Namespace: dspa.Namespace,
		},
	})

	require.Error(t, err, "wrapped reconciler should inject fault at reconcile level")
	var chaosErr *sdk.ChaosError
	require.True(t, errors.As(err, &chaosErr))
	assert.Equal(t, sdk.OpReconcile, chaosErr.Operation)
}

func TestChaos_FaultConfigActivateDeactivate(t *testing.T) {
	cfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
		sdk.OpGet: {ErrorRate: 1.0, Error: "chaos: blocked"},
	})

	assert.True(t, cfg.IsActive())

	cfg.Deactivate()
	assert.False(t, cfg.IsActive())
	assert.NoError(t, cfg.MaybeInject(sdk.OpGet), "should not inject when deactivated")

	cfg.Activate()
	assert.True(t, cfg.IsActive())
	err := cfg.MaybeInject(sdk.OpGet)
	assert.Error(t, err, "should inject when activated")
}

func TestChaos_NewForTestCleansUpAutomatically(t *testing.T) {
	tc := sdk.NewForTest(t, "dspo-apiserver")
	assert.Equal(t, "dspo-apiserver", tc.Component())

	tc.Activate(sdk.OpGet, sdk.FaultSpec{ErrorRate: 1.0, Error: "test"})
	err := tc.Config().MaybeInject(sdk.OpGet)
	assert.Error(t, err)

	tc.Deactivate(sdk.OpGet)
	err = tc.Config().MaybeInject(sdk.OpGet)
	assert.NoError(t, err)
}
