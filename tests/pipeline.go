package systemtests

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	cfg    *rest.Config
	ctx    context.Context
	cancel context.CancelFunc
)

var _ = Describe("Deploying Pipeline", func() {

	It("should be easy", func() {
		// Initialize Kubernetes client
		//cfg.BearerToken = "sha256~CvJwe4Iyrz_pr7-B3WYS9UOlv6vXwY71DiMsnAzNXes"
		var err error
		cfg, err = clientcmd.BuildConfigFromFlags("https://api.hukhan-3.dev.datahub.redhat.com:6443", "/home/hukhan/.kube/config")
		k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})

		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient).NotTo(BeNil())

		podList := &v1.PodList{}
		//ns := types.NamespacedName{Namespace: "openshift-pipelines"}

		listOpts := []client.ListOption{
			client.InNamespace("openshift-pipelines"),
		}

		err = k8sClient.List(ctx, podList, listOpts...)
		GinkgoWriter.Print("test")
		GinkgoLogr.Info("fdasfdsa")

		for _, pod := range podList.Items {
			print("hello1")
			GinkgoLogr.Info("Pod: " + pod.Name)
		}

	})

})
