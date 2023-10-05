package systemtests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Deploying Pipeline", func() {

	It("should be easy", func() {
		var err error
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient).NotTo(BeNil())

		podList := &v1.PodList{}
		//ns := types.NamespacedName{Namespace: "openshift-pipelines"}

		listOpts := []client.ListOption{
			client.InNamespace("openshift-pipelines"),
		}

		err = k8sClient.List(ctx, podList, listOpts...)
		for _, pod := range podList.Items {
			print("hello1")
			loggr.Info("Pod: " + pod.Name)
		}

	})

})
