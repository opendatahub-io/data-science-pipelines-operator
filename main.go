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

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/spf13/viper"

	"github.com/fsnotify/fsnotify"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers"
	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	admv1 "k8s.io/api/admissionregistration/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(buildv1.AddToScheme(scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))

	utilruntime.Must(dspav1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	controllers.InitMetrics()
}

func initConfig(configPath string) error {
	// Import environment variable, support nested vars e.g. OBJECTSTORECONFIG_ACCESSKEY
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	// Check for an environment variable any time a viper.Get request is made.
	viper.AutomaticEnv()
	// Treat empty environment variables as set
	viper.AllowEmptyEnv(true)

	// Set configuration file name. The format is auto-detected in this case.
	viper.SetConfigName("config")
	viper.AddConfigPath(configPath)
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}

	for _, c := range config.GetConfigRequiredFields() {
		if !viper.IsSet(c) {
			return fmt.Errorf("missing required field in config: %s", c)
		}
	}

	// Watch cfg file for live changes
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		// Read in cfg again
		err := viper.ReadInConfig()
		if err != nil {
			return
		}
	})

	return nil
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var configPath string
	var maxConcurrentReconciles int
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&configPath, "config", "", "Path to JSON file containing config")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&maxConcurrentReconciles, "MaxConcurrentReconciles", config.DefaultMaxConcurrentReconciles, "Maximum concurrent reconciles")
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.TimeEncoderOfLayout(time.RFC3339),
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	dspoNamespace := os.Getenv("DSPO_NAMESPACE")
	if dspoNamespace == "" {
		setupLog.Error(fmt.Errorf("missing environment variable"), "DSPO_NAMESPACE must be set")
		os.Exit(1)
	}

	err := initConfig(configPath)
	if err != nil {
		glog.Fatal(err)
	}

	webhookConfigName := "pipelineversions.pipelines.kubeflow.org"

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f9eb95d5.opendatahub.io",
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				// Limit the watches to only the pipelineversions.pipelines.kubeflow.org webhooks.
				&admv1.ValidatingWebhookConfiguration{}: {
					Field: fields.SelectorFromSet(fields.Set{"metadata.name": webhookConfigName}),
				},
				&admv1.MutatingWebhookConfiguration{}: {
					Field: fields.SelectorFromSet(fields.Set{"metadata.name": webhookConfigName}),
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.DSPAReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		Log:                     ctrl.Log,
		TemplatesPath:           "config/internal/",
		MaxConcurrentReconciles: maxConcurrentReconciles,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DSPAParams")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
