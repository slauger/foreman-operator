package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
	"github.com/slauger/foreman-operator/internal/controller"
)

// version is set at build time.
var version = "dev"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(foremanv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gwapiv1.Install(scheme))
}

func main() {
	var metricsAddr string
	var secureMetrics bool
	var probeAddr string
	var enableLeaderElection bool
	var watchNamespace string
	var imageTag string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"Serve metrics over HTTPS and require an authenticated, authorized client.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election, so that only one operator instance is active.")
	flag.StringVar(&watchNamespace, "watch-namespace", "",
		"Namespace to restrict the operator to. If empty, the operator watches all namespaces.")
	flag.StringVar(&imageTag, "default-image-tag", "develop",
		"Tag of the default component images, used when a resource names no tag or digest.")

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog.Info("starting foreman-operator", "version", version)

	metrics := metricsserver.Options{BindAddress: metricsAddr}
	if secureMetrics {
		metrics.SecureServing = true
		metrics.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metrics,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "foreman-operator.platform.theforeman.org",
	}
	if watchNamespace != "" {
		setupLog.Info("watching single namespace", "namespace", watchNamespace)
		mgrOptions.Cache = cache.Options{DefaultNamespaces: map[string]cache.Config{watchNamespace: {}}}
		mgrOptions.LeaderElectionNamespace = watchNamespace
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	caps := detectCapabilities(mgr.GetRESTMapper())
	setupLog.Info("detected cluster capabilities",
		"certManager", caps.CertManager, "gatewayAPI", caps.GatewayAPI, "openShift", caps.OpenShift)
	images := controller.DefaultImages(imageTag)

	if err := (&controller.ForemanReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Images:       images,
		Capabilities: caps,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Foreman")
		os.Exit(1)
	}
	if err := (&controller.SmartProxyReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Images:       images,
		Capabilities: caps,
		NewAPI:       controller.NewForemanAPI,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SmartProxy")
		os.Exit(1)
	}

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

// detectCapabilities checks which optional APIs the cluster serves. The operator only
// watches kinds that exist, so a missing CRD does not stop it from starting.
func detectCapabilities(mapper meta.RESTMapper) controller.Capabilities {
	has := func(group, kind string) bool {
		_, err := mapper.RESTMapping(schema.GroupKind{Group: group, Kind: kind}, "v1")
		return err == nil
	}
	return controller.Capabilities{
		CertManager: has("cert-manager.io", "Certificate"),
		GatewayAPI:  has("gateway.networking.k8s.io", "TLSRoute") && has("gateway.networking.k8s.io", "HTTPRoute"),
		OpenShift:   has("route.openshift.io", "Route"),
	}
}
