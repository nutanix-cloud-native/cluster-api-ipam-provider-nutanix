// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/nutanix-cloud-native/prism-go-client/environment/credentials"

	v1alpha2 "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha2"
	pcclient "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/controllers/mockclient"
)

func floatingIPPool(namespace, name, secretName, subnet string) v1alpha2.NutanixFloatingIPPool {
	return v1alpha2.NutanixFloatingIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha2.NutanixFloatingIPPoolSpec{
			PrismCentral: v1alpha2.PrismCentral{
				Address: "prism.example.com",
				Port:    9440,
				CredentialsSecretRef: v1alpha2.LocalSecretRef{
					Name: secretName,
				},
			},
			Subnet: v1alpha2.NewNutanixResourceIdentifierUUID(subnet),
		},
	}
}

func floatingIPClaim(namespace, name, poolName string) v1alpha2.FloatingIPClaim {
	return v1alpha2.FloatingIPClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha2.FloatingIPClaimSpec{
			PoolRef: v1alpha2.FloatingIPPoolReference{
				Name: poolName,
			},
		},
	}
}

func floatingIPBinding(
	namespace, name, floatingIPName, address, vpc string,
) v1alpha2.FloatingIPBinding {
	return v1alpha2.FloatingIPBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha2.FloatingIPBindingSpec{
			FloatingIPRef: v1alpha2.LocalObjectReference{Name: floatingIPName},
			PrivateIP: &v1alpha2.FloatingIPPrivateIPAssociation{
				Address: address,
				VPC:     v1alpha2.NewNutanixResourceIdentifierUUID(vpc),
			},
		},
	}
}

func pcCredsSecret(namespace, name string) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: map[string]string{
			credentials.KeyName: `[
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
}

var _ = Describe("FloatingIP controllers", func() {
	var (
		namespace string
		subnet    = "11111111-1111-1111-1111-111111111111"
	)

	BeforeEach(func() {
		ns, err := env.CreateNamespace(context.Background(), "fip-ns")
		Expect(err).NotTo(HaveOccurred())
		namespace = ns.Name

		mockController = gomock.NewController(GinkgoT())
		DeferCleanup(func() {
			Expect(mockController.Satisfied()).To(BeTrue())
		})
		DeferCleanup(mockController.Finish)
		mockPCClient = mockclient.NewMockClient(mockController)
	})

	It("allocates a FloatingIP for a claim and releases it on delete", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		mockNet := mockclient.NewMockNetworkingClient(mockController)
		mockFIP := mockclient.NewMockFloatingIPsClient(mockController)
		mockPCClient.EXPECT().Networking().Return(mockNet).AnyTimes()
		mockPCClient.EXPECT().FloatingIPs().Return(mockFIP).AnyTimes()

		allocated := &pcclient.FloatingIP{
			ExtID:               "22222222-2222-2222-2222-222222222222",
			Name:                fmt.Sprintf("caipamx-%s-%s", namespace, claim.Name),
			Address:             "203.0.113.10",
			ExternalSubnetExtID: subnet,
			AssociationType:     pcclient.FloatingIPAssociationNone,
		}

		mockFIP.EXPECT().ListByName(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockNet.EXPECT().GetSubnet(gomock.Any(), subnet, gomock.Any()).Return(
			pcclient.NewSubnet(uuid.MustParse(subnet), 24), nil,
		).AnyTimes()
		mockFIP.EXPECT().CreateAsync(gomock.Any(), gomock.Any()).Return("task-1", nil)
		mockFIP.EXPECT().WaitForCreate(gomock.Any(), "task-1").Return(allocated, nil).AnyTimes()
		mockFIP.EXPECT().Get(gomock.Any(), allocated.ExtID).Return(allocated, nil).AnyTimes()
		mockFIP.EXPECT().Delete(gomock.Any(), allocated.ExtID).Return(nil)

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())

		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
			g.Expect(fip.Spec.Address).To(Equal(allocated.Address))
			g.Expect(controllerutil.ContainsFinalizer(fip, v1alpha2.ProtectFloatingIPFinalizer)).
				To(BeTrue())
		}).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			err := env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}).Should(Succeed())
	})

	It("lets the first successful reservation win and conflicts the second binding", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		mockNet := mockclient.NewMockNetworkingClient(mockController)
		mockFIP := mockclient.NewMockFloatingIPsClient(mockController)
		mockPCClient.EXPECT().Networking().Return(mockNet).AnyTimes()
		mockPCClient.EXPECT().FloatingIPs().Return(mockFIP).AnyTimes()

		allocated := &pcclient.FloatingIP{
			ExtID:               "33333333-3333-3333-3333-333333333333",
			Name:                fmt.Sprintf("caipamx-%s-%s", namespace, claim.Name),
			Address:             "203.0.113.20",
			ExternalSubnetExtID: subnet,
			AssociationType:     pcclient.FloatingIPAssociationNone,
		}
		associated := *allocated
		associated.AssociationType = pcclient.FloatingIPAssociationPrivateIP
		associated.PrivateIPAddress = "10.0.0.10"
		associated.PrivateIPVPCExtID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		prismState := allocated

		mockFIP.EXPECT().ListByName(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockNet.EXPECT().GetSubnet(gomock.Any(), subnet, gomock.Any()).Return(
			pcclient.NewSubnet(uuid.MustParse(subnet), 24), nil,
		).AnyTimes()
		mockFIP.EXPECT().CreateAsync(gomock.Any(), gomock.Any()).Return("task-2", nil)
		mockFIP.EXPECT().WaitForCreate(gomock.Any(), "task-2").Return(allocated, nil).AnyTimes()
		mockFIP.EXPECT().Get(gomock.Any(), allocated.ExtID).DoAndReturn(
			func(context.Context, string) (*pcclient.FloatingIP, error) {
				cp := *prismState
				return &cp, nil
			},
		).AnyTimes()
		mockFIP.EXPECT().
			AssignPrivateIP(gomock.Any(), allocated.ExtID, pcclient.PrivateIPAssociation{
				Address:  "10.0.0.10",
				VPCExtID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			}).
			DoAndReturn(func(context.Context, string, pcclient.PrivateIPAssociation) (*pcclient.FloatingIP, error) {
				prismState = &associated
				cp := associated
				return &cp, nil
			}).
			Times(1)
		mockFIP.EXPECT().Deassign(gomock.Any(), allocated.ExtID).DoAndReturn(
			func(context.Context, string) (*pcclient.FloatingIP, error) {
				prismState = allocated
				cp := *allocated
				return &cp, nil
			},
		).AnyTimes()
		mockFIP.EXPECT().Delete(gomock.Any(), allocated.ExtID).Return(nil).AnyTimes()

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
		}).Should(Succeed())

		binding1 := floatingIPBinding(namespace, "bind-1", claim.Name, "10.0.0.10", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		Expect(env.CreateAndWait(context.Background(), &binding1)).To(Succeed())

		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Status.Reservation).ToNot(BeNil())
			g.Expect(fip.Status.Reservation.Name).To(Equal("bind-1"))
			updated := &v1alpha2.FloatingIPBinding{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&binding1), updated)).
				To(Succeed())
			g.Expect(updated.Status.Conditions).
				To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":   Equal(v1alpha2.FloatingIPBindingReadyCondition),
					"Status": Equal(metav1.ConditionTrue),
				})))
		}).Should(Succeed())

		binding2 := floatingIPBinding(namespace, "bind-2", claim.Name, "10.0.0.11", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		Expect(env.CreateAndWait(context.Background(), &binding2)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &v1alpha2.FloatingIPBinding{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&binding2), updated)).
				To(Succeed())
			g.Expect(updated.Status.Conditions).
				To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":   Equal(v1alpha2.FloatingIPBindingReadyCondition),
					"Status": Equal(metav1.ConditionFalse),
					"Reason": Equal(v1alpha2.FloatingIPBindingAssociationConflictReason),
				})))
		}).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &binding2)).To(Succeed())
		Expect(env.CleanupAndWait(context.Background(), &binding1)).To(Succeed())
		Expect(env.CleanupAndWait(context.Background(), &claim)).To(Succeed())
	})

	It("reports AssociationConflict when multiple bindings exist before reservation", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		mockFIP := mockclient.NewMockFloatingIPsClient(mockController)
		mockPCClient.EXPECT().FloatingIPs().Return(mockFIP).AnyTimes()

		allocated := &pcclient.FloatingIP{
			ExtID:               "44444444-4444-4444-4444-444444444444",
			Name:                fmt.Sprintf("caipamx-%s-%s", namespace, claim.Name),
			Address:             "203.0.113.30",
			ExternalSubnetExtID: subnet,
			AssociationType:     pcclient.FloatingIPAssociationNone,
		}
		mockFIP.EXPECT().
			ListByName(gomock.Any(), allocated.Name).
			Return([]pcclient.FloatingIP{*allocated}, nil).
			AnyTimes()
		mockFIP.EXPECT().Get(gomock.Any(), allocated.ExtID).Return(allocated, nil).AnyTimes()
		mockFIP.EXPECT().Delete(gomock.Any(), allocated.ExtID).Return(nil).AnyTimes()

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
		}).Should(Succeed())

		binding1 := floatingIPBinding(namespace, "race-1", claim.Name, "10.0.0.20", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		binding1.Annotations = map[string]string{v1alpha2.BindingIDAnnotation: "race-id-1"}
		binding2 := floatingIPBinding(namespace, "race-2", claim.Name, "10.0.0.21", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		binding2.Annotations = map[string]string{v1alpha2.BindingIDAnnotation: "race-id-2"}
		Expect(env.Create(context.Background(), &binding1)).To(Succeed())
		Expect(env.Create(context.Background(), &binding2)).To(Succeed())

		// Ensure both bindings are visible to the indexer before reservation runs.
		Eventually(func(g Gomega) {
			for _, name := range []string{"race-1", "race-2"} {
				updated := &v1alpha2.FloatingIPBinding{}
				g.Expect(env.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, updated)).
					To(Succeed())
				g.Expect(updated.Annotations).To(HaveKey(v1alpha2.BindingIDAnnotation))
			}
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			if fip.Annotations == nil {
				fip.Annotations = map[string]string{}
			}
			fip.Annotations["test.ipam.cluster.x-k8s.io/kick"] = "1"
			g.Expect(env.Update(context.Background(), fip)).To(Succeed())
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			for _, name := range []string{"race-1", "race-2"} {
				updated := &v1alpha2.FloatingIPBinding{}
				g.Expect(env.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, updated)).
					To(Succeed())
				g.Expect(updated.Status.Conditions).
					To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Type":   Equal(v1alpha2.FloatingIPBindingReadyCondition),
						"Status": Equal(metav1.ConditionFalse),
						"Reason": Equal(v1alpha2.FloatingIPBindingAssociationConflictReason),
					})))
			}
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Status.Reservation).To(BeNil())
		}).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &binding1)).To(Succeed())
		Expect(env.CleanupAndWait(context.Background(), &binding2)).To(Succeed())
		Expect(env.CleanupAndWait(context.Background(), &claim)).To(Succeed())
	})

	It("skips Prism writes when delete-for-move is set", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)
		claim.Annotations = map[string]string{
			clusterctlv1.DeleteForMoveAnnotation: "",
		}

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())
		Consistently(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			err := env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(2 * time.Second).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &claim)).To(Succeed())
	})

	It("adopts an existing Prism floating IP by name instead of creating again", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		mockFIP := mockclient.NewMockFloatingIPsClient(mockController)
		mockPCClient.EXPECT().FloatingIPs().Return(mockFIP).AnyTimes()

		allocated := pcclient.FloatingIP{
			ExtID:               "55555555-5555-5555-5555-555555555555",
			Name:                fmt.Sprintf("caipamx-%s-%s", namespace, claim.Name),
			Address:             "203.0.113.40",
			ExternalSubnetExtID: subnet,
			AssociationType:     pcclient.FloatingIPAssociationNone,
		}
		mockFIP.EXPECT().
			ListByName(gomock.Any(), allocated.Name).
			Return([]pcclient.FloatingIP{allocated}, nil).
			AnyTimes()
		mockFIP.EXPECT().Get(gomock.Any(), allocated.ExtID).Return(&allocated, nil).AnyTimes()
		mockFIP.EXPECT().Delete(gomock.Any(), allocated.ExtID).Return(nil)

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
			g.Expect(fip.Spec.Address).To(Equal(allocated.Address))
		}).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &claim)).To(Succeed())
	})

	It("recovers an in-flight create from a stored task id without creating again", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		mockNet := mockclient.NewMockNetworkingClient(mockController)
		mockFIP := mockclient.NewMockFloatingIPsClient(mockController)
		mockPCClient.EXPECT().Networking().Return(mockNet).AnyTimes()
		mockPCClient.EXPECT().FloatingIPs().Return(mockFIP).AnyTimes()

		allocated := &pcclient.FloatingIP{
			ExtID:               "66666666-6666-6666-6666-666666666666",
			Name:                fmt.Sprintf("caipamx-%s-%s", namespace, claim.Name),
			Address:             "203.0.113.50",
			ExternalSubnetExtID: subnet,
			AssociationType:     pcclient.FloatingIPAssociationNone,
		}
		createdTask := make(chan struct{}, 1)
		proceed := make(chan struct{})
		DeferCleanup(func() {
			select {
			case <-proceed:
			default:
				close(proceed)
			}
		})

		mockFIP.EXPECT().ListByName(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockNet.EXPECT().GetSubnet(gomock.Any(), subnet, gomock.Any()).Return(
			pcclient.NewSubnet(uuid.MustParse(subnet), 24), nil,
		).AnyTimes()
		mockFIP.EXPECT().CreateAsync(gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, pcclient.CreateFloatingIPOpts) (string, error) {
				select {
				case createdTask <- struct{}{}:
				default:
				}
				return "task-recover", nil
			},
		).Times(1)
		mockFIP.EXPECT().WaitForCreate(gomock.Any(), "task-recover").DoAndReturn(
			func(ctx context.Context, _ string) (*pcclient.FloatingIP, error) {
				select {
				case <-proceed:
					return allocated, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		).AnyTimes()
		mockFIP.EXPECT().Get(gomock.Any(), allocated.ExtID).Return(allocated, nil).AnyTimes()
		mockFIP.EXPECT().Delete(gomock.Any(), allocated.ExtID).Return(nil)

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			select {
			case <-createdTask:
			default:
				g.Expect(false).To(BeTrue(), "CreateAsync has not been called yet")
			}
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Annotations).
				To(HaveKeyWithValue(v1alpha2.NutanixTaskIDAnnotation, "task-recover"))
		}).WithTimeout(15 * time.Second).Should(Succeed())

		close(proceed)

		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
			g.Expect(fip.Annotations).NotTo(HaveKey(v1alpha2.NutanixTaskIDAnnotation))
		}).WithTimeout(15 * time.Second).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &claim)).To(Succeed())
	})

	It("fails the claim when Prism no longer has the allocated floating IP", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		mockNet := mockclient.NewMockNetworkingClient(mockController)
		mockFIP := mockclient.NewMockFloatingIPsClient(mockController)
		mockPCClient.EXPECT().Networking().Return(mockNet).AnyTimes()
		mockPCClient.EXPECT().FloatingIPs().Return(mockFIP).AnyTimes()

		allocated := &pcclient.FloatingIP{
			ExtID:               "77777777-7777-7777-7777-777777777777",
			Name:                fmt.Sprintf("caipamx-%s-%s", namespace, claim.Name),
			Address:             "203.0.113.60",
			ExternalSubnetExtID: subnet,
			AssociationType:     pcclient.FloatingIPAssociationNone,
		}
		missing := false

		mockFIP.EXPECT().ListByName(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockNet.EXPECT().GetSubnet(gomock.Any(), subnet, gomock.Any()).Return(
			pcclient.NewSubnet(uuid.MustParse(subnet), 24), nil,
		).AnyTimes()
		mockFIP.EXPECT().CreateAsync(gomock.Any(), gomock.Any()).Return("task-missing", nil)
		mockFIP.EXPECT().
			WaitForCreate(gomock.Any(), "task-missing").
			Return(allocated, nil).
			AnyTimes()
		mockFIP.EXPECT().Get(gomock.Any(), allocated.ExtID).DoAndReturn(
			func(context.Context, string) (*pcclient.FloatingIP, error) {
				if missing {
					return nil, fmt.Errorf("floating IP %q not found", allocated.ExtID)
				}
				cp := *allocated
				return &cp, nil
			},
		).AnyTimes()
		mockFIP.EXPECT().Delete(gomock.Any(), allocated.ExtID).DoAndReturn(
			func(context.Context, string) error {
				missing = true
				return nil
			},
		).AnyTimes()

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
		}).Should(Succeed())

		missing = true
		Eventually(func(g Gomega) {
			updated := &v1alpha2.FloatingIPClaim{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), updated)).
				To(Succeed())
			if updated.Annotations == nil {
				updated.Annotations = map[string]string{}
			}
			updated.Annotations["test.ipam.cluster.x-k8s.io/force"] = "1"
			g.Expect(env.Update(context.Background(), updated)).To(Succeed())
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			updated := &v1alpha2.FloatingIPClaim{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), updated)).
				To(Succeed())
			g.Expect(updated.Status.Conditions).
				To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":   Equal(v1alpha2.FloatingIPClaimReadyCondition),
					"Status": Equal(metav1.ConditionFalse),
					"Reason": Equal(v1alpha2.FloatingIPClaimReadyAllocationFailedReason),
				})))
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
		}).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &claim)).To(Succeed())
	})

	It("requeues claim deletion while a binding holds the floating IP", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		mockNet := mockclient.NewMockNetworkingClient(mockController)
		mockFIP := mockclient.NewMockFloatingIPsClient(mockController)
		mockPCClient.EXPECT().Networking().Return(mockNet).AnyTimes()
		mockPCClient.EXPECT().FloatingIPs().Return(mockFIP).AnyTimes()

		allocated := &pcclient.FloatingIP{
			ExtID:               "88888888-8888-8888-8888-888888888888",
			Name:                fmt.Sprintf("caipamx-%s-%s", namespace, claim.Name),
			Address:             "203.0.113.70",
			ExternalSubnetExtID: subnet,
			AssociationType:     pcclient.FloatingIPAssociationNone,
		}
		associated := *allocated
		associated.AssociationType = pcclient.FloatingIPAssociationPrivateIP
		associated.PrivateIPAddress = "10.0.0.70"
		associated.PrivateIPVPCExtID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		prismState := allocated

		mockFIP.EXPECT().ListByName(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockNet.EXPECT().GetSubnet(gomock.Any(), subnet, gomock.Any()).Return(
			pcclient.NewSubnet(uuid.MustParse(subnet), 24), nil,
		).AnyTimes()
		mockFIP.EXPECT().CreateAsync(gomock.Any(), gomock.Any()).Return("task-bound", nil)
		mockFIP.EXPECT().WaitForCreate(gomock.Any(), "task-bound").Return(allocated, nil).AnyTimes()
		mockFIP.EXPECT().Get(gomock.Any(), allocated.ExtID).DoAndReturn(
			func(context.Context, string) (*pcclient.FloatingIP, error) {
				cp := *prismState
				return &cp, nil
			},
		).AnyTimes()
		mockFIP.EXPECT().AssignPrivateIP(gomock.Any(), allocated.ExtID, gomock.Any()).DoAndReturn(
			func(context.Context, string, pcclient.PrivateIPAssociation) (*pcclient.FloatingIP, error) {
				prismState = &associated
				cp := associated
				return &cp, nil
			},
		)
		mockFIP.EXPECT().Deassign(gomock.Any(), allocated.ExtID).DoAndReturn(
			func(context.Context, string) (*pcclient.FloatingIP, error) {
				prismState = allocated
				cp := *allocated
				return &cp, nil
			},
		).AnyTimes()
		mockFIP.EXPECT().Delete(gomock.Any(), allocated.ExtID).Return(nil).AnyTimes()

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
		}).Should(Succeed())

		binding := floatingIPBinding(namespace, "bound-1", claim.Name, "10.0.0.70", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		Expect(env.CreateAndWait(context.Background(), &binding)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(controllerutil.ContainsFinalizer(fip, v1alpha2.BoundFloatingIPFinalizer)).
				To(BeTrue())
			g.Expect(fip.Status.Reservation).NotTo(BeNil())
		}).Should(Succeed())

		Expect(env.Delete(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			updated := &v1alpha2.FloatingIPClaim{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), updated)).
				To(Succeed())
			g.Expect(updated.DeletionTimestamp.IsZero()).To(BeFalse())
			g.Expect(controllerutil.ContainsFinalizer(updated, v1alpha2.ReleaseFloatingIPFinalizer)).
				To(BeTrue())
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(controllerutil.ContainsFinalizer(fip, v1alpha2.ProtectFloatingIPFinalizer)).
				To(BeTrue())
			g.Expect(controllerutil.ContainsFinalizer(fip, v1alpha2.BoundFloatingIPFinalizer)).
				To(BeTrue())
		}).Should(Succeed())
		Consistently(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(controllerutil.ContainsFinalizer(fip, v1alpha2.ProtectFloatingIPFinalizer)).
				To(BeTrue())
			updated := &v1alpha2.FloatingIPClaim{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), updated)).
				To(Succeed())
			g.Expect(updated.DeletionTimestamp.IsZero()).To(BeFalse())
		}).WithTimeout(2 * time.Second).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &binding)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			err := env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			updated := &v1alpha2.FloatingIPClaim{}
			err = env.Get(context.Background(), client.ObjectKeyFromObject(&claim), updated)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(20 * time.Second).Should(Succeed())
	})

	It("clears reservation on unbind and lets a later binding reserve the same FloatingIP", func() {
		secret := pcCredsSecret(namespace, "pc-creds")
		pool := floatingIPPool(namespace, "fip-pool", secret.Name, subnet)
		claim := floatingIPClaim(namespace, "fip-claim", pool.Name)

		Expect(env.CreateAndWait(context.Background(), &secret)).To(Succeed())
		Expect(env.CreateAndWait(context.Background(), &pool)).To(Succeed())
		DeferCleanup(env.CleanupAndWait, context.Background(), &pool, &secret)

		mockNet := mockclient.NewMockNetworkingClient(mockController)
		mockFIP := mockclient.NewMockFloatingIPsClient(mockController)
		mockPCClient.EXPECT().Networking().Return(mockNet).AnyTimes()
		mockPCClient.EXPECT().FloatingIPs().Return(mockFIP).AnyTimes()

		allocated := &pcclient.FloatingIP{
			ExtID:               "99999999-9999-9999-9999-999999999999",
			Name:                fmt.Sprintf("caipamx-%s-%s", namespace, claim.Name),
			Address:             "203.0.113.80",
			ExternalSubnetExtID: subnet,
			AssociationType:     pcclient.FloatingIPAssociationNone,
		}
		associated1 := *allocated
		associated1.AssociationType = pcclient.FloatingIPAssociationPrivateIP
		associated1.PrivateIPAddress = "10.0.0.80"
		associated1.PrivateIPVPCExtID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		associated2 := *allocated
		associated2.AssociationType = pcclient.FloatingIPAssociationPrivateIP
		associated2.PrivateIPAddress = "10.0.0.81"
		associated2.PrivateIPVPCExtID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		prismState := allocated

		mockFIP.EXPECT().ListByName(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockNet.EXPECT().GetSubnet(gomock.Any(), subnet, gomock.Any()).Return(
			pcclient.NewSubnet(uuid.MustParse(subnet), 24), nil,
		).AnyTimes()
		mockFIP.EXPECT().CreateAsync(gomock.Any(), gomock.Any()).Return("task-rebind", nil)
		mockFIP.EXPECT().
			WaitForCreate(gomock.Any(), "task-rebind").
			Return(allocated, nil).
			AnyTimes()
		mockFIP.EXPECT().Get(gomock.Any(), allocated.ExtID).DoAndReturn(
			func(context.Context, string) (*pcclient.FloatingIP, error) {
				cp := *prismState
				return &cp, nil
			},
		).AnyTimes()
		mockFIP.EXPECT().
			AssignPrivateIP(gomock.Any(), allocated.ExtID, pcclient.PrivateIPAssociation{
				Address:  "10.0.0.80",
				VPCExtID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			}).
			DoAndReturn(func(context.Context, string, pcclient.PrivateIPAssociation) (*pcclient.FloatingIP, error) {
				prismState = &associated1
				cp := associated1
				return &cp, nil
			})
		mockFIP.EXPECT().
			AssignPrivateIP(gomock.Any(), allocated.ExtID, pcclient.PrivateIPAssociation{
				Address:  "10.0.0.81",
				VPCExtID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			}).
			DoAndReturn(func(context.Context, string, pcclient.PrivateIPAssociation) (*pcclient.FloatingIP, error) {
				prismState = &associated2
				cp := associated2
				return &cp, nil
			})
		mockFIP.EXPECT().Deassign(gomock.Any(), allocated.ExtID).DoAndReturn(
			func(context.Context, string) (*pcclient.FloatingIP, error) {
				prismState = allocated
				cp := *allocated
				return &cp, nil
			},
		).AnyTimes()
		mockFIP.EXPECT().Delete(gomock.Any(), allocated.ExtID).Return(nil).AnyTimes()

		Expect(env.CreateAndWait(context.Background(), &claim)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Spec.ExtID).To(Equal(allocated.ExtID))
		}).Should(Succeed())

		binding1 := floatingIPBinding(namespace, "rebind-1", claim.Name, "10.0.0.80", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		Expect(env.CreateAndWait(context.Background(), &binding1)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Status.Reservation).NotTo(BeNil())
			g.Expect(fip.Status.Reservation.Name).To(Equal("rebind-1"))
		}).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &binding1)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Status.Reservation).To(BeNil())
			g.Expect(controllerutil.ContainsFinalizer(fip, v1alpha2.BoundFloatingIPFinalizer)).
				To(BeFalse())
		}).WithTimeout(20 * time.Second).Should(Succeed())

		binding2 := floatingIPBinding(namespace, "rebind-2", claim.Name, "10.0.0.81", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		Expect(env.CreateAndWait(context.Background(), &binding2)).To(Succeed())
		Eventually(func(g Gomega) {
			fip := &v1alpha2.FloatingIP{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&claim), fip)).
				To(Succeed())
			g.Expect(fip.Status.Reservation).NotTo(BeNil())
			g.Expect(fip.Status.Reservation.Name).To(Equal("rebind-2"))
			updated := &v1alpha2.FloatingIPBinding{}
			g.Expect(env.Get(context.Background(), client.ObjectKeyFromObject(&binding2), updated)).
				To(Succeed())
			g.Expect(updated.Status.Conditions).
				To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":   Equal(v1alpha2.FloatingIPBindingReadyCondition),
					"Status": Equal(metav1.ConditionTrue),
				})))
		}).Should(Succeed())

		Expect(env.CleanupAndWait(context.Background(), &binding2, &claim)).To(Succeed())
	})
})
