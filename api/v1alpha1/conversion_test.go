// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"testing"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha2"
)

func TestNutanixIPPoolConvertUUIDSubnet(t *testing.T) {
	subnetUUID := uuid.NewString()
	src := &NutanixIPPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "default"},
		Spec: NutanixIPPoolSpec{
			PrismCentral: PrismCentral{
				Address: "pc.example.com",
				Port:    9440,
				CredentialsSecretRef: LocalSecretRef{
					Name: "creds",
				},
			},
			Subnet: subnetUUID,
		},
	}

	dst := &v1alpha2.NutanixIPPool{}
	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo: %v", err)
	}
	if !dst.Spec.Subnet.IsUUID() || dst.Spec.Subnet.String() != subnetUUID {
		t.Fatalf("subnet = %#v, want uuid %s", dst.Spec.Subnet, subnetUUID)
	}

	roundTrip := &NutanixIPPool{}
	if err := roundTrip.ConvertFrom(dst); err != nil {
		t.Fatalf("ConvertFrom: %v", err)
	}
	if roundTrip.Spec.Subnet != subnetUUID {
		t.Fatalf("round-trip subnet = %q, want %q", roundTrip.Spec.Subnet, subnetUUID)
	}
}

func TestNutanixIPPoolConvertNamedSubnetAndCluster(t *testing.T) {
	src := &NutanixIPPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "default"},
		Spec: NutanixIPPoolSpec{
			PrismCentral: PrismCentral{
				Address: "pc.example.com",
				Port:    9440,
				CredentialsSecretRef: LocalSecretRef{
					Name: "creds",
				},
			},
			Subnet:  "sp-coke-subnet",
			Cluster: ptr.To("ncn-ci-workloads"),
		},
	}

	dst := &v1alpha2.NutanixIPPool{}
	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo: %v", err)
	}
	if !dst.Spec.Subnet.IsName() || dst.Spec.Subnet.String() != "sp-coke-subnet" {
		t.Fatalf("subnet = %#v", dst.Spec.Subnet)
	}
	if dst.Spec.Cluster == nil || !dst.Spec.Cluster.IsName() ||
		dst.Spec.Cluster.String() != "ncn-ci-workloads" {
		t.Fatalf("cluster = %#v", dst.Spec.Cluster)
	}

	roundTrip := &NutanixIPPool{}
	if err := roundTrip.ConvertFrom(dst); err != nil {
		t.Fatalf("ConvertFrom: %v", err)
	}
	if roundTrip.Spec.Subnet != "sp-coke-subnet" {
		t.Fatalf("round-trip subnet = %q", roundTrip.Spec.Subnet)
	}
	if roundTrip.Spec.Cluster == nil || *roundTrip.Spec.Cluster != "ncn-ci-workloads" {
		t.Fatalf("round-trip cluster = %#v", roundTrip.Spec.Cluster)
	}
}
