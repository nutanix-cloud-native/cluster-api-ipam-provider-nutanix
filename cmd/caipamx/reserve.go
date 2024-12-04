// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go4.org/netipx"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
)

func reserveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reserve",
		Short: "Reserve IP addresses in a subnet",
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

			requestID, err := uuid.NewV7()
			if err != nil {
				return fmt.Errorf("failed to generate request ID: %w", err)
			}

			var reserveType client.IPReservationTypeFunc

			switch {
			case len(args) == 0:
				reserveType = client.ReserveIPCountFunc(1)
			case len(args) == 1 && strings.Contains(args[0], "-"):
				reserveType, err = client.ReserveIPRangeFunc(args[0])
				if err != nil {
					return fmt.Errorf("failed to create reserve IP range: %w", err)
				}
			default:
				reserveType, err = client.ReserveIPListFunc(args...)
				if err != nil {
					return fmt.Errorf("failed to create reserve IP list: %w", err)
				}
			}

			subnet := viper.GetString("subnet")
			cluster := viper.GetString("cluster")

			var ips []netip.Addr

			err = wait.PollUntilContextTimeout(
				context.Background(),
				time.Second,
				time.Minute,
				true,
				func(ctx context.Context) (bool, error) {
					var err error
					ips, err = pcClient.Networking().ReserveIP(
						reserveType,
						subnet,
						client.ReserveIPOpts{
							AsyncTaskOpts: client.AsyncTaskOpts{
								RequestID: requestID.String(),
							},
							Cluster: cluster,
						},
					)
					if err != nil {
						if errors.Is(err, client.ErrTaskOngoing) {
							return false, nil
						}

						return false, fmt.Errorf("failed to reserve IP: %w", err)
					}

					return true, nil
				},
			)
			if err != nil {
				return err
			}

			// The ReserveIP API call returns each IP address that has been reserved. Using an IPSet allows us to display the
			// returned IPs in a user friendly way, either displaying individual IPs or ranges that have been reserved.
			ipSetBuilder := &netipx.IPSetBuilder{}
			for _, ip := range ips {
				netIP, err := netip.ParseAddr(ip.String())
				if err != nil {
					return fmt.Errorf("failed to parse IP address: %w", err)
				}
				ipSetBuilder.Add(netIP)
			}

			// Create the IPSet that can then be converted to IP ranges for pretty printing.
			ipSet, err := ipSetBuilder.IPSet()
			if err != nil {
				return fmt.Errorf("failed to create IP set: %w", err)
			}

			// Loop through all the constructed ranges. If a range only represents a single IP then just print that single
			// IP. If a range represents multiple IPs then print the range.
			for _, ipRange := range ipSet.Ranges() {
				if ipRange.From() == ipRange.To() {
					fmt.Println(ipRange.From().String())
					continue
				}

				fmt.Println(ipRange.String())
			}

			return nil
		},
	}

	return cmd
}
