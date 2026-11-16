// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import v1alpha2 "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha2"

func resourceIdentifierString(id *v1alpha2.NutanixResourceIdentifier) string {
	if id == nil {
		return ""
	}
	return id.String()
}
