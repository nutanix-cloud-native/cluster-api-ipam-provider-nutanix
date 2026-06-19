// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"

	convergedv4 "github.com/nutanix-cloud-native/prism-go-client/converged/v4"
	"github.com/nutanix-cloud-native/prism-go-client/environment/types"
	v4sdk "github.com/nutanix-cloud-native/prism-go-client/v4"
)

var v4ClientCache = convergedv4.NewClientCache(v4sdk.WithSessionAuth(true))

type CachedClientParams = types.CachedClientParams

func GetClient(params CachedClientParams) (Client, error) {
	v4Client, err := v4ClientCache.GetOrCreate(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create converged v4 API client: %w", err)
	}
	return &client{v4Client: v4Client}, nil
}

type Client interface {
	Networking() NetworkingClient
	Cluster() ClusterClient
}

type client struct {
	v4Client *convergedv4.Client
}
