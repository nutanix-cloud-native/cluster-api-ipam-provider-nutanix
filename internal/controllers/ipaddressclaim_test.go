// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -copyright_file ../../hack/license-header.txt -typed -destination ./mock_adapter/mock_client.go github.com/nutanix-cloud-native/prism-go-client/adapter Client,ClusterClient,PrismClient,NetworkingClient

package controllers

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"github.com/nutanix-cloud-native/prism-go-client/adapter"
	"github.com/nutanix-cloud-native/prism-go-client/environment/credentials"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/controllers/mock_adapter"
)

var ignoreUIDsOnIPAddress = komega.IgnorePaths{
	"TypeMeta",
	"ObjectMeta.OwnerReferences[0].UID",
	"ObjectMeta.OwnerReferences[1].UID",
	"ObjectMeta.OwnerReferences[2].UID",
	"Spec.Claim.UID",
	"Spec.Pool.UID",
}

func TestIPAddressClaimReconciler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "IPAddressClaimReconciler Suite")
}

func newClaim(name, namespace, poolKind, poolName string) ipamv1.IPAddressClaim {
	return ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(v1alpha1.GroupVersion.Group),
				Kind:     poolKind,
				Name:     poolName,
			},
		},
	}
}

var _ = Describe("IPAddressClaimReconciler", func() {
	var namespace string

	BeforeEach(func() {
		ns, err := env.CreateNamespace(context.Background(), "test-ns")
		Expect(err).NotTo(HaveOccurred())
		namespace = ns.Name

		mockController = gomock.NewController(GinkgoT())
		DeferCleanup(func() {
			Expect(mockController.Satisfied()).To(BeTrue())
		})
		DeferCleanup(mockController.Finish)

		mockPCClient = mock_adapter.NewMockClient(mockController)
	})

	Context("When a new IPAddressClaim is created", func() {
		When("the referenced pool is an unrecognized kind", func() {
			const poolName = "unknown-pool"

			It("should ignore the claim", func() {
				claim := newClaim("unknown-pool-test", namespace, "UnknownIPPool", poolName)
				Expect(env.Create(context.Background(), &claim)).To(Succeed())
				DeferCleanup(env.CleanupAndWait, context.Background(), &claim)

				Consistently(func(g Gomega) []ipamv1.IPAddress {
					addresses := ipamv1.IPAddressList{}
					g.Expect(
						env.List(context.Background(), &addresses, client.InNamespace(namespace)),
					).To(Succeed())
					return addresses.Items
				}).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(HaveLen(0))
			})
		})

		When("the referenced namespaced pool exists", func() {
			const (
				clusterName = "test-cluster"
				poolName    = "test-pool"
			)

			var pool v1alpha1.NutanixIPPool

			BeforeEach(func() {
				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: namespace,
					},
					StringData: map[string]string{
						credentials.KeyName: `
		[
		  {
		    "type": "basic_auth",
		    "data": {
		      "prismCentral":{
		        "username": "auser",
		        "password": "apassword"
		      }
		    }
		  }
		]`,
					},
				}
				Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())

				pool = v1alpha1.NutanixIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha1.NutanixIPPoolSpec{
						PrismCentral: v1alpha1.PrismCentral{
							Address: "prism.example.com",
							Port:    9440,
							CredentialsSecretRef: v1alpha1.LocalSecretRef{
								Name: "test-secret",
							},
						},
						Subnet: uuid.NewString(),
					},
				}
				Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
				DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)
			})

			It("should allocate an Address from the Pool", func() {
				mockPCClient.EXPECT().Networking().DoAndReturn(func() adapter.NetworkingClient {
					mockNC := mock_adapter.NewMockNetworkingClient(mockController)
					mockNC.EXPECT().ReserveIP(
						gomock.Any(),
						pool.Spec.Subnet,
						adapter.ReserveIPOpts{Cluster: ""},
					).Return(
						net.ParseIP("127.0.0.1"), nil,
					).Times(1)

					return mockNC
				}).Times(1)

				mockPCClient.EXPECT().Networking().DoAndReturn(func() adapter.NetworkingClient {
					mockNC := mock_adapter.NewMockNetworkingClient(mockController)
					mockNC.EXPECT().UnreserveIP(
						gomock.Any(),
						net.ParseIP("127.0.0.1"),
						pool.Spec.Subnet,
						adapter.UnreserveIPOpts{Cluster: ""},
					).Return(nil).Times(1)

					return mockNC
				}).Times(1)

				claim := newClaim("test", namespace, v1alpha1.NutanixIPPoolKind, poolName)
				expectedIPAddress := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion:         ipamv1.GroupVersion.String(),
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(true),
							Kind:               "IPAddressClaim",
							Name:               "test",
						}, {
							APIVersion:         v1alpha1.GroupVersion.String(),
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(false),
							Kind:               v1alpha1.NutanixIPPoolKind,
							Name:               poolName,
						}},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To(v1alpha1.GroupVersion.Group),
							Kind:     v1alpha1.NutanixIPPoolKind,
							Name:     poolName,
						},
						Address: "127.0.0.1",
					},
				}

				Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())

				Eventually(func(g Gomega) *ipamv1.IPAddress {
					address := ipamv1.IPAddress{}
					g.Expect(
						env.Get(context.Background(), client.ObjectKeyFromObject(&claim), &address),
					).To(Succeed())
					return &address
				}).WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					komega.EqualObject(
						&expectedIPAddress,
						komega.IgnoreAutogeneratedMetadata,
						ignoreUIDsOnIPAddress,
					),
				)

				Expect(env.CleanupAndWait(context.Background(), &claim)).To(Succeed())
			})
		})
	})
})
