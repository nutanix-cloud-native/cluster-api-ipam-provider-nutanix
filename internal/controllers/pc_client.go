// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"

	coreinformers "k8s.io/client-go/informers/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nutanix-cloud-native/prism-go-client/environment/credentials"

	v1alpha2 "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha2"
	pcclient "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
)

func getPCClientForPrismCentral(
	pc v1alpha2.PrismCentral,
	namespace string,
	cacheKeyObject ctrlclient.Object,
	secretInformer coreinformers.SecretInformer,
	cmInformer coreinformers.ConfigMapInformer,
	pcClientGetter func(pcclient.CachedClientParams) (pcclient.Client, error),
) (pcclient.Client, error) {
	var additionalTrustBundle *credentials.NutanixTrustBundleReference
	if pc.AdditionalTrustBundle != nil {
		switch {
		case len(pc.AdditionalTrustBundle.Data) > 0:
			additionalTrustBundle = &credentials.NutanixTrustBundleReference{
				Data: string(pc.AdditionalTrustBundle.Data),
				Kind: credentials.NutanixTrustBundleKindString,
			}
		case pc.AdditionalTrustBundle.ConfigMapReference != nil && pc.AdditionalTrustBundle.ConfigMapReference.Name != "":
			additionalTrustBundle = &credentials.NutanixTrustBundleReference{
				Name:      pc.AdditionalTrustBundle.ConfigMapReference.Name,
				Namespace: namespace,
				Kind:      credentials.NutanixTrustBundleKindConfigMap,
			}
		default:
			return nil, fmt.Errorf(
				"invalid additional trust bundle configuration: either data or secretRef must be set",
			)
		}
	}

	cacheClientParams, err := newClientCacheParamsForObject(
		credentials.NutanixPrismEndpoint{
			Address:               pc.Address,
			Port:                  int32(pc.Port),
			Insecure:              pc.Insecure,
			AdditionalTrustBundle: additionalTrustBundle,
			CredentialRef: &credentials.NutanixCredentialReference{
				Kind:      credentials.SecretKind,
				Name:      pc.CredentialsSecretRef.Name,
				Namespace: namespace,
			},
		},
		secretInformer,
		cmInformer,
		cacheKeyObject,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nutanix cache client params: %w", err)
	}

	c, err := pcClientGetter(cacheClientParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get Nutanix client: %w", err)
	}
	return c, nil
}
