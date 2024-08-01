// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"
	"net"
	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/client"

	prismgoclient "github.com/nutanix-cloud-native/prism-go-client"
	"github.com/nutanix-cloud-native/prism-go-client/adapter"
	"github.com/nutanix-cloud-native/prism-go-client/environment/types"
)

type clientCacheParams struct {
	key                string
	managementEndpoint types.ManagementEndpoint
}

func newClientCacheParams(
	pool genericNutanixIPPool, credentials *prismgoclient.Credentials,
) (adapter.CachedClientParams, error) {
	mgmtURL, err := url.Parse("https://" + net.JoinHostPort(credentials.Endpoint, credentials.Port))
	if err != nil {
		return nil, fmt.Errorf("invalid PC endpoint: %w", err)
	}

	return &clientCacheParams{
		key: client.ObjectKeyFromObject(pool).String(),
		managementEndpoint: types.ManagementEndpoint{
			Address: mgmtURL,
			ApiCredentials: types.ApiCredentials{
				Username: credentials.Username,
				Password: credentials.Password,
			},
			Insecure: credentials.Insecure,
		},
	}, nil
}

func (p *clientCacheParams) ManagementEndpoint() types.ManagementEndpoint {
	return p.managementEndpoint
}

func (p *clientCacheParams) Key() string {
	return p.key
}
