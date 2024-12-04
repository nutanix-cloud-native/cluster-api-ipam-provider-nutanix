// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
)

func unreserveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unreserve",
		Short: "Unreserve IP addresses in a subnet",
		Args:  cobra.MinimumNArgs(1),
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
			switch len(args) {
			case 1:
				if strings.Contains(args[0], "-") {
					unreserveType, err = client.UnreserveIPRange(args[0])
				} else {
					unreserveType, err = client.UnreserveIPList(args...)
				}
			default:
				unreserveType, err = client.UnreserveIPList(args...)
			}
			if err != nil {
				return fmt.Errorf("failed to create unreserve IP request: %w", err)
			}

			requestID, err := uuid.NewV7()
			if err != nil {
				return fmt.Errorf("failed to generate request ID: %w", err)
			}

			for {
				err := pcClient.Networking().UnreserveIP(
					unreserveType,
					viper.GetString("subnet"),
					client.UnreserveIPOpts{
						AsyncTaskOpts: client.AsyncTaskOpts{
							RequestID: requestID.String(),
						},
						Cluster: viper.GetString("cluster"),
					},
				)
				if err != nil {
					if errors.Is(err, client.ErrTaskOngoing) {
						time.Sleep(1 * time.Second)
						continue
					}

					return fmt.Errorf("failed to unreserve IP: %w", err)
				}

				return nil
			}
		},
	}

	return cmd
}
