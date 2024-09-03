// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clientgocache "k8s.io/client-go/tools/cache"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/controllers"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/index"
)

func main() {
	// Creates a logger to be used during the main func.
	setupLog := ctrl.Log.WithName("setup")

	clientScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(clientScheme))
	utilruntime.Must(clusterv1.AddToScheme(clientScheme))
	utilruntime.Must(ipamv1.AddToScheme(clientScheme))
	utilruntime.Must(v1alpha1.AddToScheme(clientScheme))

	mgrOptions := &ctrl.Options{
		Scheme: clientScheme,
		Metrics: metricsserver.Options{
			BindAddress: ":8080",
		},
		HealthProbeBindAddress: ":8081",
		LeaderElection:         true,
	}

	pflag.CommandLine.StringVar(
		&mgrOptions.Metrics.BindAddress,
		"metrics-bind-address",
		mgrOptions.Metrics.BindAddress,
		"The address the metric endpoint binds to.",
	)

	pflag.CommandLine.StringVar(
		&mgrOptions.HealthProbeBindAddress,
		"health-probe-bind-address",
		mgrOptions.HealthProbeBindAddress,
		"The address the probe endpoint binds to.",
	)

	pflag.CommandLine.StringVar(&mgrOptions.PprofBindAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	pflag.CommandLine.BoolVar(&mgrOptions.LeaderElection, "leader-elect", mgrOptions.LeaderElection,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	pflag.CommandLine.StringVar(
		&mgrOptions.LeaderElectionID,
		"leader-election-id",
		"",
		"The name of the resource that leader election will use for holding the leader lock.",
	)

	pflag.CommandLine.StringVar(
		&mgrOptions.LeaderElectionNamespace,
		"leader-election-namespace",
		"",
		"The namespace of the resource that leader election will use for holding the leader lock.",
	)

	logOptions := logs.NewOptions()

	// Initialize and parse command line flags.
	var (
		watchNamespace string
		watchFilter    string
	)
	pflag.CommandLine.StringVar(
		&watchNamespace,
		"namespace",
		"",
		"Namespace that the controller watches to reconcile cluster-api objects. "+
			"If unspecified, the controller watches for cluster-api objects across all namespaces.",
	)
	pflag.CommandLine.StringVar(&watchFilter, "watch-filter", "", "")

	reconcilerOpts := controllers.DefaultReconcilerOptions()
	reconcilerOpts.AddFlags(pflag.CommandLine)

	logs.AddFlags(pflag.CommandLine, logs.SkipLoggingConfigurationFlags())
	logsv1.AddFlags(logOptions, pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	verflag.PrintAndExitIfRequested()

	if mgrOptions.LeaderElection && mgrOptions.LeaderElectionID == "" {
		setupLog.Error(nil, "leader-election-id must be specified when leader-election is enabled")
		os.Exit(1)
	}

	if watchNamespace != "" {
		setupLog.Info("Watching cluster-api objects only in namespace", "namespace", watchNamespace)
		mgrOptions.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				watchNamespace: {},
			},
		}
	}

	// Validates logs flags using Kubernetes component-base machinery and applies them
	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "unable to apply logging configuration")
		os.Exit(1)
	}

	// Add the klog logger in the context.
	ctrl.SetLogger(klog.Background())

	signalCtx := ctrl.SetupSignalHandler()

	mgr, err := newManager(mgrOptions)
	if err != nil {
		setupLog.Error(err, "failed to create a new controller manager")
		os.Exit(1)
	}

	if err = index.SetupIndexes(signalCtx, mgr); err != nil {
		setupLog.Error(err, "failed to setup indexes")
		os.Exit(1)
	}

	secretInformer, configMapInformer, err := createInformers(signalCtx, mgr)
	if err != nil {
		setupLog.Error(err, "unable to create informers")
		os.Exit(1)
	}

	if err = (&ipamutil.ClaimReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		WatchFilterValue: watchFilter,
		Adapter: controllers.NewNutanixProviderAdapter(
			mgr.GetClient(),
			watchFilter,
			secretInformer,
			configMapInformer,
			reconcilerOpts,
		),
	}).SetupWithManager(signalCtx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPAddressClaim")
		os.Exit(1)
	}
	if err := mgr.Start(signalCtx); err != nil {
		setupLog.Error(err, "unable to start controller manager")
		os.Exit(1)
	}
}

func newManager(opts *manager.Options) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), *opts)
	if err != nil {
		return nil, fmt.Errorf("unable to create manager: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up ready check: %w", err)
	}

	return mgr, nil
}

func createInformers(
	ctx context.Context,
	mgr manager.Manager,
) (coreinformers.SecretInformer, coreinformers.ConfigMapInformer, error) {
	// Create a secret informer for the Nutanix client.
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create clientset for management cluster: %w", err)
	}

	informerFactory := informers.NewSharedInformerFactory(clientset, time.Minute)
	secretInformer := informerFactory.Core().V1().Secrets()
	informer := secretInformer.Informer()
	go informer.Run(ctx.Done())
	clientgocache.WaitForCacheSync(ctx.Done(), informer.HasSynced)

	configMapInformer := informerFactory.Core().V1().ConfigMaps()
	cmInformer := configMapInformer.Informer()
	go cmInformer.Run(ctx.Done())
	clientgocache.WaitForCacheSync(ctx.Done(), cmInformer.HasSynced)

	return secretInformer, configMapInformer, nil
}
