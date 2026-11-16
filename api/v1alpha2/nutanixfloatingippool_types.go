// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// NutanixFloatingIPPoolKind is the kind for NutanixFloatingIPPool objects.
	NutanixFloatingIPPoolKind = "NutanixFloatingIPPool"
)

// NutanixFloatingIPPoolSpec defines the desired state of NutanixFloatingIPPool.
// +kubebuilder:validation:XValidation:message="cluster is required when subnet.type is name",rule="self.subnet.type == 'uuid' || has(self.cluster)"
type NutanixFloatingIPPoolSpec struct {
	// PrismCentral is the configuration details of the Prism Central instance to use for floating IP allocation.
	// +kubebuilder:validation:Required
	PrismCentral PrismCentral `json:"prismCentral"`

	// Subnet is the Nutanix external subnet to allocate floating IPs from.
	// When type is name, the Cluster field must be set in order to resolve the name to a UUID.
	// +kubebuilder:validation:Required
	Subnet NutanixResourceIdentifier `json:"subnet"`

	// Cluster is the Nutanix PE cluster to use to resolve the Subnet name to a UUID.
	// This field is only required when Subnet.type is name.
	// +kubebuilder:validation:Optional
	Cluster *NutanixResourceIdentifier `json:"cluster,omitempty"`

	// Project is the optional Prism project that owns allocated floating IPs.
	// Accepts either a project UUID or project name; names are resolved at allocation time.
	// Required on project-scoped Prism Central deployments.
	// +kubebuilder:validation:Optional
	Project *NutanixResourceIdentifier `json:"project,omitempty"`
}

// NutanixFloatingIPPoolStatus defines the observed state of NutanixFloatingIPPool.
type NutanixFloatingIPPoolStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=cluster-api
// +kubebuilder:printcolumn:name="SubnetType",type="string",JSONPath=".spec.subnet.type",description="External subnet identifier type (uuid or name)"
// +kubebuilder:printcolumn:name="ClusterType",type="string",JSONPath=".spec.cluster.type",description="Optional PE Cluster identifier type"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NutanixFloatingIPPool is the Schema for the nutanixfloatingippools API.
type NutanixFloatingIPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NutanixFloatingIPPoolSpec   `json:"spec,omitempty"`
	Status NutanixFloatingIPPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NutanixFloatingIPPoolList contains a list of NutanixFloatingIPPool.
type NutanixFloatingIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NutanixFloatingIPPool `json:"items"`
}

func init() { //nolint:gochecknoinits // Idiomatic pattern for Kubernetes API types.
	SchemeBuilder.Register(
		&NutanixFloatingIPPool{},
		&NutanixFloatingIPPoolList{},
	)
}

// PoolSpec returns the pool spec.
func (p *NutanixFloatingIPPool) PoolSpec() *NutanixFloatingIPPoolSpec {
	return &p.Spec
}
