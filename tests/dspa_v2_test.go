//go:build test_integration

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

package integration

import (
	"fmt"
	"testing"
	"time"

	testUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (suite *IntegrationTestSuite) TestDSPADeployment() {
	podCount := 8
	if suite.DSPA.Spec.ObjectStorage.ExternalStorage != nil {
		podCount--
	}
	if suite.DSPA.Spec.Database.ExternalDB != nil {
		podCount--
	}
	deployments := []string{
		fmt.Sprintf("ds-pipeline-%s", suite.DSPA.Name),
		fmt.Sprintf("ds-pipeline-persistenceagent-%s", suite.DSPA.Name),
		fmt.Sprintf("ds-pipeline-scheduledworkflow-%s", suite.DSPA.Name),
	}

	skippedDeployments := []string{}

	if suite.DSPA.Spec.ObjectStorage.ExternalStorage == nil && suite.DSPA.Spec.Database.ExternalDB == nil {
		deployments = append(deployments,
			fmt.Sprintf("mariadb-%s", suite.DSPA.Name),
			fmt.Sprintf("minio-%s", suite.DSPA.Name),
		)
	}

	if suite.ArgoWorkflowsControllersManagementState == "Managed" || suite.ArgoWorkflowsControllersManagementState == "" {
		deployments = append(deployments,
			fmt.Sprintf("ds-pipeline-workflow-controller-%s", suite.DSPA.Name),
		)
	} else {
		podCount--
		skippedDeployments = append(skippedDeployments,
			fmt.Sprintf("ds-pipeline-workflow-controller-%s", suite.DSPA.Name),
		)
	}

	suite.T().Run("with default MariaDB and Minio", func(t *testing.T) {
		t.Run(fmt.Sprintf("should have %d pods", podCount), func(t *testing.T) {
			timeout := time.Second * 120
			interval := time.Second * 2
			actualPodCount := 0

			require.Eventually(t, func() bool {
				podList := &corev1.PodList{}
				// retrieve the running pods only, to allow for multiple reruns of  the test suite
				listOpts := []client.ListOption{
					client.InNamespace(suite.DSPANamespace),
					client.MatchingFields{"status.phase": string(corev1.PodRunning)},
				}
				err := suite.Clientmgr.k8sClient.List(suite.Ctx, podList, listOpts...)
				require.NoError(suite.T(), err)
				actualPodCount = len(podList.Items)

				// Print out pod statuses for troubleshooting
				if podCount != actualPodCount {
					t.Log(fmt.Sprintf("expected %d pods to successfully deploy, got %d instead. Pods in the namespace:", podCount, actualPodCount))
					totalPodList := &corev1.PodList{}
					listOpts1 := []client.ListOption{
						client.InNamespace(suite.DSPANamespace),
					}
					err1 := suite.Clientmgr.k8sClient.List(suite.Ctx, totalPodList, listOpts1...)
					require.NoError(t, err1)
					for _, pod := range totalPodList.Items {
						t.Log(fmt.Sprintf("Pod Name: %s, Status: %s", pod.Name, pod.Status.Phase))
					}
					return false
				} else {
					return true
				}
			}, timeout, interval)
		})

		testUtil.WaitForDSPAReady(t, suite.Ctx, suite.Clientmgr.k8sClient, suite.DSPA.Name, suite.DSPANamespace, DeployTimeout, PollInterval)

		for _, deployment := range deployments {
			t.Run(fmt.Sprintf("should have a ready %s deployment", deployment), func(t *testing.T) {
				testUtil.TestForSuccessfulDeployment(t, suite.Ctx, suite.DSPANamespace, deployment, suite.Clientmgr.k8sClient)
			})
		}
		for _, deployment := range skippedDeployments {
			t.Run(fmt.Sprintf("should not have a ready %s deployment", deployment), func(t *testing.T) {
				testUtil.TestForDeploymentAbsence(t, suite.Ctx, suite.DSPANamespace, deployment, suite.Clientmgr.k8sClient)
			})
		}
	})
}

func (suite *IntegrationTestSuite) TestDSPADeploymentWithK8sNativeApi() {
	// Skip the entire test if PipelineStore is not set to kubernetes
	if suite.DSPA.Spec.APIServer.PipelineStore != "kubernetes" {
		suite.T().Log("PipelineStore is not set to kubernetes, skipping K8s Native API verification tests")
		suite.T().SkipNow()
	}
	suite.T().Run("should verify API Server configuration based on PipelineStore", func(t *testing.T) {
		timeout := time.Second * 120
		interval := time.Second * 2

		// Verify API Server configuration
		require.Eventually(t, func() bool {
			podList := &corev1.PodList{}
			err := suite.Clientmgr.k8sClient.List(suite.Ctx, podList,
				client.InNamespace(suite.DSPANamespace),
				client.MatchingLabels{
					"app": fmt.Sprintf("ds-pipeline-%s", suite.DSPA.Name),
				},
			)
			require.NoError(t, err)

			if len(podList.Items) == 0 {
				t.Logf("Expected at least 1 API Server pod, but found %d", len(podList.Items))
				return false
			}

			pod := podList.Items[0]
			if pod.Status.Phase != corev1.PodRunning {
				t.Logf("API Server pod %s is not running (status: %s)", pod.Name, pod.Status.Phase)
				return false
			}

			for _, container := range pod.Spec.Containers {
				if container.Name == "ds-pipeline-api-server" {
					// Look for the K8s Native API flag
					for _, arg := range container.Args {
						if arg == "--pipelinesStoreKubernetes=true" {
							return true
						}
					}
					return false
				}
			}
			return false
		}, timeout, interval)
	})

	suite.T().Run("should verify webhook pod is running in DSPO namespace", func(t *testing.T) {
		timeout := 120 * time.Second
		interval := 2 * time.Second
		dspoNamespace := "opendatahub"
		webhookName := "ds-pipelines-webhook"

		require.Eventually(t, func() bool {
			podList := &corev1.PodList{}
			err := suite.Clientmgr.k8sClient.List(suite.Ctx, podList,
				client.InNamespace(dspoNamespace),
				client.MatchingLabels{
					"app": webhookName,
				},
			)
			if err != nil {
				t.Logf("Error listing webhook pod: %v", err)
				return false
			}

			if len(podList.Items) == 0 {
				t.Log("No webhook pod found in DSPO namespace")
				return false
			}

			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodRunning {
					t.Logf("Webhook pod %s is not running (status: %s)", pod.Name, pod.Status.Phase)
					return false
				}
			}

			return true
		}, timeout, interval)
	})
}
