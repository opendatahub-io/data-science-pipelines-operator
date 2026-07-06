//go:build test_integration

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

package integration

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"testing"
	"time"

	routev1 "github.com/openshift/api/route/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"net/http"

	"github.com/anthhub/forwarder"
	"github.com/go-logr/logr"
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	testUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	loggr     logr.Logger
	ctx       context.Context
	cfg       *rest.Config
	cancel    context.CancelFunc
	clientmgr ClientManager

	APIServerURL                            string
	kubeconfig                              string
	k8sApiServerHost                        string
	DSPAPath                                string
	DSPANamespace                           string
	MinioNamespace                          string
	ArgoWorkflowsControllersManagementState string
	skipDeploy                              bool
	skipCleanup                             bool
	PortforwardLocalPort                    int
	DSPA                                    *dspav1.DataSciencePipelinesApplication
	forwarderResult                         *forwarder.Result
	endpointType                            string
)

var (
	DeployTimeout time.Duration
	PollInterval  time.Duration
	DeleteTimeout time.Duration
)

const (
	APIServerPort                                  = 8888
	DefaultKubeConfigPath                          = "~/.kube/config"
	Defaultk8sApiServerHost                        = "localhost:6443"
	DefaultDSPANamespace                           = "default"
	DefaultMinioNamespace                          = "default"
	DefaultArgoWorkflowsControllersManagementState = "Managed"
	DefaultDeployTimeout                           = 240
	DefaultPollInterval                            = 2
	DefaultDeleteTimeout                           = 120
	DefaultPortforwardLocalPort                    = 8888
	DefaultSkipDeploy                              = false
	DefaultSkipCleanup                             = false
	DefaultDSPAPath                                = ""
	DefaultEndpointType                            = "service"
)

type ClientManager struct {
	k8sClient  client.Client
	mfsClient  mf.Client
	mfopts     mf.Option
	httpClient http.Client
}

type IntegrationTestSuite struct {
	suite.Suite
	Clientmgr                               ClientManager
	Ctx                                     context.Context
	DSPANamespace                           string
	MinioNamespace                          string
	ArgoWorkflowsControllersManagementState string
	DSPA                                    *dspav1.DataSciencePipelinesApplication
}

type testLogWriter struct {
	t *testing.T
}

type customTransport struct {
	Transport http.RoundTripper
	Token     string
}

func (w *testLogWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}

// newTestLogWriter creates a new instance of testLogWriter
// that adapts *testing.T to an io.Writer.
func newTestLogWriter(t *testing.T) *testLogWriter {
	return &testLogWriter{t: t}
}

// Register flags in an init function. This ensures they are registered _before_ `go test` calls flag.Parse()
func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", DefaultKubeConfigPath, "The path to the kubeconfig.")
	flag.StringVar(&k8sApiServerHost, "k8sApiServerHost", Defaultk8sApiServerHost, "The k8s cluster api server host.")
	flag.StringVar(&DSPAPath, "DSPAPath", DefaultDSPAPath, "The DSP resource file to deploy for testing.")
	flag.StringVar(&DSPANamespace, "DSPANamespace", DefaultDSPANamespace, "The namespace to deploy DSPA.")
	flag.StringVar(&MinioNamespace, "MinioNamespace", DefaultMinioNamespace, "The namespace where MinIO is deployed.")
	flag.StringVar(&ArgoWorkflowsControllersManagementState, "ArgoWorkflowsControllersManagementState", DefaultArgoWorkflowsControllersManagementState, "The global management state of the DSPA-owned Argo WorkflowsControllers. Options: 'Managed' or 'Removed'.")

	flag.DurationVar(&DeployTimeout, "DeployTimeout", DefaultDeployTimeout, "Seconds to wait for deployments. Consider increasing this on resource starved environments.")
	DeployTimeout *= time.Second
	flag.DurationVar(&PollInterval, "PollInterval", DefaultPollInterval, "Seconds to wait before retrying fetches to the api server.")
	PollInterval *= time.Second
	flag.DurationVar(&DeleteTimeout, "DeleteTimeout", DefaultDeleteTimeout, "Seconds to wait for deployment deletions. Consider increasing this on resource starved environments.")
	DeleteTimeout *= time.Second
	flag.IntVar(&PortforwardLocalPort, "PortforwardLocalPort", DefaultPortforwardLocalPort, "Local port to use for port forwarding dspa server.")

	flag.BoolVar(&skipDeploy, "skipDeploy", DefaultSkipDeploy, "Skip DSPA deployment. Use this if you have already "+
		"manually deployed a DSPA, and want to skip this part.")
	flag.BoolVar(&skipCleanup, "skipCleanup", DefaultSkipCleanup, "Skip DSPA cleanup.")

	flag.StringVar(&endpointType, "endpointType", DefaultEndpointType, "Specify how the test suite will interact with DSPO. Options: 'service' for a Kubernetes service or 'route' for an OpenShift route.")
}

func (suite *IntegrationTestSuite) SetupSuite() {
	loggr = logf.Log
	ctx, cancel = context.WithCancel(context.Background())
	suite.Ctx = ctx

	// Initialize logger
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.TimeEncoderOfLayout(time.RFC3339),
	}
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(newTestLogWriter(suite.T())), zap.UseFlagOptions(&opts)))

	var err error

	utilruntime.Must(dspav1.AddToScheme(scheme.Scheme))
	clientmgr = ClientManager{}

	cfg, err = clientcmd.BuildConfigFromFlags(k8sApiServerHost, kubeconfig)
	suite.Require().NoError(err)

	clientmgr.k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	suite.Require().NoError(err)
	suite.Require().NotNil(clientmgr.k8sClient)
	clientmgr.mfsClient = mfc.NewClient(clientmgr.k8sClient)
	clientmgr.mfopts = mf.UseClient(clientmgr.mfsClient)
	clientmgr.httpClient = http.Client{}
	suite.Clientmgr = clientmgr

	// Register API objects
	utilruntime.Must(routev1.AddToScheme(scheme.Scheme))

	DSPA = testUtil.GetDSPAFromPath(suite.T(), clientmgr.mfopts, DSPAPath)

	suite.DSPANamespace = DSPANamespace
	suite.MinioNamespace = MinioNamespace
	suite.ArgoWorkflowsControllersManagementState = ArgoWorkflowsControllersManagementState
	suite.DSPA = DSPA

	if !skipDeploy {
		loggr.Info("Deploying DSPA...")
		err = testUtil.DeployDSPA(suite.T(), ctx, clientmgr.k8sClient, DSPA, DSPANamespace, DeployTimeout, PollInterval)
		require.NoError(suite.T(), err)
		loggr.Info("Waiting for DSPA pods to ready...")
	}

	err = testUtil.WaitForDSPAReady(suite.T(), ctx, clientmgr.k8sClient, DSPA.Name, DSPANamespace, DeployTimeout, PollInterval)
	require.NoError(suite.T(), err, fmt.Sprintf("Error waiting for DSPA being ready:\n%s", testUtil.PrintConditions(ctx, DSPA, DSPANamespace, clientmgr.k8sClient)))
	loggr.Info("DSPA Deployed.")

	if endpointType == "service" {

		loggr.Info("Setting up Portforwarding service.")
		options := []*forwarder.Option{
			{
				LocalPort:   PortforwardLocalPort,
				RemotePort:  APIServerPort,
				ServiceName: fmt.Sprintf("ds-pipeline-%s", DSPA.Name),
				Namespace:   DSPANamespace,
			},
		}

		forwarderResult, err = forwarder.WithForwarders(ctx, options, kubeconfig)
		suite.Require().NoError(err)
		_, err = forwarderResult.Ready()
		suite.Require().NoError(err)

		APIServerURL = fmt.Sprintf("http://127.0.0.1:%d", PortforwardLocalPort)

		loggr.Info(fmt.Sprintf("Port forwarding service Successfully set up: %s", APIServerURL))

	} else if endpointType == "route" {

		route, err := testUtil.GetDSPARoute(clientmgr.k8sClient, DSPANamespace, suite.DSPA.Name)
		if err != nil {
			log.Fatal(err, "failed to retrieve route")
		}

		APIServerURL = fmt.Sprintf("https://%s", route.Status.Ingress[0].Host)

		loggr.Info(fmt.Sprintf("The Api Server URL was retrieved from the Route: %s", APIServerURL))

		suite.Clientmgr.httpClient.Transport = &customTransport{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Token: cfg.BearerToken,
		}

		// waiting for pods to sit down
		err = testUtil.WaitFor(ctx, DeployTimeout, PollInterval, func() (bool, error) {
			response, err := suite.Clientmgr.httpClient.Get(fmt.Sprintf("%s/apis/v2beta1/healthz", APIServerURL))
			if response.StatusCode != 200 {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			log.Fatal(err, "healthz endpoint is not working properly")
		}

	} else {
		log.Fatal(fmt.Sprintf("Invalid endpointType. Supported service or route. Provided: %s", endpointType))
	}
}

func (suite *IntegrationTestSuite) TearDownSuite() {
	if !skipCleanup {
		err := testUtil.DeleteDSPA(suite.T(), ctx, clientmgr.k8sClient, DSPA.Name, DSPANamespace, DeployTimeout, PollInterval)
		assert.NoError(suite.T(), err)
	}
	if forwarderResult != nil {
		forwarderResult.Close()
	}
	cancel()
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (t *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add the Authorization header globally
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.Token))

	// Proceed with the default RoundTripper (http.DefaultTransport)
	return t.Transport.RoundTrip(req)
}
