// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/wait"

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

			requestID, err := uuid.NewV7()
			if err != nil {
				return fmt.Errorf("failed to generate request ID: %w", err)
			}

			err = wait.PollUntilContextTimeout(
				context.Background(),
				time.Second,
				time.Minute,
				true,
				func(ctx context.Context) (bool, error) {
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
							return false, nil
						}

						return false, fmt.Errorf("failed to unreserve IP: %w", err)
					}

					return true, nil
				},
			)

			return err
		},
	}

	return cmd
}
