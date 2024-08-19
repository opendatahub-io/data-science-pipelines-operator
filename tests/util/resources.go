/*
Copyright 2023.
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
	"fmt"
	"testing"
	"time"

	mf "github.com/manifestival/manifestival"
	"github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeployDSPA will deploy resource found in path by requesting
func DeployDSPA(t *testing.T, ctx context.Context, client client.Client, deployDSPA *v1alpha1.DataSciencePipelinesApplication, dspaNS string, timeout, interval time.Duration) error {
	deployDSPA.ObjectMeta.Namespace = dspaNS
	err := client.Create(ctx, deployDSPA)
	require.NoError(t, err)

	nsn := types.NamespacedName{
		Name:      deployDSPA.ObjectMeta.Name,
		Namespace: dspaNS,
	}
	fetchedDspa := &v1alpha1.DataSciencePipelinesApplication{}
	return waitFor(ctx, timeout, interval, func() (bool, error) {
		err := client.Get(ctx, nsn, fetchedDspa)
		if err != nil {
			return false, err
		}
		return true, nil
	})
}

// WaitForDSPAReady will assert for DSPA CR Ready Status
func WaitForDSPAReady(t *testing.T, ctx context.Context, client client.Client, dspaName, dspaNS string, timeout, interval time.Duration) error {
	nsn := types.NamespacedName{
		Name:      dspaName,
		Namespace: dspaNS,
	}
	dspa := &v1alpha1.DataSciencePipelinesApplication{}
	return waitFor(ctx, timeout, interval, func() (bool, error) {
		err := client.Get(ctx, nsn, dspa)
		if err != nil {
			return false, err
		}
		for _, condition := range dspa.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

// DeleteDSPA will delete DSPA found in path by requesting
func DeleteDSPA(t *testing.T, ctx context.Context, client client.Client, dspaName, dspaNS string, timeout, interval time.Duration) error {
	nsn := types.NamespacedName{
		Name:      dspaName,
		Namespace: dspaNS,
	}
	dspa := &v1alpha1.DataSciencePipelinesApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dspaName,
			Namespace: dspaNS,
		},
	}
	err := client.Delete(ctx, dspa)
	require.NoError(t, err)
	return waitFor(ctx, timeout, interval, func() (bool, error) {
		err := client.Get(ctx, nsn, dspa)
		if apierrs.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func TestForSuccessfulDeployment(t *testing.T, ctx context.Context, namespace, deploymentName string, client client.Client) {
	deployment := &appsv1.Deployment{}
	nsn := types.NamespacedName{
		Name:      deploymentName,
		Namespace: namespace,
	}
	err := client.Get(ctx, nsn, deployment)
	require.NoError(t, err)
	deploymentAvailable := false
	for _, condition := range deployment.Status.Conditions {
		if condition.Reason == "MinimumReplicasAvailable" && condition.Type == appsv1.DeploymentAvailable {
			deploymentAvailable = true
			break
		}
	}
	require.True(t, deploymentAvailable)
}

func GetDSPAFromPath(t *testing.T, opts mf.Option, path string) *v1alpha1.DataSciencePipelinesApplication {
	dspa := &v1alpha1.DataSciencePipelinesApplication{}
	manifest, err := mf.NewManifest(path, opts)
	require.NoError(t, err)
	expected := &manifest.Resources()[0]
	err = scheme.Scheme.Convert(expected, dspa, nil)
	require.NoError(t, err)
	return dspa
}

// waitFor is a helper function
func waitFor(ctx context.Context, timeout, interval time.Duration, conditionFunc func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		done, err := conditionFunc()
		if done {
			return nil
		}
		if err != nil {
			return err
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("timed out waiting for condition")
}

func PrintConditions(ctx context.Context, dspa *v1alpha1.DataSciencePipelinesApplication, namespace string, client client.Client) string {
	nsn := types.NamespacedName{
		Name:      dspa.Name,
		Namespace: namespace,
	}
	err := client.Get(ctx, nsn, dspa)
	if err != nil {
		return "No conditions"
	}
	conditions := ""
	for _, condition := range dspa.Status.Conditions {
		conditions = conditions + fmt.Sprintf("Type: %s, Status: %s, Message: %s\n", condition.Type, condition.Status, condition.Message)
	}
	return conditions
}
