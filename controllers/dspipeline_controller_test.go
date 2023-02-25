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
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	dspcrcase2                  = "./testdata/deploy/case_2/cr.yaml"
	dspcrcase5                  = "./testdata/deploy/case_5/cr.yaml"
	apiserverDeployment         = "./testdata/results/case_2/apiserver/deployment.yaml"
	apiserverConfigMap1         = "./testdata/results/case_2/apiserver/configmap_artifact_script.yaml"
	apiserverConfigMap2         = "./testdata/results/case_5/apiserver/configmap_artifact_script.yaml"
	mariadbDeployment           = "./testdata/results/case_2/mariadb/deployment.yaml"
	minioDeployment             = "./testdata/results/case_2/minio/deployment.yaml"
	mlpipelinesUIDeployment     = "./testdata/results/case_2/mlpipelines-ui/deployment.yaml"
	persistenceAgentDeployment  = "./testdata/results/case_2/persistence-agent/deployment.yaml"
	scheduledWorkflowDeployment = "./testdata/results/case_2/scheduled-workflow/deployment.yaml"
	viewerCrdDeployment         = "./testdata/results/case_2/viewer-crd/deployment.yaml"
)

func deployDSP(ctx context.Context, path string, opts mf.Option) {
	dsp := &dspipelinesiov1alpha1.DSPipeline{}
	err := convertToStructuredResource(path, dsp, opts)
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Create(ctx, dsp)).Should(Succeed())
}

func compareDeployments(path string, opts mf.Option) {
	expectedDeployment := &appsv1.Deployment{}
	Expect(convertToStructuredResource(path, expectedDeployment, opts)).NotTo(HaveOccurred())

	actualDeployment := &appsv1.Deployment{}
	Eventually(func() error {
		namespacedNamed := types.NamespacedName{Name: expectedDeployment.Name, Namespace: WorkingNamespace}
		return k8sClient.Get(ctx, namespacedNamed, actualDeployment)
	}, timeout, interval).ShouldNot(HaveOccurred())

	Expect(testutil.DeploymentsAreEqual(*expectedDeployment, *actualDeployment)).Should(BeTrue())

}

func compareConfigMaps(path string, opts mf.Option) {
	expectedConfigMap := &v1.ConfigMap{}
	Expect(convertToStructuredResource(path, expectedConfigMap, opts)).NotTo(HaveOccurred())

	actualConfigMap := &v1.ConfigMap{}
	Eventually(func() error {
		namespacedNamed := types.NamespacedName{Name: expectedConfigMap.Name, Namespace: WorkingNamespace}
		return k8sClient.Get(ctx, namespacedNamed, actualConfigMap)
	}, timeout, interval).ShouldNot(HaveOccurred())

	Expect(testutil.ConfigMapsAreEqual(*expectedConfigMap, *actualConfigMap)).Should(BeTrue())

}

func configMapDoesNotExists(path string, opts mf.Option) {
	expectedConfigMap := &v1.ConfigMap{}
	actualConfigMap := &v1.ConfigMap{}
	Expect(convertToStructuredResource(path, expectedConfigMap, opts)).NotTo(HaveOccurred())

	namespacedNamed := types.NamespacedName{
		Name:      expectedConfigMap.Name,
		Namespace: WorkingNamespace,
	}

	Eventually(func() error {
		return k8sClient.Get(ctx, namespacedNamed, actualConfigMap)
	}, timeout, interval).Should(HaveOccurred())

	Expect(actualConfigMap).To(Equal(&v1.ConfigMap{}))

}

var _ = Describe("The DS Pipeline Controller", func() {
	client := mfc.NewClient(k8sClient)
	opts := mf.UseClient(client)
	ctx := context.Background()
	Context("In a namespace, when a DSP CR is deployed", func() {

		It("Should create an api server deployment", func() {
			deployDSP(ctx, dspcrcase2, opts)
			By("Creating apiserverDeployment UI resources")
			compareDeployments(apiserverDeployment, opts)
			By("Creating default artifact ConfigMap UI resources")
			compareConfigMaps(apiserverConfigMap1, opts)
			By("Creating mlpipelinesUIDeployment UI resources")
			compareDeployments(mlpipelinesUIDeployment, opts)
			By("Creating mariadbDeployment UI resources")
			compareDeployments(mariadbDeployment, opts)
			By("Creating minioDeployment UI resources")
			compareDeployments(minioDeployment, opts)
			By("Creating persistenceAgentDeployment UI resources")
			compareDeployments(persistenceAgentDeployment, opts)
			By("Creating scheduledWorkflowDeployment UI resources")
			compareDeployments(scheduledWorkflowDeployment, opts)
			By("Creating viewerCrdDeployment UI resources")
			compareDeployments(viewerCrdDeployment, opts)
		})
	})

	Context("In a namespace, when a DSP CR with custom Artifact ConfigMap is deployed", func() {
		It("Should report error if specified configmap does not exist.", func() {
			deployDSP(ctx, dspcrcase5, opts)
			configMapDoesNotExists(apiserverConfigMap2, opts)
		})
	})

})
