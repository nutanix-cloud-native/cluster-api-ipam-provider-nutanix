// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// FloatingIPBindingKind is the kind for FloatingIPBinding objects.
	FloatingIPBindingKind = "FloatingIPBinding"

	// FloatingIPBindingReadyCondition reports whether the binding is associated.
	FloatingIPBindingReadyCondition = "Ready"

	// FloatingIPBindingAssociationConflictReason is used when another binding holds or
	// competes for the floating IP reservation.
	FloatingIPBindingAssociationConflictReason = "AssociationConflict"

	// FloatingIPBindingAssociationPendingReason is used while association is in progress.
	FloatingIPBindingAssociationPendingReason = "AssociationPending"

	// FloatingIPBindingAssociationFailedReason is used when Prism association fails.
	FloatingIPBindingAssociationFailedReason = "AssociationFailed"

	// BoundFinalizer is set on a FloatingIPBinding while it holds the reservation.
	BoundFinalizer = "ipam.cluster.x-k8s.io/bound"
)

// FloatingIPBindingSpec is the desired state of a FloatingIPBinding.
// +kubebuilder:validation:XValidation:message="exactly one of privateIP or vmNic must be set",rule="has(self.privateIP) != has(self.vmNic)"
type FloatingIPBindingSpec struct {
	// FloatingIPRef is a reference to the FloatingIP to associate.
	// +kubebuilder:validation:Required
	FloatingIPRef LocalObjectReference `json:"floatingIPRef"`

	// PrivateIP associates the floating IP with a private IP in a VPC.
	// +optional
	PrivateIP *FloatingIPPrivateIPAssociation `json:"privateIP,omitempty"`

	// VMNic associates the floating IP with a VM NIC.
	// +optional
	VMNic *NutanixResourceIdentifier `json:"vmNic,omitempty"`
}

// FloatingIPBindingStatus is the observed state of a FloatingIPBinding.
type FloatingIPBindingStatus struct {
	// Conditions represents the observations of a FloatingIPBinding's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=floatingipbindings,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FloatingIP",type="string",JSONPath=".spec.floatingIPRef.name",description="Referenced FloatingIP"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status",description="Ready condition"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FloatingIPBinding is the Schema for the floatingipbindings API.
type FloatingIPBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FloatingIPBindingSpec   `json:"spec,omitempty"`
	Status FloatingIPBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FloatingIPBindingList contains a list of FloatingIPBinding.
type FloatingIPBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FloatingIPBinding `json:"items"`
}

func init() { //nolint:gochecknoinits // Idiomatic pattern for Kubernetes API types.
	SchemeBuilder.Register(
		&FloatingIPBinding{},
		&FloatingIPBindingList{},
	)
}

// GetConditions returns the set of conditions for this object.
func (b *FloatingIPBinding) GetConditions() []metav1.Condition {
	return b.Status.Conditions
}

// SetConditions sets conditions for this object.
func (b *FloatingIPBinding) SetConditions(conditions []metav1.Condition) {
	b.Status.Conditions = conditions
}
