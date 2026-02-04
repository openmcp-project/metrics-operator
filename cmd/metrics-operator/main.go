/*
Copyright 2024.

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
	"context"
	"embed"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/openmcp-project/controller-utils/pkg/api"
	"github.com/openmcp-project/controller-utils/pkg/init/crds"
	"github.com/openmcp-project/controller-utils/pkg/init/webhooks"

	"github.com/openmcp-project/metrics-operator/internal/controller"

	metricsv1alpha1 "github.com/openmcp-project/metrics-operator/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var _ = api.Target{}

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	//go:embed embedded/crds
	crdFiles embed.FS

	crdFlags      = crds.BindFlags(flag.CommandLine)
	webhooksFlags = webhooks.BindFlags(flag.CommandLine)
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))

	utilruntime.Must(metricsv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func runInit(setupClient client.Client) {
	initContext := context.Background()

	if webhooksFlags.Install {
		// Generate webhook certificate
		if err := webhooks.GenerateCertificate(initContext, setupClient, webhooksFlags.CertOptions...); err != nil {
			setupLog.Error(err, "unable to generate webhook certificates")
			os.Exit(1)
		}

		webhookTypes := []webhooks.APITypes{
			{
				Obj:       &metricsv1alpha1.Metric{},
				Validator: true,
				Defaulter: true,
			},
			{
				Obj:       &metricsv1alpha1.ManagedMetric{},
				Validator: true,
				Defaulter: true,
			},
			{
				Obj:       &metricsv1alpha1.RemoteClusterAccess{},
				Validator: true,
				Defaulter: false,
			},
			{
				Obj:       &metricsv1alpha1.FederatedMetric{},
				Validator: true,
				Defaulter: true,
			},
		}

		// Install webhooks
		err := webhooks.Install(
			initContext,
			setupClient,
			scheme,
			webhookTypes,
			webhooksFlags.InstallOptions...,
		)
		if err != nil {
			setupLog.Error(err, "unable to configure webhooks")
			os.Exit(1)
		}
	}

	if crdFlags.Install {
		// Install CRDs
		if err := crds.Install(initContext, setupClient, crdFiles, crdFlags.InstallOptions...); err != nil {
			setupLog.Error(err, "unable to install Custom Resource Definitions")
			os.Exit(1)
		}
	}
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	// skip os.Args[1] which is the command (start or init)
	err := flag.CommandLine.Parse(os.Args[2:])
	if err != nil {
		setupLog.Error(err, "unable to parse arguments for main method")
		return
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger)

	config := ctrl.GetConfigOrDie()
	setupClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create setup client")
		os.Exit(1)
	}

	if os.Args[1] == "init" {
		runInit(setupClient)
		return
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "82620e19.metrics.openmcp.cloud",
		Logger:                 logger,
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupMetricController(mgr)

	setupManagedMetricController(mgr)

	setupFederatedMetricController(mgr)

	setupFederatedManagedMetricController(mgr)

	// +kubebuilder:scaffold:builder

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

func setupFederatedMetricController(mgr ctrl.Manager) {
	if err := (controller.NewFederatedMetricReconciler(mgr)).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "federated metric")
		os.Exit(1)
	}
}

func setupFederatedManagedMetricController(mgr ctrl.Manager) {
	if err := (controller.NewFederatedManagedMetricReconciler(mgr)).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "federated managed metric")
		os.Exit(1)
	}
}

func setupMetricController(mgr ctrl.Manager) {
	if err := controller.NewMetricReconciler(mgr).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "metric")
		os.Exit(1)
	}
}

func setupManagedMetricController(mgr ctrl.Manager) {
	if err := controller.NewManagedMetricReconciler(mgr).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ManagedMetric")
		os.Exit(1)
	}
}
