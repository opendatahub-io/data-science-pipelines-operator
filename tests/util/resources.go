package systemsTesttUtil

import (
	"context"
	"fmt"
	mf "github.com/manifestival/manifestival"
	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
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
const (
	DeployTimeout  = time.Second * 240
	DeployInterval = time.Millisecond * 2
	DeleteTimeout  = time.Second * 120
)

// DeployDSPA will deploy resource found in path by requesting
func DeployDSPA(ctx context.Context, client client.Client, deployDSPA *v1alpha1.DataSciencePipelinesApplication, dspaNS string) {
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
	}, DeployTimeout, DeployInterval).ShouldNot(HaveOccurred())
}

// WaitForDSPAReady will assert for DSPA CR Ready Status
func WaitForDSPAReady(ctx context.Context, client client.Client, dspaName, dspaNS string) {
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
	}, DeployTimeout, DeployInterval).Should(Equal(metav1.ConditionTrue))
}

// DeleteDSPA will delete DSPA found in path by requesting
func DeleteDSPA(ctx context.Context, client client.Client, dspaName, dspaNS string) {
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

	}, DeleteTimeout, DeployInterval).ShouldNot(HaveOccurred())

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
