// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/google/uuid"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha2"
)

var _ conversion.Convertible = &NutanixIPPool{}

// ConvertTo converts this NutanixIPPool to the Hub version (v1alpha2).
func (src *NutanixIPPool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.NutanixIPPool)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.PrismCentral = convertPrismCentralToV1Alpha2(src.Spec.PrismCentral)
	dst.Spec.Subnet = stringToResourceIdentifier(src.Spec.Subnet)
	if src.Spec.Cluster != nil {
		id := stringToResourceIdentifier(*src.Spec.Cluster)
		dst.Spec.Cluster = &id
	}
	dst.Status = v1alpha2.NutanixIPPoolStatus{}
	return nil
}

// ConvertFrom converts from the Hub version (v1alpha2) to this NutanixIPPool.
func (dst *NutanixIPPool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.NutanixIPPool)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.PrismCentral = convertPrismCentralFromV1Alpha2(src.Spec.PrismCentral)
	dst.Spec.Subnet = src.Spec.Subnet.String()
	if src.Spec.Cluster != nil {
		dst.Spec.Cluster = ptr.To(src.Spec.Cluster.String())
	} else {
		dst.Spec.Cluster = nil
	}
	dst.Status = NutanixIPPoolStatus{}
	return nil
}

func stringToResourceIdentifier(value string) v1alpha2.NutanixResourceIdentifier {
	if _, err := uuid.Parse(value); err == nil {
		return v1alpha2.NewNutanixResourceIdentifierUUID(value)
	}
	return v1alpha2.NewNutanixResourceIdentifierName(value)
}

func convertPrismCentralToV1Alpha2(in PrismCentral) v1alpha2.PrismCentral {
	out := v1alpha2.PrismCentral{
		Address: in.Address,
		Port:    in.Port,
		CredentialsSecretRef: v1alpha2.LocalSecretRef{
			Name: in.CredentialsSecretRef.Name,
		},
		Insecure: in.Insecure,
	}
	if in.AdditionalTrustBundle != nil {
		out.AdditionalTrustBundle = &v1alpha2.AdditionalTrustBundle{
			Data: append([]byte(nil), in.AdditionalTrustBundle.Data...),
		}
		if in.AdditionalTrustBundle.ConfigMapReference != nil {
			out.AdditionalTrustBundle.ConfigMapReference = &v1alpha2.LocalConfigMapRef{
				Name: in.AdditionalTrustBundle.ConfigMapReference.Name,
			}
		}
	}
	return out
}

func convertPrismCentralFromV1Alpha2(in v1alpha2.PrismCentral) PrismCentral {
	out := PrismCentral{
		Address: in.Address,
		Port:    in.Port,
		CredentialsSecretRef: LocalSecretRef{
			Name: in.CredentialsSecretRef.Name,
		},
		Insecure: in.Insecure,
	}
	if in.AdditionalTrustBundle != nil {
		out.AdditionalTrustBundle = &AdditionalTrustBundle{
			Data: append([]byte(nil), in.AdditionalTrustBundle.Data...),
		}
		if in.AdditionalTrustBundle.ConfigMapReference != nil {
			out.AdditionalTrustBundle.ConfigMapReference = &LocalConfigMapRef{
				Name: in.AdditionalTrustBundle.ConfigMapReference.Name,
			}
		}
	}
	return out
}
