// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// FloatingIPClaimKind is the kind for FloatingIPClaim objects.
	FloatingIPClaimKind = "FloatingIPClaim"

	// FloatingIPClaimReadyCondition reports whether allocation succeeded.
	FloatingIPClaimReadyCondition = "Ready"

	// FloatingIPClaimReadyAllocationFailedReason is used when Prism allocation fails.
	FloatingIPClaimReadyAllocationFailedReason = "AllocationFailed"

	// FloatingIPClaimReadyPoolNotReadyReason is used when the referenced pool cannot be used.
	FloatingIPClaimReadyPoolNotReadyReason = "PoolNotReady"

	// FloatingIPClaimReadyWaitingForUnbindReason is used while a binding still holds the floating IP.
	FloatingIPClaimReadyWaitingForUnbindReason = "WaitingForUnbind"
)

// FloatingIPClaimSpec is the desired state of a FloatingIPClaim.
type FloatingIPClaimSpec struct {
	// ClusterName is the optional name of the Cluster this claim belongs to.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`

	// PoolRef is a reference to the NutanixFloatingIPPool to allocate from.
	// +kubebuilder:validation:Required
	PoolRef FloatingIPPoolReference `json:"poolRef"`

	// Address is an optional requested IPv4 floating IP address.
	// +optional
	// +kubebuilder:validation:Format=ipv4
	Address string `json:"address,omitempty"`
}

// FloatingIPPoolReference references a NutanixFloatingIPPool.
type FloatingIPPoolReference struct {
	// Name of the pool.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Kind of the pool. Defaults to NutanixFloatingIPPool.
	// +optional
	// +kubebuilder:default=NutanixFloatingIPPool
	// +kubebuilder:validation:Enum=NutanixFloatingIPPool
	Kind string `json:"kind,omitempty"`

	// APIGroup of the pool. Defaults to ipam.cluster.x-k8s.io.
	// +optional
	// +kubebuilder:default=ipam.cluster.x-k8s.io
	APIGroup string `json:"apiGroup,omitempty"`
}

// FloatingIPClaimStatus is the observed state of a FloatingIPClaim.
type FloatingIPClaimStatus struct {
	// Conditions represents the observations of a FloatingIPClaim's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// FloatingIPRef is a reference to the FloatingIP created for this claim.
	// +optional
	FloatingIPRef *LocalObjectReference `json:"floatingIPRef,omitempty"`
}

// LocalObjectReference references an object in the same namespace.
type LocalObjectReference struct {
	// Name of the referenced object.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=floatingipclaims,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pool",type="string",JSONPath=".spec.poolRef.name",description="Name of the pool to allocate from"
// +kubebuilder:printcolumn:name="FloatingIP",type="string",JSONPath=".status.floatingIPRef.name",description="Name of the allocated FloatingIP"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status",description="Ready condition"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FloatingIPClaim is the Schema for the floatingipclaims API.
type FloatingIPClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FloatingIPClaimSpec   `json:"spec,omitempty"`
	Status FloatingIPClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FloatingIPClaimList contains a list of FloatingIPClaim.
type FloatingIPClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FloatingIPClaim `json:"items"`
}

func init() { //nolint:gochecknoinits // Idiomatic pattern for Kubernetes API types.
	SchemeBuilder.Register(
		&FloatingIPClaim{},
		&FloatingIPClaimList{},
	)
}

// GetConditions returns the set of conditions for this object.
func (c *FloatingIPClaim) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets conditions for this object.
func (c *FloatingIPClaim) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}
