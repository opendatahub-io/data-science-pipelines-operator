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
	"errors"
	"fmt"
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
)

type TestCase struct {
	Description string
	Path        string
}

type CaseComponentResources map[string]ResourcePath
type ResourcePath map[string]string

var cases = map[string]TestCase{
	"case0": {
		Description: "empty CR Spec",
		Path:        "./testdata/deploy/case_0/cr.yaml",
	},
	"case1": {
		Description: "all Deploy fields are set to false",
		Path:        "./testdata/deploy/case_1/cr.yaml",
	},
	"case2": {
		Description: "standard CR Spec with components specified",
		Path:        "./testdata/deploy/case_2/cr.yaml",
	},
	"case3": {
		Description: "custom Artifact configmap is provided",
		Path:        "./testdata/deploy/case_3/cr.yaml",
	},
}
var deploymentsCreated = CaseComponentResources{
	"case0": {
		"apiserver":                   "./testdata/results/case_0/apiserver/deployment.yaml",
		"mariadb":                     "./testdata/results/case_0/mariadb/deployment.yaml",
		"minioDeployment":             "./testdata/results/case_0/minio/deployment.yaml",
		"mlpipelinesUIDeployment":     "./testdata/results/case_0/mlpipelines-ui/deployment.yaml",
		"persistenceAgentDeployment":  "./testdata/results/case_0/persistence-agent/deployment.yaml",
		"scheduledWorkflowDeployment": "./testdata/results/case_0/scheduled-workflow/deployment.yaml",
	},
	"case2": {
		"apiserver":                   "./testdata/results/case_2/apiserver/deployment.yaml",
		"mariadb":                     "./testdata/results/case_2/mariadb/deployment.yaml",
		"minioDeployment":             "./testdata/results/case_2/minio/deployment.yaml",
		"mlpipelinesUIDeployment":     "./testdata/results/case_2/mlpipelines-ui/deployment.yaml",
		"persistenceAgentDeployment":  "./testdata/results/case_2/persistence-agent/deployment.yaml",
		"scheduledWorkflowDeployment": "./testdata/results/case_2/scheduled-workflow/deployment.yaml",
		"viewerCrdDeployment":         "./testdata/results/case_2/viewer-crd/deployment.yaml",
	},
}

var deploymentsNotCreated = CaseComponentResources{
	"case0": {
		"viewerCrdDeployment": "./testdata/results/case_0/viewer-crd/deployment.yaml",
	},
	"case1": {
		"apiserver":                   "./testdata/results/case_1/apiserver/deployment.yaml",
		"mariadb":                     "./testdata/results/case_1/mariadb/deployment.yaml",
		"minioDeployment":             "./testdata/results/case_1/minio/deployment.yaml",
		"mlpipelinesUIDeployment":     "./testdata/results/case_1/mlpipelines-ui/deployment.yaml",
		"persistenceAgentDeployment":  "./testdata/results/case_1/persistence-agent/deployment.yaml",
		"scheduledWorkflowDeployment": "./testdata/results/case_1/scheduled-workflow/deployment.yaml",
	},
}

var configMapsCreated = CaseComponentResources{
	"case0": {
		"apiserver": "./testdata/results/case_0/apiserver/configmap_artifact_script.yaml",
	},
	"case2": {
		"apiserver": "./testdata/results/case_2/apiserver/configmap_artifact_script.yaml",
	},
}

var configMapsNotCreated = CaseComponentResources{
	"case3": {
		"apiserver": "./testdata/results/case_3/apiserver/configmap_artifact_script.yaml",
	},
}

func deployDSP(path string, opts mf.Option) {
	dsp := &dspav1alpha1.DataSciencePipelinesApplication{}
	err := convertToStructuredResource(path, dsp, opts)
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Create(ctx, dsp)).Should(Succeed())

	dsp2 := &dspav1alpha1.DataSciencePipelinesApplication{}
	Eventually(func() error {
		namespacedNamed := types.NamespacedName{Name: dsp.Name, Namespace: WorkingNamespace}
		return k8sClient.Get(ctx, namespacedNamed, dsp2)
	}, timeout, interval).ShouldNot(HaveOccurred())
}

func deleteDSP(path string, opts mf.Option) {
	dsp := &dspav1alpha1.DataSciencePipelinesApplication{}
	err := convertToStructuredResource(path, dsp, opts)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		return k8sClient.Delete(ctx, dsp)
	}, timeout, interval).ShouldNot(HaveOccurred())

	dsp2 := &dspav1alpha1.DataSciencePipelinesApplication{}
	Eventually(func() error {
		namespacedNamed := types.NamespacedName{Name: dsp.Name, Namespace: WorkingNamespace}
		Expect(k8sClient.Get(ctx, namespacedNamed, dsp2)).NotTo(HaveOccurred())
		if !reflect.DeepEqual(dsp2, &dspav1alpha1.DataSciencePipelinesApplication{}) {
			return errors.New("DSP still exists on cluster")
		}
		return nil
	}, timeout, interval).Should(HaveOccurred())

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

func deploymentDoesNotExists(path string, opts mf.Option) {
	expectedDeployment := &appsv1.Deployment{}
	actualDeployment := &appsv1.Deployment{}
	Expect(convertToStructuredResource(path, expectedDeployment, opts)).NotTo(HaveOccurred())

	namespacedNamed := types.NamespacedName{
		Name:      expectedDeployment.Name,
		Namespace: WorkingNamespace,
	}

	Eventually(func() error {
		return k8sClient.Get(ctx, namespacedNamed, actualDeployment)
	}, timeout, interval).Should(HaveOccurred())

	Expect(actualDeployment).To(Equal(&appsv1.Deployment{}))

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

	for testcase := range cases {
		testcase := testcase
		description := cases[testcase].Description
		dspPath := cases[testcase].Path
		expectedDeployments := deploymentsCreated[testcase]
		Context(description, func() {
			It(fmt.Sprintf("Should successfully deploy the Custom Resource for case %s", testcase), func() {
				deployDSP(dspPath, opts)
			})
			for component := range expectedDeployments {
				component := component
				deploymentPath := expectedDeployments[component]
				It(fmt.Sprintf("Should create deployment for component %s", deploymentsCreated[testcase][component]), func() {
					compareDeployments(deploymentPath, opts)
				})
			}

			for component := range deploymentsNotCreated[testcase] {
				It(fmt.Sprintf("Should NOT create deployments for component %s", component), func() {
					deploymentDoesNotExists(deploymentsNotCreated[testcase][component], opts)
				})
			}

			for component := range configMapsCreated[testcase] {
				It(fmt.Sprintf("Should create configmaps for component %s", component), func() {
					compareConfigMaps(configMapsCreated[testcase][component], opts)
				})
			}
			for component := range configMapsNotCreated[testcase] {
				It(fmt.Sprintf("Should NOT create configmaps for component %s", component), func() {
					configMapDoesNotExists(configMapsNotCreated[testcase][component], opts)
				})
			}

			It(fmt.Sprintf("Should successfully delete the Custom Resource for case %s", testcase), func() {
				deleteDSP(dspPath, opts)
			})
		})
	}
})
