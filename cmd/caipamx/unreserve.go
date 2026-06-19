// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
)

func unreserveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unreserve",
		Short: "Unreserve IP addresses in a subnet",
		Args:  cobra.MinimumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 && strings.Contains(args[0], "-") {
				return fmt.Errorf("only one argument is allowed when reserving an IP range")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			clientParams, err := newClientParams()
			if err != nil {
				return fmt.Errorf("failed to create client params: %w", err)
			}

			pcClient, err := client.GetClient(clientParams)
			if err != nil {
				return fmt.Errorf("failed to create Prism Central client: %w", err)
			}

			var unreserveType client.IPUnreservationTypeFunc
			switch {
			case len(args) == 1 && strings.Contains(args[0], "-"):
				unreserveType, err = client.UnreserveIPRangeFunc(args[0])
			default:
				unreserveType, err = client.UnreserveIPListFunc(args...)
			}
			if err != nil {
				return fmt.Errorf("failed to create unreserve IP request: %w", err)
			}

			aosCluster := viper.GetString("aos-cluster")
			if aosCluster == "" {
				aosCluster = viper.GetString("cluster")
			}

			// UnreserveIPs blocks until the underlying Prism task completes; bound
			// the wait so the command does not hang indefinitely.
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()

			if err := pcClient.Networking().UnreserveIPs(
				ctx,
				unreserveType,
				viper.GetString("subnet"),
				client.UnreserveIPOpts{
					Cluster: aosCluster,
				},
			); err != nil {
				return fmt.Errorf("failed to unreserve IP: %w", err)
			}

			return nil
		},
	}

	return cmd
}
