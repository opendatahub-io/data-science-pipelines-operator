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
	"fmt"
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type TestCase struct {
	Description string
	Path        string
}

type CaseComponentResources map[string]ResourcePath
type ResourcePath map[string]string

var cases = map[string]TestCase{
	"case_0": {
		Description: "empty CR Spec",
		Path:        "./testdata/deploy/case_0/cr.yaml",
	},
	"case_1": {
		Description: "all Deploy fields are set to false",
		Path:        "./testdata/deploy/case_1/cr.yaml",
	},
	"case_2": {
		Description: "standard CR Spec with components specified",
		Path:        "./testdata/deploy/case_2/cr.yaml",
	},
	"case_3": {
		Description: "custom Artifact configmap is provided, custom images override defaults",
		Path:        "./testdata/deploy/case_3/cr.yaml",
	},
}
var deploymentsCreated = CaseComponentResources{
	"case_0": {
		"apiserver":                   "./testdata/results/case_0/apiserver/deployment.yaml",
		"mariadb":                     "./testdata/results/case_0/mariadb/deployment.yaml",
		"persistenceAgentDeployment":  "./testdata/results/case_0/persistence-agent/deployment.yaml",
		"scheduledWorkflowDeployment": "./testdata/results/case_0/scheduled-workflow/deployment.yaml",
	},
	"case_2": {
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
	"case_0": {
		"viewerCrdDeployment": "./testdata/results/case_0/viewer-crd/deployment.yaml",
	},
	"case_1": {
		"apiserver":                   "./testdata/results/case_1/apiserver/deployment.yaml",
		"mariadb":                     "./testdata/results/case_1/mariadb/deployment.yaml",
		"minioDeployment":             "./testdata/results/case_1/minio/deployment.yaml",
		"mlpipelinesUIDeployment":     "./testdata/results/case_1/mlpipelines-ui/deployment.yaml",
		"persistenceAgentDeployment":  "./testdata/results/case_1/persistence-agent/deployment.yaml",
		"scheduledWorkflowDeployment": "./testdata/results/case_1/scheduled-workflow/deployment.yaml",
	},
}

var configMapsCreated = CaseComponentResources{
	"case_0": {
		"apiserver": "./testdata/results/case_0/apiserver/configmap_artifact_script.yaml",
	},
	"case_2": {
		"apiserver": "./testdata/results/case_2/apiserver/configmap_artifact_script.yaml",
	},
}

var configMapsNotCreated = CaseComponentResources{
	"case_3": {
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
		err = k8sClient.Get(ctx, namespacedNamed, dsp2)
		if err != nil {
			if apierrs.IsNotFound(err) {
				return nil
			}
			return err
		}
		return fmt.Errorf("resource still exists on cluster")

	}, timeout, interval).ShouldNot(HaveOccurred())

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

var _ = Describe("The DS Pipeline Controller", Ordered, func() {
	client := mfc.NewClient(k8sClient)
	opts := mf.UseClient(client)

	for tc := range cases {
		// We assign local copies of all looping variables, as they are mutating
		// we want the correct variables captured in each `It` closure, we do this
		// by creating local variables
		// https://onsi.github.io/ginkgo/#dynamically-generating-specs
		testcase := tc
		description := cases[testcase].Description
		dspPath := cases[testcase].Path

		Context(description, func() {
			It(fmt.Sprintf("Should successfully deploy the Custom Resource for case %s", testcase), func() {
				viper.New()
				viper.SetConfigFile(fmt.Sprintf("testdata/deploy/%s/config.yaml", testcase))
				err := viper.ReadInConfig()
				Expect(err).ToNot(HaveOccurred(), "Failed to read config file")
				deployDSP(dspPath, opts)
			})
			expectedDeployments := deploymentsCreated[testcase]
			for component := range expectedDeployments {
				component := component
				deploymentPath := expectedDeployments[component]
				It(fmt.Sprintf("[%s] Should create deployment for component %s", testcase, component), func() {
					compareDeployments(deploymentPath, opts)
				})
			}

			notExpectedDeployments := deploymentsNotCreated[testcase]
			for component := range deploymentsNotCreated[testcase] {
				deploymentPath := notExpectedDeployments[component]
				It(fmt.Sprintf("[%s] Should NOT create deployments for component %s", testcase, component), func() {
					deploymentDoesNotExists(deploymentPath, opts)
				})
			}

			for component := range configMapsCreated[testcase] {
				It(fmt.Sprintf("[%s] Should create configmaps for component %s", testcase, component), func() {
					compareConfigMaps(configMapsCreated[testcase][component], opts)
				})
			}
			for component := range configMapsNotCreated[testcase] {
				It(fmt.Sprintf("[%s] Should NOT create configmaps for component %s", testcase, component), func() {
					configMapDoesNotExists(configMapsNotCreated[testcase][component], opts)
				})
			}

			It(fmt.Sprintf("Should successfully delete the Custom Resource for case %s", testcase), func() {
				deleteDSP(dspPath, opts)
			})
		})
	}
})
