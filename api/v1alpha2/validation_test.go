// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha2"
)

var _ = DescribeTableSubtree(
	"validation",
	func(inputSpec v1alpha2.NutanixIPPoolSpec, wantErr bool) {
		var obj client.Object
		AfterEach(func() {
			if obj.GetName() != "" {
				Expect(k8sClient.Delete(context.Background(), obj)).To(Succeed())
			}
		})
		It("NutanixIPPool", func() {
			obj = &v1alpha2.NutanixIPPool{
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

	Entry("success with valid subnet uuid and ipv4 pc address", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: v1alpha2.NewNutanixResourceIdentifierUUID(uuid.NewString()),
	}, false),

	Entry("success with valid subnet uuid and ipv6 pc address", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "::1",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: v1alpha2.NewNutanixResourceIdentifierUUID(uuid.NewString()),
	}, false),

	Entry("success with valid subnet uuid and hostname pc address", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "aaa.example.com",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: v1alpha2.NewNutanixResourceIdentifierUUID(uuid.NewString()),
	}, false),

	Entry("failure with invalid hostname pc address", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "aaa.example.com/this/is/invalid",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: v1alpha2.NewNutanixResourceIdentifierUUID(uuid.NewString()),
	}, true),

	Entry("success with cluster and named subnet", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet:  v1alpha2.NewNutanixResourceIdentifierName("example-subnet-name"),
		Cluster: ptr.To(v1alpha2.NewNutanixResourceIdentifierName("example-cluster-name")),
	}, false),

	Entry("failure with missing cluster and named subnet", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
		},
		Subnet: v1alpha2.NewNutanixResourceIdentifierName("example-subnet-name"),
	}, true),

	Entry("failure with both additionalTrustBundle data and ref set", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
			AdditionalTrustBundle: &v1alpha2.AdditionalTrustBundle{
				Data: []byte("example-trust-bundle-data"),
				ConfigMapReference: &v1alpha2.LocalConfigMapRef{
					Name: "example-config-map-name",
				},
			},
		},
		Subnet:  v1alpha2.NewNutanixResourceIdentifierName("example-subnet-name"),
		Cluster: ptr.To(v1alpha2.NewNutanixResourceIdentifierName("example-cluster-name")),
	}, true),

	Entry("success with only data additionalTrustBundle set", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
			AdditionalTrustBundle: &v1alpha2.AdditionalTrustBundle{
				Data: []byte("example-trust-bundle-data"),
			},
		},
		Subnet:  v1alpha2.NewNutanixResourceIdentifierName("example-subnet-name"),
		Cluster: ptr.To(v1alpha2.NewNutanixResourceIdentifierName("example-cluster-name")),
	}, false),

	Entry("success with only configmap ref additionalTrustBundle set", v1alpha2.NutanixIPPoolSpec{
		PrismCentral: v1alpha2.PrismCentral{
			Address: "127.0.0.1",
			Port:    9440,
			CredentialsSecretRef: v1alpha2.LocalSecretRef{
				Name: "test-secret",
			},
			AdditionalTrustBundle: &v1alpha2.AdditionalTrustBundle{
				ConfigMapReference: &v1alpha2.LocalConfigMapRef{
					Name: "example-config-map-name",
				},
			},
		},
		Subnet:  v1alpha2.NewNutanixResourceIdentifierName("example-subnet-name"),
		Cluster: ptr.To(v1alpha2.NewNutanixResourceIdentifierName("example-cluster-name")),
	}, false),

	Entry(
		"failure with no configmap ref or data additionalTrustBundle set",
		v1alpha2.NutanixIPPoolSpec{
			PrismCentral: v1alpha2.PrismCentral{
				Address: "127.0.0.1",
				Port:    9440,
				CredentialsSecretRef: v1alpha2.LocalSecretRef{
					Name: "test-secret",
				},
				AdditionalTrustBundle: &v1alpha2.AdditionalTrustBundle{},
			},
			Subnet:  v1alpha2.NewNutanixResourceIdentifierName("example-subnet-name"),
			Cluster: ptr.To(v1alpha2.NewNutanixResourceIdentifierName("example-cluster-name")),
		},
		true,
	),
)
