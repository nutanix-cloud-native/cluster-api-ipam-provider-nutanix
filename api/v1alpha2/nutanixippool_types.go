// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// NutanixIPPoolKind is the kind for NutanixIPPool objects.
	NutanixIPPoolKind = "NutanixIPPool"
)

// NutanixIPPoolSpec defines the desired state of NutanixIPPool.
// +kubebuilder:validation:XValidation:message="cluster is required when subnet.type is name",rule="self.subnet.type == 'uuid' || has(self.cluster)"
type NutanixIPPoolSpec struct {
	// PrismCentral is the configuration details of the Prism Central instance to use for IPAM.
	// +kubebuilder:validation:Required
	PrismCentral PrismCentral `json:"prismCentral"`

	// Subnet is the Nutanix subnet to allocate IPs from.
	// When type is name, the Cluster field must be set in order to resolve the name to a UUID.
	// +kubebuilder:validation:Required
	Subnet NutanixResourceIdentifier `json:"subnet"`

	// Cluster is the Nutanix PE cluster to use to resolve the Subnet name to a UUID.
	// This field is only required when Subnet.type is name.
	// +kubebuilder:validation:Optional
	Cluster *NutanixResourceIdentifier `json:"cluster,omitempty"`
}

type PrismCentral struct {
	// Address is the address of the Prism Central instance to use for IPAM.
	// Address can either be the IP address or the DNS name of the Prism Central instance, omitting
	// the protocol and port.
	// +kubebuilder:validation:Required
	Address string `json:"address"`

	// Port is the port of the Prism Central instance to use for IPAM.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Required
	// +kubebuilder:default=9440
	Port uint16 `json:"port"`

	// CredentialsSecretRef is the reference to the secret containing the credentials to use to connect
	// the specified Prism Central.
	// +kubebuilder:validation:Required
	CredentialsSecretRef LocalSecretRef `json:"credentialsSecretRef"`

	// use insecure connection to Prism endpoint
	// +kubebuilder:default=false
	// +kubebuilder:validation:Optional
	Insecure bool `json:"insecure,omitempty"`

	// AdditionalTrustBundle is a PEM encoded x509 cert for the RootCA that was used to create the certificate
	// for a Prism Central that uses certificates that were issued by a non-publicly trusted RootCA. The trust
	// bundle is added to the cert pool used to authenticate the TLS connection to the Prism Central.
	// +kubebuilder:validation:Optional
	AdditionalTrustBundle *AdditionalTrustBundle `json:"additionalTrustBundle,omitempty"`
}

// AdditionalTrustBundle is a reference to a Nutanix trust bundle.
type AdditionalTrustBundle struct {
	// Data of the trust bundle.
	// +kubebuilder:validation:Optional
	Data []byte `json:"trustBundleData,omitempty"`

	// ConfigMapReference to the configmap holding the trust bundle data.
	// +kubebuilder:validation:Optional
	ConfigMapReference *LocalConfigMapRef `json:"trustBundleConfigMapRef,omitempty"`
}

type LocalConfigMapRef struct {
	// Name is the name of the referenced configmap.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

type LocalSecretRef struct {
	// Name is the name of the referenced secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// NutanixIPPoolStatus defines the observed state of NutanixIPPool.
type NutanixIPPoolStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=cluster-api
// +kubebuilder:printcolumn:name="SubnetType",type="string",JSONPath=".spec.subnet.type",description="Subnet identifier type (uuid or name)"
// +kubebuilder:printcolumn:name="ClusterType",type="string",JSONPath=".spec.cluster.type",description="Optional PE Cluster identifier type"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NutanixIPPool is the Schema for the nutanixippools API.
type NutanixIPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NutanixIPPoolSpec   `json:"spec,omitempty"`
	Status NutanixIPPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NutanixIPPoolList contains a list of NutanixIPPool.
type NutanixIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NutanixIPPool `json:"items"`
}

func init() { //nolint:gochecknoinits // Idiomatic pattern for Kubernetes API types.
	SchemeBuilder.Register(
		&NutanixIPPool{},
		&NutanixIPPoolList{},
	)
}

// PoolSpec implements the generic NutanixIPPool interface.
func (p *NutanixIPPool) PoolSpec() *NutanixIPPoolSpec {
	return &p.Spec
}

// PoolStatus implements the generic NutanixIPPool interface.
func (p *NutanixIPPool) PoolStatus() *NutanixIPPoolStatus {
	return &p.Status
}
