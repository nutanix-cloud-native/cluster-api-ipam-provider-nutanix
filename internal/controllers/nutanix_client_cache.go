// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"

	coreinformers "k8s.io/client-go/informers/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nutanix-cloud-native/prism-go-client/environment"
	"github.com/nutanix-cloud-native/prism-go-client/environment/credentials"
	"github.com/nutanix-cloud-native/prism-go-client/environment/providers/kubernetes"
	"github.com/nutanix-cloud-native/prism-go-client/environment/types"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
)

type clientCacheParams struct {
	managementEndpoint types.ManagementEndpoint
	key                string
}

var _ client.CachedClientParams = &clientCacheParams{}

func newClientCacheParams(
	prismEndpoint credentials.NutanixPrismEndpoint,
	secretInformer coreinformers.SecretInformer,
	cmInformer coreinformers.ConfigMapInformer,
	pool genericNutanixIPPool,
) (client.CachedClientParams, error) {
	env := environment.NewEnvironment(kubernetes.NewProvider(
		prismEndpoint,
		secretInformer,
		cmInformer,
	))

	me, err := env.GetManagementEndpoint(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get management endpoint: %w", err)
	}

	return &clientCacheParams{
		key:                ctrlclient.ObjectKeyFromObject(pool).String(),
		managementEndpoint: *me,
	}, nil
}

func (p *clientCacheParams) ManagementEndpoint() types.ManagementEndpoint {
	return p.managementEndpoint
}

func (p *clientCacheParams) Key() string {
	return p.key
}
