// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/url"

	"github.com/spf13/viper"

	"github.com/nutanix-cloud-native/prism-go-client/environment/types"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
)

type clientParams struct {
	endpoint *url.URL
	username string
	password string
	insecure bool
}

var _ client.CachedClientParams = &clientParams{}

func newClientParams() (*clientParams, error) {
	endpointURL, err := url.Parse(viper.GetString("prism-endpoint"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint URL: %w", err)
	}

	return &clientParams{
		endpoint: endpointURL,
		username: viper.GetString("user"),
		password: viper.GetString("password"),
		insecure: viper.GetBool("insecure"),
	}, nil
}

func (c *clientParams) ManagementEndpoint() types.ManagementEndpoint {
	return types.ManagementEndpoint{
		Address: c.endpoint,
		ApiCredentials: types.ApiCredentials{
			Username: c.username,
			Password: c.password,
		},
		Insecure: c.insecure,
	}
}

func (c *clientParams) Key() string {
	// Can be anything, only used once here.
	return c.endpoint.String()
}
