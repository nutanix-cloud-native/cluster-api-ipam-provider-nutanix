// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
)

var _ = DescribeTableSubtree(
	"validation",
	func(inputSpec v1alpha1.NutanixIPPoolSpec, wantErr bool) {
		var obj client.Object
		AfterEach(func() {
			if obj.GetName() != "" {
				Expect(k8sClient.Delete(context.Background(), obj)).To(Succeed())
			}
		})
		It("NutanixIPPool", func() {
			obj = &v1alpha1.NutanixIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    metav1.NamespaceDefault,
					GenerateName: "test-",
				},
				Spec: inputSpec,
			}
			err := k8sClient.Create(context.Background(), obj)
			if wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	},

	Entry("success with valid subnet uuid and ipv4 pc address", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: uuid.NewString(),
	}, false),

	Entry("success with valid subnet uuid and ipv6 pc address", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "::1",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: uuid.NewString(),
	}, false),

	Entry("success with valid subnet uuid and hostname pc address", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "aaa.example.com",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: uuid.NewString(),
	}, false),

	Entry("failure with invalid hostname pc address", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "aaa.example.com/this/is/invalid",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: uuid.NewString(),
	}, true),

	Entry("success with cluster and named subnet", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet:  "example-subnet-name",
		Cluster: ptr.To("example-cluster-name"),
	}, false),

	Entry("failure with missing cluster and named subnet", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: "example-subnet-name",
	}, true),

	Entry("failure with both additionalTrustBundle data and ref set", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
			AdditionalTrustBundle: &v1alpha1.AdditionalTrustBundle{
				Data: ptr.To("example-trust-bundle-data"),
				ConfigMapReference: &v1alpha1.LocalConfigMapRef{
					Name: "example-config-map-name",
				},
			},
		},
		Subnet:  "example-subnet-name",
		Cluster: ptr.To("example-cluster-name"),
	}, true),

	Entry("success with only data additionalTrustBundle set", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
			AdditionalTrustBundle: &v1alpha1.AdditionalTrustBundle{
				Data: ptr.To("example-trust-bundle-data"),
			},
		},
		Subnet:  "example-subnet-name",
		Cluster: ptr.To("example-cluster-name"),
	}, false),

	Entry("success with only configmap ref additionalTrustBundle set", v1alpha1.NutanixIPPoolSpec{
		PrismCentral: v1alpha1.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha1.LocalSecretRef{
				Name: "test-secret",
			},
			AdditionalTrustBundle: &v1alpha1.AdditionalTrustBundle{
				ConfigMapReference: &v1alpha1.LocalConfigMapRef{
					Name: "example-config-map-name",
				},
			},
		},
		Subnet:  "example-subnet-name",
		Cluster: ptr.To("example-cluster-name"),
	}, false),

	Entry(
		"failure with no configmap ref or data additionalTrustBundle set",
		v1alpha1.NutanixIPPoolSpec{
			PrismCentral: v1alpha1.PrismCentral{
				Address: "127.0.0.1",
				Port:    9440,
				CredentialsSecretRef: v1alpha1.LocalSecretRef{
					Name: "test-secret",
				},
				AdditionalTrustBundle: &v1alpha1.AdditionalTrustBundle{},
			},
			Subnet:  "example-subnet-name",
			Cluster: ptr.To("example-cluster-name"),
		},
		true,
	),
)
