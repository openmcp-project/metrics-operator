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

	"github.com/go-logr/logr"
	"github.com/openmcp-project/controller-utils/pkg/api"
	"github.com/openmcp-project/controller-utils/pkg/init/crds"
	"github.com/openmcp-project/controller-utils/pkg/init/webhooks"

	"github.com/SAP/metrics-operator/internal/controller"

	metricsv1alpha1 "github.com/SAP/metrics-operator/api/v1alpha1"
	//+kubebuilder:scaffold:imports
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
	//+kubebuilder:scaffold:scheme
}

func runInit(setupClient client.Client) {
	initContext := context.Background()

	if webhooksFlags.Install {
		// Generate webhook certificate
		if err := webhooks.GenerateCertificate(initContext, setupClient, webhooksFlags.CertOptions...); err != nil {
			setupLog.Error(err, "unable to generate webhook certificates")
			os.Exit(1)
		}

		// Install webhooks
		err := webhooks.Install(
			initContext,
			setupClient,
			scheme,
			[]client.Object{
				&metricsv1alpha1.Metric{},
				&metricsv1alpha1.ManagedMetric{},
				&metricsv1alpha1.RemoteClusterAccess{},
				&metricsv1alpha1.FederatedMetric{},
			},
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
	var useEventDrivenController bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&useEventDrivenController, "use-event-driven-controller", true,
		"Use the new event-driven controller instead of the traditional metric controller.")

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
		LeaderElectionID:       "82620e19.orchestrate.cloud.sap",
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

	// TODO: to deprecate v1beta1 resources
	// Choose between traditional and event-driven controller based on feature flag
	if useEventDrivenController {
		setupEventDrivenController(mgr) // New event-driven controller for Metric CRs
	} else {
		setupMetricController(mgr) // Traditional metric controller
	}
	setupManagedMetricController(mgr)

	setupReconcilersV1beta1(mgr)

	if err = (&controller.ClusterAccessReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterAccess")
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

func setupReconcilersV1beta1(mgr ctrl.Manager) {
	if err := (controller.NewFederatedMetricReconciler(mgr)).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "federated metric")
		os.Exit(1)
	}

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

func setupEventDrivenController(mgr ctrl.Manager) {
	// Create and setup the new event-driven controller
	eventDrivenController := controller.NewEventDrivenController(mgr)
	if err := eventDrivenController.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create event-driven controller", "controller", "EventDriven")
		os.Exit(1)
	}

	// Add a runnable to start the event-driven system when the manager starts
	err := mgr.Add(&eventDrivenRunnable{
		controller: eventDrivenController,
		logger:     setupLog,
	})
	if err != nil {
		setupLog.Error(err, "unable to add event-driven runnable to manager")
		os.Exit(1)
	}
}

// eventDrivenRunnable implements manager.Runnable to properly integrate with the controller manager lifecycle
type eventDrivenRunnable struct {
	controller *controller.EventDrivenController
	logger     logr.Logger
}

func (r *eventDrivenRunnable) Start(ctx context.Context) error {
	r.logger.Info("Starting event-driven runnable")

	// Start the event-driven system with the proper context
	if err := r.controller.Start(ctx); err != nil {
		r.logger.Error(err, "failed to start event-driven controller")
		return err
	}

	// Keep running until context is cancelled
	<-ctx.Done()
	r.logger.Info("Event-driven runnable stopping")
	r.controller.Stop()
	return nil
}

func setupManagedMetricController(mgr ctrl.Manager) {
	if err := controller.NewManagedMetricReconciler(mgr).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ManagedMetric")
		os.Exit(1)
	}
}
