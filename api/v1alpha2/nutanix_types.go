// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import "fmt"

// NutanixIdentifierType is an enumeration of different resource identifier types.
type NutanixIdentifierType string

const (
	// NutanixIdentifierUUID is a resource identifier identifying the object by UUID.
	NutanixIdentifierUUID NutanixIdentifierType = "uuid"

	// NutanixIdentifierName is a resource identifier identifying the object by Name.
	NutanixIdentifierName NutanixIdentifierType = "name"
)

// NutanixResourceIdentifier holds the identity of a Nutanix PC resource (cluster, image, subnet, etc.)
// +union
// +kubebuilder:validation:XValidation:rule="self.type == 'name' ? has(self.name) : !has(self.name)",message="'name' must be set when type is 'name', and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="self.type == 'uuid' ? has(self.uuid) && self.uuid.contains('-') : !has(self.uuid)",message="'uuid' must be set when type is 'uuid', and forbidden otherwise"
type NutanixResourceIdentifier struct {
	// Type is the identifier type to use for this resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:=uuid;name
	Type NutanixIdentifierType `json:"type"`

	// uuid is the UUID of the resource in the PC.
	// +optional
	// +kubebuilder:validation:Format=uuid
	// +kubebuilder:validation:MaxLength=36
	// +kubebuilder:validation:MinLength=36
	UUID *string `json:"uuid,omitempty"`

	// name is the resource name in the PC
	// +optional
	// +kubebuilder:validation:MinLength=1
	Name *string `json:"name,omitempty"`
}

// NewNutanixResourceIdentifierUUID returns a UUID-typed resource identifier.
func NewNutanixResourceIdentifierUUID(id string) NutanixResourceIdentifier {
	return NutanixResourceIdentifier{
		Type: NutanixIdentifierUUID,
		UUID: &id,
	}
}

// NewNutanixResourceIdentifierName returns a name-typed resource identifier.
func NewNutanixResourceIdentifierName(name string) NutanixResourceIdentifier {
	return NutanixResourceIdentifier{
		Type: NutanixIdentifierName,
		Name: &name,
	}
}

func (nri NutanixResourceIdentifier) String() string {
	if nri.Type == NutanixIdentifierUUID && nri.UUID != nil {
		return *nri.UUID
	}
	if nri.Type == NutanixIdentifierName && nri.Name != nil {
		return *nri.Name
	}
	return ""
}

// DisplayString returns a human-readable string representation of the NutanixResourceIdentifier
// that includes both the type and value, suitable for error messages and logging.
func (nri NutanixResourceIdentifier) DisplayString() string {
	switch nri.Type {
	case NutanixIdentifierUUID:
		if nri.UUID != nil {
			return fmt.Sprintf("uuid=%q", *nri.UUID)
		}
	case NutanixIdentifierName:
		if nri.Name != nil {
			return fmt.Sprintf("name=%q", *nri.Name)
		}
	}
	return "unknown"
}

// IsUUID reports whether the identifier is a UUID with a value set.
func (nri NutanixResourceIdentifier) IsUUID() bool {
	return nri.Type == NutanixIdentifierUUID && nri.UUID != nil
}

// IsName reports whether the identifier is a name with a value set.
func (nri NutanixResourceIdentifier) IsName() bool {
	return nri.Type == NutanixIdentifierName && nri.Name != nil
}

// EqualTo checks if two NutanixResourceIdentifiers are equal based on their type and value.
func (nri NutanixResourceIdentifier) EqualTo(other *NutanixResourceIdentifier) bool {
	if other == nil {
		return false
	}
	if nri.Type != other.Type {
		return false
	}

	switch nri.Type {
	case NutanixIdentifierName:
		return nri.Name != nil && other.Name != nil && *nri.Name == *other.Name
	case NutanixIdentifierUUID:
		return nri.UUID != nil && other.UUID != nil && *nri.UUID == *other.UUID
	}

	return false
}
