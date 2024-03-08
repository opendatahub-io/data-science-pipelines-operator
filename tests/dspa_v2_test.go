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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	systemsTesttUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("A successfully deployed DSPA", func() {

	podCount := 6

	var isTestDspa bool

	BeforeEach(func() {
		isTestDspa = DSPA.Name == "test-dspa"
		if isTestDspa {
			podCount = 8
		}
	})

	Context("with default MariaDB and Minio", func() {
		It(fmt.Sprintf("should have %d pods", podCount), func() {
			podList := &corev1.PodList{}
			listOpts := []client.ListOption{
				client.InNamespace(DSPANamespace),
			}
			err := clientmgr.k8sClient.List(ctx, podList, listOpts...)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(podList.Items)).Should(Equal(podCount))
		})

		It(fmt.Sprintf("should have a ready %s deployment", "DSP API Server"), func() {
			systemsTesttUtil.TestForSuccessfulDeployment(ctx, DSPANamespace, fmt.Sprintf("ds-pipeline-%s", DSPA.Name), clientmgr.k8sClient)
		})
		It(fmt.Sprintf("should have a ready %s deployment", "Persistence Agent"), func() {
			systemsTesttUtil.TestForSuccessfulDeployment(ctx, DSPANamespace, fmt.Sprintf("ds-pipeline-persistenceagent-%s", DSPA.Name), clientmgr.k8sClient)
		})
		It(fmt.Sprintf("should have a ready %s deployment", "Scheduled Workflow"), func() {
			systemsTesttUtil.TestForSuccessfulDeployment(ctx, DSPANamespace, fmt.Sprintf("ds-pipeline-scheduledworkflow-%s", DSPA.Name), clientmgr.k8sClient)
		})
		if isTestDspa {
			It(fmt.Sprintf("should have a ready %s deployment", "MariaDB"), func() {
				systemsTesttUtil.TestForSuccessfulDeployment(ctx, DSPANamespace, fmt.Sprintf("mariadb-%s", DSPA.Name), clientmgr.k8sClient)
			})
		}
		if isTestDspa {
			It(fmt.Sprintf("should have a ready %s deployment", "Minio"), func() {
				systemsTesttUtil.TestForSuccessfulDeployment(ctx, DSPANamespace, fmt.Sprintf("minio-%s", DSPA.Name), clientmgr.k8sClient)
			})
		}
	})

})
