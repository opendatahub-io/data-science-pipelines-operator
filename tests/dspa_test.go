package systemtests

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Deploying a DSPA", func() {

	podCount := 5

	Context("with default MariaDB and Minio", func() {
		It(fmt.Sprintf("should result in %d pods", podCount), func() {
			podList := &corev1.PodList{}
			listOpts := []client.ListOption{
				client.InNamespace(DSPANamespace),
			}
			err := clientmgr.k8sClient.List(ctx, podList, listOpts...)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(podList.Items)).Should(Equal(podCount))
		})

		It("should result in a ready DSP API Server deployment", func() {
			deployment := &appsv1.Deployment{}
			nsn := types.NamespacedName{
				Name:      fmt.Sprintf("ds-pipeline-%s", DSPA.Name),
				Namespace: DSPANamespace,
			}
			err := clientmgr.k8sClient.Get(ctx, nsn, deployment)
			Expect(err).ToNot(HaveOccurred())
			deploymentAvailable := false
			for _, condition := range deployment.Status.Conditions {
				if condition.Reason == "MinimumReplicasAvailable" && condition.Type == appsv1.DeploymentAvailable {
					deploymentAvailable = true
				}
			}
			Expect(deploymentAvailable).To(BeTrue())
		})
	})

})
