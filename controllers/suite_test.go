//go:build test_all || test_functional

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

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/onsi/gomega/format"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	//+kubebuilder:scaffold:imports
)

// These tests use Testify (A toolkit with common assertions and mocks). Refer to
// https://github.com/stretchr/testify to learn more about Testify.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

const (
	WorkingNamespace = "default"
	DSPCRName        = "testdsp"
	timeout          = time.Second * 6
	interval         = time.Millisecond * 2
)

type ControllerSuite struct {
	suite.Suite
}

func TestAPIs(t *testing.T) {
	testingSuite := new(ControllerSuite)
	suite.Run(t, testingSuite)
}

func (s *ControllerSuite) SetupTest() {
	logf.Log.Info("Overriding the Database and Object Store live connection functions with trivial stubs")
	ConnectAndQueryDatabase = func(
		host string,
		log logr.Logger,
		port, username, password, dbname, tls string,
		dbConnectionTimeout time.Duration,
		pemCerts [][]byte,
		extraParams map[string]string) (bool, error) {
		return true, nil
	}
	ConnectAndQueryObjStore = func(
		ctx context.Context,
		log logr.Logger,
		endpoint, bucket string,
		accesskey, secretkey []byte,
		secure bool,
		pemCerts [][]byte,
		objStoreConnectionTimeout time.Duration) (bool, error) {
		return true, nil
	}
}

func (s *ControllerSuite) SetupSuite() {
	ctx, cancel = context.WithCancel(context.TODO())

	format.MaxLength = 0

	// Initialize logger
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.TimeEncoderOfLayout(time.RFC3339),
	}
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseFlagOptions(&opts)))

	// Register API objects
	utilruntime.Must(clientgoscheme.AddToScheme(scheme.Scheme))
	utilruntime.Must(buildv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme.Scheme))
	utilruntime.Must(routev1.AddToScheme(scheme.Scheme))
	utilruntime.Must(dspav1alpha1.AddToScheme(scheme.Scheme))

	//+kubebuilder:scaffold:scheme

	logf.Log.Info("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases"), filepath.Join("..", "config", "crd", "external")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), cfg)

	// Initialize Kubernetes client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), k8sClient)

	// Setup controller manager
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	assert.NoError(s.T(), err)

	err = (&DSPAReconciler{
		Client:        k8sClient,
		Log:           ctrl.Log.WithName("controllers").WithName("ds-pipelines-controller"),
		Scheme:        scheme.Scheme,
		TemplatesPath: "../config/internal/",
	}).SetupWithManager(mgr)
	assert.NoError(s.T(), err)

	// Start the manager
	go func() {
		err = mgr.Start(ctx)
		assert.NoError(s.T(), err, "Failed to run manager")
	}()
}

func (s *ControllerSuite) TearDownSuite() {
	// Give some time to allow workers to gracefully shutdown
	time.Sleep(5 * time.Second)
	cancel()
	logf.Log.Info("tearing down the test environment")
	time.Sleep(1 * time.Second)
	err := testEnv.Stop()
	assert.NoError(s.T(), err)
}
