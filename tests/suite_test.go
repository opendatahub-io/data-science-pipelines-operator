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

package systemtests

import (
	"context"
	"flag"
	"fmt"
	"github.com/anthhub/forwarder"
	"github.com/go-logr/logr"
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	systemsTesttUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
	"time"
)

const (
	APIServerPort = 8888
)

var (
	loggr     logr.Logger
	ctx       context.Context
	cfg       *rest.Config
	cancel    context.CancelFunc
	clientmgr ClientManager

	APIServerURL         string
	kubeconfig           string
	k8sApiServerHost     string
	DSPAPath             string
	DSPANamespace        string
	skipDeploy           bool
	skipCleanup          bool
	PortforwardLocalPort int
	DSPA                 *dspav1alpha1.DataSciencePipelinesApplication
	forwarderResult      *forwarder.Result
)

var (
	DefaultDeployTimeout time.Duration
	DefaultPollInterval  time.Duration
	DefaultDeleteTimeout time.Duration
)

type ClientManager struct {
	k8sClient client.Client
	mfsClient mf.Client
	mfopts    mf.Option
}

// TestAPIs - This is the entry point for Ginkgo -
// the go test runner will run this function when you run go test or ginkgo.
// Under the hood, Ginkgo is simply calling go test.
// You can run go test instead of the ginkgo CLI, But Ginkgo has several capabilities that can only be accessed via ginkgo.
// It is best practice to embrace the ginkgo CLI and treat it as a first-class member of the testing toolchain.
func TestAPIs(t *testing.T) {
	// Single line of glue code connecting Ginkgo to Gomega
	// Inform our matcher library (Gomega) which function to call (Ginkgo's Fail) in the event a failure is detected.
	RegisterFailHandler(Fail)

	// Inform Ginkgo to start the test suite, passing it the *testing.T instance and a description of the suite.
	// Only call RunSpecs once and let Ginkgo worry about calling *testing.T for us.
	RunSpecs(t, "Controller Suite")
}

// Register flags in an init function. This ensures they are registered _before_ `go test` calls flag.Parse()
func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "~/.kube/config", "The path to the kubeconfig.")
	flag.StringVar(&k8sApiServerHost, "k8sApiServerHost", "localhost:6443", "The k8s cluster api server host.")
	flag.StringVar(&DSPAPath, "DSPAPath", "", "The DSP resource file to deploy for testing.")
	flag.StringVar(&DSPANamespace, "DSPANamespace", "default", "The namespace to deploy DSPA.")

	flag.DurationVar(&DefaultDeployTimeout, "DefaultDeployTimeout", 240, "Seconds to wait for deployments. Consider increasing this on resource starved environments.")
	DefaultDeployTimeout *= time.Second
	flag.DurationVar(&DefaultPollInterval, "DefaultPollInterval", 2, "Seconds to wait before retrying fetches to the api server.")
	DefaultPollInterval *= time.Second
	flag.DurationVar(&DefaultDeleteTimeout, "DefaultDeleteTimeout", 120, "Seconds to wait for deployment deletions. Consider increasing this on resource starved environments.")
	DefaultDeleteTimeout *= time.Second

	flag.IntVar(&PortforwardLocalPort, "PortforwardLocalPort", 8888, "Local port to use for port forwarding dspa server.")

	flag.BoolVar(&skipDeploy, "skipDeploy", false, "Skip DSPA deployment. Use this if you have already "+
		"manually deployed a DSPA, and want to skip this part.")
	flag.BoolVar(&skipCleanup, "skipCleanup", false, "Skip DSPA cleanup.")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())

	// Initialize logger
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.TimeEncoderOfLayout(time.RFC3339),
	}
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseFlagOptions(&opts)))

	loggr = logf.Log
	var err error

	// Register API objects
	utilruntime.Must(dspav1alpha1.AddToScheme(scheme.Scheme))

	clientmgr = ClientManager{}

	// Set up client auth configs
	cfg, err = clientcmd.BuildConfigFromFlags(k8sApiServerHost, kubeconfig)
	Expect(err).ToNot(HaveOccurred())

	// Initialize Kubernetes client
	clientmgr.k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clientmgr.k8sClient).NotTo(BeNil())
	clientmgr.mfsClient = mfc.NewClient(clientmgr.k8sClient)
	clientmgr.mfopts = mf.UseClient(clientmgr.mfsClient)

	// Get DSPA structured
	DSPA = systemsTesttUtil.GetDSPAFromPath(clientmgr.mfopts, DSPAPath)

	if !skipDeploy {
		loggr.Info("Deploying DSPA...")
		systemsTesttUtil.DeployDSPA(ctx, clientmgr.k8sClient, DSPA, DSPANamespace, DefaultDeployTimeout, DefaultPollInterval)
		loggr.Info("Waiting for DSPA pods to ready...")
		systemsTesttUtil.WaitForDSPAReady(ctx, clientmgr.k8sClient, DSPA.Name, DSPANamespace, DefaultDeployTimeout, DefaultPollInterval)
		loggr.Info("DSPA Deployed.")
	}

	// Forward ApiServer Service
	loggr.Info("Setting up Portforwarding service.")
	options := []*forwarder.Option{
		{
			// the local port for forwarding
			LocalPort: PortforwardLocalPort,
			// the k8s pod port
			RemotePort: APIServerPort,
			// the forwarding service name
			ServiceName: fmt.Sprintf("ds-pipeline-%s", DSPA.Name),
			// namespace default is "default"
			Namespace: DSPANamespace,
		},
	}
	// create a forwarder, and you need provide a path of kubeconfig
	forwarderResult, err = forwarder.WithForwarders(context.Background(), options, kubeconfig)
	Expect(err).NotTo(HaveOccurred())
	// wait forwarding ready
	_, err = forwarderResult.Ready()
	Expect(err).NotTo(HaveOccurred())
	loggr.Info("Portforwarding service Successfully set up.")

	APIServerURL = fmt.Sprintf("http://127.0.0.1:%d", PortforwardLocalPort)

	loggr.Info("Starting Test Suite.")
})

var _ = BeforeEach(func() {
})

var _ = AfterSuite(func() {
	if !skipCleanup {
		systemsTesttUtil.DeleteDSPA(ctx, clientmgr.k8sClient, DSPA.Name, DSPANamespace, DefaultDeployTimeout, DefaultPollInterval)
	}
	forwarderResult.Close()
})
