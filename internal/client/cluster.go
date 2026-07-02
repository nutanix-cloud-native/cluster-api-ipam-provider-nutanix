// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/nutanix-cloud-native/prism-go-client/converged"
	convergedv4 "github.com/nutanix-cloud-native/prism-go-client/converged/v4"
)

type ClusterClient interface {
	GetCluster(ctx context.Context, cluster string) (*Cluster, error)
}

type Cluster struct {
	extID uuid.UUID
}

func (c *Cluster) ExtID() uuid.UUID {
	return c.extID
}

func (c *client) Cluster() ClusterClient {
	return &clusterClient{
		v4Client: c.v4Client,
		client:   c,
	}
}

type clusterClient struct {
	v4Client *convergedv4.Client
	client   Client
}

func (n *clusterClient) GetCluster(ctx context.Context, cluster string) (*Cluster, error) {
	clusterUUID, err := uuid.Parse(cluster)
	if err == nil {
		return n.getClusterByExtID(ctx, clusterUUID)
	}

	return n.getClusterByName(ctx, cluster)
}

func (n *clusterClient) getClusterByName(
	ctx context.Context,
	clusterName string,
) (*Cluster, error) {
	apiClusters, err := n.v4Client.Clusters.List(
		ctx,
		converged.WithFilter(fmt.Sprintf(`name eq '%s'`, clusterName)),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find cluster uuid for cluster %s: %w",
			clusterName,
			err,
		)
	}
	if apiClusters == nil {
		return nil, fmt.Errorf("no cluster found with name %q", clusterName)
	}

	if len(apiClusters) == 0 {
		return nil, fmt.Errorf("no cluster found with name %q", clusterName)
	}
	if len(apiClusters) > 1 {
		return nil, fmt.Errorf(
			"multiple clusters (%d) found with name %q",
			len(apiClusters),
			clusterName,
		)
	}
	if apiClusters[0].ExtId == nil {
		return nil, fmt.Errorf("no extID found for cluster %q", clusterName)
	}

	extID := *apiClusters[0].ExtId
	clusterUUID, err := uuid.Parse(extID)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to parse cluster uuid %q for cluster %q: %w",
			extID,
			clusterName,
			err,
		)
	}

	return &Cluster{
		extID: clusterUUID,
	}, nil
}

func (n *clusterClient) getClusterByExtID(
	ctx context.Context,
	clusterExtID uuid.UUID,
) (*Cluster, error) {
	apiCluster, err := n.v4Client.Clusters.Get(ctx, clusterExtID.String())
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find cluster with extID %q: %w",
			clusterExtID,
			err,
		)
	}
	if apiCluster == nil {
		return nil, fmt.Errorf("no cluster found with extID %q", clusterExtID)
	}

	if apiCluster.ExtId == nil {
		return nil, fmt.Errorf("no extID found for cluster %q", clusterExtID)
	}
	clusterUUID, err := uuid.Parse(*apiCluster.ExtId)
	if err != nil {
		return nil,
			fmt.Errorf(
				"failed to parse cluster extID %q for cluster %q: %w",
				*apiCluster.ExtId,
				clusterExtID,
				err,
			)
	}

	return &Cluster{
		extID: clusterUUID,
	}, nil
}
