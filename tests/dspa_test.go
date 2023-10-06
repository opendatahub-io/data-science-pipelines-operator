package systemtests

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	systemsTesttUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("A successfully deployed DSPA", func() {

	podCount := 5

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
		It(fmt.Sprintf("should have a ready %s deployment", "MariaDB"), func() {
			systemsTesttUtil.TestForSuccessfulDeployment(ctx, DSPANamespace, fmt.Sprintf("mariadb-%s", DSPA.Name), clientmgr.k8sClient)
		})
		It(fmt.Sprintf("should have a ready %s deployment", "Minio"), func() {
			systemsTesttUtil.TestForSuccessfulDeployment(ctx, DSPANamespace, fmt.Sprintf("minio-%s", DSPA.Name), clientmgr.k8sClient)
		})

	})

})
