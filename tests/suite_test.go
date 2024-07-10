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
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anthhub/forwarder"
	"github.com/go-logr/logr"
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	"github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	testUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/types"
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
	DeployTimeout time.Duration
	PollInterval  time.Duration
	DeleteTimeout time.Duration
)

const (
	APIServerPort               = 8888
	DefaultKubeConfigPath       = "~/.kube/config"
	Defaultk8sApiServerHost     = "localhost:6443"
	DefaultDSPANamespace        = "default"
	DefaultDeployTimeout        = 240
	DefaultPollInterval         = 2
	DefaultDeleteTimeout        = 120
	DefaultPortforwardLocalPort = 8888
	DefaultSkipDeploy           = false
	DefaultSkipCleanup          = false
	DefaultDSPAPath             = ""
)

type ClientManager struct {
	k8sClient client.Client
	mfsClient mf.Client
	mfopts    mf.Option
}

type IntegrationTestSuite struct {
	suite.Suite
	Clientmgr     ClientManager
	Ctx           context.Context
	DSPANamespace string
	DSPA          *dspav1alpha1.DataSciencePipelinesApplication
}

type testLogWriter struct {
	t *testing.T
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

	utilruntime.Must(dspav1alpha1.AddToScheme(scheme.Scheme))
	clientmgr = ClientManager{}

	cfg, err = clientcmd.BuildConfigFromFlags(k8sApiServerHost, kubeconfig)
	suite.Require().NoError(err)

	clientmgr.k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	suite.Require().NoError(err)
	suite.Require().NotNil(clientmgr.k8sClient)
	clientmgr.mfsClient = mfc.NewClient(clientmgr.k8sClient)
	clientmgr.mfopts = mf.UseClient(clientmgr.mfsClient)
	suite.Clientmgr = clientmgr

	DSPA = testUtil.GetDSPAFromPath(suite.T(), clientmgr.mfopts, DSPAPath)

	suite.DSPANamespace = DSPANamespace
	suite.DSPA = DSPA

	if !skipDeploy {
		loggr.Info("Deploying DSPA...")
		err = testUtil.DeployDSPA(suite.T(), ctx, clientmgr.k8sClient, DSPA, DSPANamespace, DeployTimeout, PollInterval)
		require.NoError(suite.T(), err)
		loggr.Info("Waiting for DSPA pods to ready...")
	}

	err = testUtil.WaitForDSPAReady(suite.T(), ctx, clientmgr.k8sClient, DSPA.Name, DSPANamespace, DeployTimeout, PollInterval)
	require.NoError(suite.T(), err, fmt.Sprintf("Error Deploying DSPA:\n%s", printConditions(ctx, DSPA, DSPANamespace, clientmgr.k8sClient)))
	loggr.Info("DSPA Deployed.")

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
	loggr.Info("Portforwarding service Successfully set up.")
}

func printConditions(ctx context.Context, dspa *v1alpha1.DataSciencePipelinesApplication, namespace string, client client.Client) string {
	nsn := types.NamespacedName{
		Name:      dspa.Name,
		Namespace: namespace,
	}
	err := client.Get(ctx, nsn, dspa)
	if err != nil {
		return "No conditions"
	}
	conditions := ""
	for _, condition := range dspa.Status.Conditions {
		conditions = conditions + fmt.Sprintf("Type: %s, Status: %s, Message: %s\n", condition.Type, condition.Status, condition.Message)
	}
	return conditions
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
