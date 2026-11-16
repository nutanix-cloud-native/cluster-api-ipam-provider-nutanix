// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	"testing"

	"github.com/google/uuid"
	"k8s.io/utils/ptr"
)

func TestNutanixResourceIdentifierUUID(t *testing.T) {
	id := uuid.NewString()
	nri := NewNutanixResourceIdentifierUUID(id)

	if !nri.IsUUID() {
		t.Fatalf("expected IsUUID")
	}
	if nri.IsName() {
		t.Fatalf("did not expect IsName")
	}
	if got := nri.String(); got != id {
		t.Fatalf("String() = %q, want %q", got, id)
	}
	if got := nri.DisplayString(); got != `uuid="`+id+`"` {
		t.Fatalf("DisplayString() = %q", got)
	}
	other := NewNutanixResourceIdentifierUUID(id)
	if !nri.EqualTo(&other) {
		t.Fatalf("expected EqualTo for same uuid")
	}
	different := NewNutanixResourceIdentifierUUID(uuid.NewString())
	if nri.EqualTo(&different) {
		t.Fatalf("did not expect EqualTo for different uuid")
	}
}

func TestNutanixResourceIdentifierName(t *testing.T) {
	nri := NewNutanixResourceIdentifierName("coke")

	if !nri.IsName() {
		t.Fatalf("expected IsName")
	}
	if nri.IsUUID() {
		t.Fatalf("did not expect IsUUID")
	}
	if got := nri.String(); got != "coke" {
		t.Fatalf("String() = %q, want coke", got)
	}
	if got := nri.DisplayString(); got != `name="coke"` {
		t.Fatalf("DisplayString() = %q", got)
	}
	other := NewNutanixResourceIdentifierName("coke")
	if !nri.EqualTo(&other) {
		t.Fatalf("expected EqualTo for same name")
	}
	uuidID := NewNutanixResourceIdentifierUUID(uuid.NewString())
	if nri.EqualTo(&uuidID) {
		t.Fatalf("did not expect EqualTo across types")
	}
	if nri.EqualTo(nil) {
		t.Fatalf("did not expect EqualTo nil")
	}
}

func TestNutanixResourceIdentifierEmpty(t *testing.T) {
	nri := NutanixResourceIdentifier{Type: NutanixIdentifierUUID}
	if nri.String() != "" {
		t.Fatalf("expected empty String for uuid without value")
	}
	if nri.DisplayString() != "unknown" {
		t.Fatalf("expected unknown DisplayString, got %q", nri.DisplayString())
	}
	nri = NutanixResourceIdentifier{Type: NutanixIdentifierName, Name: ptr.To("")}
	// Name pointer set but empty string still IsName per CAPX (non-nil).
	if !nri.IsName() {
		t.Fatalf("expected IsName when Name pointer is set")
	}
}
