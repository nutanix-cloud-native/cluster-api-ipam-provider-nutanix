// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	clientgocache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/controllers/mockclient"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/index"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/test/envtest"
)

var (
	ctx            = ctrl.SetupSignalHandler()
	env            *envtest.Environment
	mockController *gomock.Controller
	mockPCClient   *mockclient.MockClient
)

func TestMain(m *testing.M) {
	RegisterFailHandler(Fail)

	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
		Expect(err).NotTo(HaveOccurred())
		informerFactory := informers.NewSharedInformerFactory(clientset, time.Minute)
		secretInformer := informerFactory.Core().V1().Secrets()
		informer := secretInformer.Informer()
		go informer.Run(ctx.Done())
		Expect(clientgocache.WaitForCacheSync(ctx.Done(), informer.HasSynced)).To(BeTrue())

		Expect(index.SetupIndexes(ctx, mgr)).To(Succeed())
		Expect(
			(&ipamutil.ClaimReconciler{
				Client: mgr.GetClient(),
				Scheme: mgr.GetScheme(),
				Adapter: &NutanixProviderAdapter{
					k8sClient:      mgr.GetClient(),
					secretInformer: secretInformer,
					pcClientGetter: func(_ client.CachedClientParams) (client.Client, error) {
						return mockPCClient, nil
					},
				},
			}).SetupWithManager(ctx, mgr),
		).To(Succeed())
	}
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(5 * time.Second)
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M: m,
		SetupEnv: func(e *envtest.Environment) {
			env = e
		},
		SetupReconcilers: setupReconcilers,
	}))
}
