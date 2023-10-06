package systemsTesttUtil

import (
	"context"
	"fmt"
	mf "github.com/manifestival/manifestival"
	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// TODO:
// TODO:
// TODO:
// TODO: Flag these

// DeployDSPA will deploy resource found in path by requesting
func DeployDSPA(ctx context.Context, client client.Client, deployDSPA *v1alpha1.DataSciencePipelinesApplication, dspaNS string, timeout, interval time.Duration) {
	deployDSPA.ObjectMeta.Namespace = dspaNS
	err := client.Create(ctx, deployDSPA)
	Expect(err).ToNot(HaveOccurred())

	nsn := types.NamespacedName{
		Name:      deployDSPA.ObjectMeta.Name,
		Namespace: dspaNS,
	}
	fetchedDspa := &v1alpha1.DataSciencePipelinesApplication{}
	Eventually(func() error {
		err := client.Get(ctx, nsn, fetchedDspa)
		return err
	}, timeout, interval).ShouldNot(HaveOccurred())
}

// WaitForDSPAReady will assert for DSPA CR Ready Status
func WaitForDSPAReady(ctx context.Context, client client.Client, dspaName, dspaNS string, timeout, interval time.Duration) {
	nsn := types.NamespacedName{
		Name:      dspaName,
		Namespace: dspaNS,
	}
	dspa := &v1alpha1.DataSciencePipelinesApplication{}
	Eventually(func() metav1.ConditionStatus {
		err := client.Get(ctx, nsn, dspa)
		Expect(err).ToNot(HaveOccurred())
		for _, condition := range dspa.Status.Conditions {
			if condition.Type == "Ready" {
				return condition.Status
			}
		}
		return metav1.ConditionFalse
	}, timeout, interval).Should(Equal(metav1.ConditionTrue))
}

// DeleteDSPA will delete DSPA found in path by requesting
func DeleteDSPA(ctx context.Context, client client.Client, dspaName, dspaNS string, timeout, interval time.Duration) {
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
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() error {
		err := client.Get(ctx, nsn, dspa)
		if err != nil {
			if apierrs.IsNotFound(err) {
				return nil
			}
			return err
		}
		return fmt.Errorf("resource still exists on cluster")

	}, timeout, interval).ShouldNot(HaveOccurred())

}

func TestForSuccessfulDeployment(ctx context.Context, namespace, deploymentName string, client client.Client) {
	deployment := &appsv1.Deployment{}
	nsn := types.NamespacedName{
		Name:      deploymentName,
		Namespace: namespace,
	}
	err := client.Get(ctx, nsn, deployment)
	Expect(err).ToNot(HaveOccurred())
	deploymentAvailable := false
	for _, condition := range deployment.Status.Conditions {
		if condition.Reason == "MinimumReplicasAvailable" && condition.Type == appsv1.DeploymentAvailable {
			deploymentAvailable = true
		}
	}
	Expect(deploymentAvailable).To(BeTrue())

}

func GetDSPAFromPath(opts mf.Option, path string) *v1alpha1.DataSciencePipelinesApplication {
	dspa := &v1alpha1.DataSciencePipelinesApplication{}
	manifest, err := mf.NewManifest(path, opts)
	Expect(err).NotTo(HaveOccurred())
	expected := &manifest.Resources()[0]
	err = scheme.Scheme.Convert(expected, dspa, nil)
	Expect(err).NotTo(HaveOccurred())
	return dspa
}
