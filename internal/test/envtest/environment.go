// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//nolint:gochecknoinits,lll // Code is copied from upstream.
package envtest

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
)

func init() {
	logger := klog.Background()
	// Use klog as the internal logger for this envtest environment.
	log.SetLogger(logger)
	// Additionally force all controllers to use the Ginkgo logger.
	ctrl.SetLogger(logger)
	// Add logger for ginkgo.
	klog.SetOutput(ginkgo.GinkgoWriter)
}

// RunInput is the input for Run.
type RunInput struct {
	M                   *testing.M
	ManagerUncachedObjs []client.Object
	SetupReconcilers    func(ctx context.Context, mgr ctrl.Manager)
	SetupEnv            func(e *Environment)
}

// Run executes the tests of the given testing.M in a test environment.
// Note: The environment will be created in this func and should not be created before. This func takes a *Environment
//
//	because our tests require access to the *Environment. We use this field to make the created Environment available
//	to the consumer.
//
// Note: It's possible to write a kubeconfig for the test environment to a file by setting `TEST_ENV_KUBECONFIG`.
// Note: It's possible to skip stopping the test env after the tests have been run by setting `TEST_ENV_SKIP_STOP`
// to a non-empty value.
func Run(ctx context.Context, input RunInput) int {
	// Bootstrapping test environment
	env := newEnvironment(input.ManagerUncachedObjs...)

	if input.SetupReconcilers != nil {
		input.SetupReconcilers(ctx, env.Manager)
	}

	// Start the environment.
	env.start(ctx)

	if kubeconfigPath := os.Getenv("TEST_ENV_KUBECONFIG"); kubeconfigPath != "" {
		klog.Infof("Writing test env kubeconfig to %q", kubeconfigPath)
		config := kubeconfig.FromEnvTestConfig(env.Config, &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		})
		if err := os.WriteFile(kubeconfigPath, config, 0o600); err != nil {
			panic(errors.Wrapf(err, "failed to write the test env kubeconfig"))
		}
	}

	// Expose the environment.
	input.SetupEnv(env)

	// Run tests
	code := input.M.Run()

	if skipStop := os.Getenv("TEST_ENV_SKIP_STOP"); skipStop != "" {
		klog.Info("Skipping test env stop as TEST_ENV_SKIP_STOP is set")
		return code
	}

	// Tearing down the test environment
	if err := env.stop(); err != nil {
		panic(fmt.Sprintf("Failed to stop the test environment: %v", err))
	}
	return code
}

var cacheSyncBackoff = wait.Backoff{
	Duration: 100 * time.Millisecond,
	Factor:   1.5,
	Steps:    8,
	Jitter:   0.4,
}

// Environment encapsulates a Kubernetes local test environment.
type Environment struct {
	manager.Manager
	client.Client
	Config *rest.Config

	env           *envtest.Environment
	cancelManager context.CancelFunc
}

// newEnvironment creates a new environment spinning up a local api-server.
//
// This function should be called only once for each package you're running tests within,
// usually the environment is initialized in a suite_test.go file within a `BeforeSuite` ginkgo block.
func newEnvironment(uncachedObjs ...client.Object) *Environment {
	// Create the test environment.
	env := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     CRDDirectoryPaths(),
	}

	if _, err := env.Start(); err != nil {
		err = kerrors.NewAggregate([]error{err, env.Stop()})
		panic(err)
	}

	// Calculate the scheme.
	mgrScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(mgrScheme))
	utilruntime.Must(clusterv1.AddToScheme(mgrScheme))
	utilruntime.Must(ipamv1.AddToScheme(mgrScheme))
	utilruntime.Must(v1alpha1.AddToScheme(mgrScheme))

	options := manager.Options{
		Scheme: mgrScheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: uncachedObjs,
			},
		},
	}

	mgr, err := ctrl.NewManager(env.Config, options)
	if err != nil {
		klog.Fatalf("Failed to start testenv manager: %v", err)
	}

	return &Environment{
		Manager: mgr,
		Client:  mgr.GetClient(),
		Config:  mgr.GetConfig(),
		env:     env,
	}
}

// start starts the manager.
func (e *Environment) start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancelManager = cancel

	go func() {
		fmt.Println("Starting the test environment manager")
		if err := e.Manager.Start(ctx); err != nil {
			panic(fmt.Sprintf("Failed to start the test environment manager: %v", err))
		}
	}()
	<-e.Manager.Elected()
}

// stop stops the test environment.
func (e *Environment) stop() error {
	fmt.Println("Stopping the test environment")
	e.cancelManager()
	return e.env.Stop()
}

// CreateKubeconfigSecret generates a new Kubeconfig secret from the envtest config.
func (e *Environment) CreateKubeconfigSecret(
	ctx context.Context,
	cluster *clusterv1.Cluster,
) error {
	return e.Create(
		ctx,
		kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(e.Config, cluster)),
	)
}

// Cleanup deletes all the given objects.
func (e *Environment) Cleanup(ctx context.Context, objs ...client.Object) error {
	errs := []error{}
	for _, o := range objs {
		err := e.Client.Delete(ctx, o)
		if apierrors.IsNotFound(err) {
			continue
		}
		errs = append(errs, err)
	}
	return kerrors.NewAggregate(errs)
}

// CleanupAndWait deletes all the given objects and waits for the cache to be updated accordingly.
//
// NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays.
func (e *Environment) CleanupAndWait(ctx context.Context, objs ...client.Object) error {
	if err := e.Cleanup(ctx, objs...); err != nil {
		return err
	}

	// Makes sure the cache is updated with the deleted object
	errs := []error{}
	for _, o := range objs {
		// Ignoring namespaces because in testenv the namespace cleaner is not running.
		if o.GetObjectKind().
			GroupVersionKind().
			GroupKind() ==
			corev1.SchemeGroupVersion.WithKind("Namespace").
				GroupKind() {
			continue
		}

		oCopy := o.DeepCopyObject().(client.Object)
		key := client.ObjectKeyFromObject(o)
		err := wait.ExponentialBackoff(
			cacheSyncBackoff,
			func() (done bool, err error) {
				if err := e.Get(ctx, key, oCopy); err != nil {
					if apierrors.IsNotFound(err) {
						return true, nil
					}
					return false, err
				}
				return false, nil
			})
		errs = append(
			errs,
			errors.Wrapf(
				err,
				"key %s, %s is not being deleted from the testenv client cache",
				o.GetObjectKind().GroupVersionKind().String(),
				key,
			),
		)
	}
	return kerrors.NewAggregate(errs)
}

// CreateAndWait creates the given object and waits for the cache to be updated accordingly.
//
// NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays.
func (e *Environment) CreateAndWait(
	ctx context.Context,
	obj client.Object,
	opts ...client.CreateOption,
) error {
	if err := e.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}

	// Makes sure the cache is updated with the new object
	objCopy := obj.DeepCopyObject().(client.Object)
	key := client.ObjectKeyFromObject(obj)
	if err := wait.ExponentialBackoff(
		cacheSyncBackoff,
		func() (done bool, err error) {
			if err := e.Get(ctx, key, objCopy); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		}); err != nil {
		return errors.Wrapf(
			err,
			"object %s, %s is not being added to the testenv client cache",
			obj.GetObjectKind().GroupVersionKind().String(),
			key,
		)
	}
	return nil
}

// PatchAndWait creates or updates the given object using server-side apply and waits for the cache to be updated accordingly.
//
// NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays.
func (e *Environment) PatchAndWait(
	ctx context.Context,
	obj client.Object,
	opts ...client.PatchOption,
) error {
	key := client.ObjectKeyFromObject(obj)
	objCopy := obj.DeepCopyObject().(client.Object)
	if err := e.GetAPIReader().Get(ctx, key, objCopy); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	// Store old resource version, empty string if not found.
	oldResourceVersion := objCopy.GetResourceVersion()

	if err := e.Client.Patch(ctx, obj, client.Apply, opts...); err != nil {
		return err
	}

	// Makes sure the cache is updated with the new object
	if err := wait.ExponentialBackoff(
		cacheSyncBackoff,
		func() (done bool, err error) {
			if err := e.Get(ctx, key, objCopy); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			if objCopy.GetResourceVersion() == oldResourceVersion {
				return false, nil
			}
			return true, nil
		}); err != nil {
		return errors.Wrapf(
			err,
			"object %s, %s is not being added to or did not get updated in the testenv client cache",
			obj.GetObjectKind().GroupVersionKind().String(),
			key,
		)
	}
	return nil
}

// CreateNamespace creates a new namespace with a generated name.
func (e *Environment) CreateNamespace(
	ctx context.Context,
	generateName string,
) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", generateName),
		},
	}
	if err := e.Client.Create(ctx, ns); err != nil {
		return nil, err
	}

	return ns, nil
}
