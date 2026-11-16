// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// FloatingIPKind is the kind for FloatingIP objects.
	FloatingIPKind = "FloatingIP"

	// FloatingIPReadyCondition reports whether the Prism floating IP is allocated.
	FloatingIPReadyCondition = "Ready"

	// FloatingIPAssociationNotAssociated means the floating IP is not associated.
	FloatingIPAssociationNotAssociated FloatingIPAssociationType = "notAssociated"
	// FloatingIPAssociationPrivateIP means the floating IP is associated to a private IP.
	FloatingIPAssociationPrivateIP FloatingIPAssociationType = "privateIP"
	// FloatingIPAssociationVMNic means the floating IP is associated to a VM NIC.
	FloatingIPAssociationVMNic FloatingIPAssociationType = "vmNic"

	// BindingIDAnnotation stores a stable identity for a FloatingIPBinding across recreation and move.
	BindingIDAnnotation = "ipam.cluster.x-k8s.io/binding-id"
	// NutanixTaskIDAnnotation stores an in-flight Prism task id.
	NutanixTaskIDAnnotation = "ipam.cluster.x-k8s.io/nutanix-task-id"
	// PrismFloatingIPNameAnnotation stores the deterministic Prism floating IP name used for adopt-by-name.
	PrismFloatingIPNameAnnotation = "ipam.cluster.x-k8s.io/prism-floating-ip-name"

	// ProtectFloatingIPFinalizer prevents deletion of a FloatingIP while its claim is not deleted.
	ProtectFloatingIPFinalizer = "ipam.cluster.x-k8s.io/protect-floating-ip"
	// ReleaseFloatingIPFinalizer releases the Prism floating IP before cleaning up the claim.
	ReleaseFloatingIPFinalizer = "ipam.cluster.x-k8s.io/release-floating-ip"
	// BoundFloatingIPFinalizer is set on FloatingIP while a binding holds the reservation.
	BoundFloatingIPFinalizer = "ipam.cluster.x-k8s.io/bound"
)

// FloatingIPAssociationType is the observed association kind on a FloatingIP.
// +kubebuilder:validation:Enum=notAssociated;privateIP;vmNic
type FloatingIPAssociationType string

// FloatingIPSpec is the desired state of a FloatingIP.
// Spec fields other than ClaimRef and PoolRef are written by the controller.
type FloatingIPSpec struct {
	// ClaimRef is a reference to the claim this FloatingIP was created for.
	// +kubebuilder:validation:Required
	ClaimRef LocalObjectReference `json:"claimRef"`

	// PoolRef is a reference to the pool that this FloatingIP was created from.
	// +kubebuilder:validation:Required
	PoolRef FloatingIPPoolReference `json:"poolRef"`

	// ExtID is the Prism floating IP external ID. Immutable once set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="extID is immutable once set"
	ExtID string `json:"extID,omitempty"`

	// Address is the allocated floating IPv4 address.
	// +optional
	// +kubebuilder:validation:Format=ipv4
	Address string `json:"address,omitempty"`

	// ExternalSubnet is the Prism external subnet the floating IP was allocated from.
	// +optional
	ExternalSubnet *NutanixResourceIdentifier `json:"externalSubnet,omitempty"`
}

// FloatingIPStatus is the observed state of a FloatingIP.
type FloatingIPStatus struct {
	// Conditions represents the observations of a FloatingIP's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Association is the observed Prism association for this floating IP.
	// +optional
	Association *FloatingIPAssociationStatus `json:"association,omitempty"`

	// Reservation is the active binding reservation for this floating IP.
	// +optional
	Reservation *FloatingIPReservation `json:"reservation,omitempty"`
}

// FloatingIPAssociationStatus reports the live association.
type FloatingIPAssociationStatus struct {
	// Type of association.
	// +kubebuilder:validation:Required
	Type FloatingIPAssociationType `json:"type"`

	// PrivateIP is set when Type is privateIP.
	// +optional
	PrivateIP *FloatingIPPrivateIPAssociation `json:"privateIP,omitempty"`

	// VMNic is set when Type is vmNic.
	// +optional
	VMNic *NutanixResourceIdentifier `json:"vmNic,omitempty"`
}

// FloatingIPReservation records which binding currently owns association rights.
type FloatingIPReservation struct {
	// Name of the reserved FloatingIPBinding.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// BindingID is the stable identity of the reserved FloatingIPBinding.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	BindingID string `json:"bindingID"`
}

// FloatingIPPrivateIPAssociation describes a private IP association.
type FloatingIPPrivateIPAssociation struct {
	// Address is the private IPv4 address.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=ipv4
	Address string `json:"address"`

	// VPC is the VPC that contains the private IP.
	// +kubebuilder:validation:Required
	VPC NutanixResourceIdentifier `json:"vpc"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=floatingips,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".spec.address",description="Allocated floating IP address"
// +kubebuilder:printcolumn:name="ExtID",type="string",JSONPath=".spec.extID",description="Prism floating IP external ID"
// +kubebuilder:printcolumn:name="Association",type="string",JSONPath=".status.association.type",description="Observed association type"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FloatingIP is the Schema for the floatingips API.
type FloatingIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FloatingIPSpec   `json:"spec,omitempty"`
	Status FloatingIPStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FloatingIPList contains a list of FloatingIP.
type FloatingIPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FloatingIP `json:"items"`
}

func init() { //nolint:gochecknoinits // Idiomatic pattern for Kubernetes API types.
	SchemeBuilder.Register(
		&FloatingIP{},
		&FloatingIPList{},
	)
}

// GetConditions returns the set of conditions for this object.
func (f *FloatingIP) GetConditions() []metav1.Condition {
	return f.Status.Conditions
}

// SetConditions sets conditions for this object.
func (f *FloatingIP) SetConditions(conditions []metav1.Condition) {
	f.Status.Conditions = conditions
}
