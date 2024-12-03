// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go4.org/netipx"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
)

func reserveCmd(cfg *prismConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reserve",
		Short: "Reserve IP addresses in a subnet",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientParams, err := newClientParams(cfg.endpoint, cfg.username, cfg.password)
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

			reserveType := client.ReserveIPCount(1)

			if len(args) == 1 {
				ipRange := args[0]
				if !strings.Contains(ipRange, "-") {
					ipRange = ipRange + "-" + ipRange
				}

				reserveType, err = client.ReserveIPRange(ipRange)
				if err != nil {
					return fmt.Errorf("failed to create reserve IP range: %w", err)
				}
			}

			for {
				ips, err := pcClient.Networking().ReserveIP(
					reserveType,
					cfg.subnet,
					client.ReserveIPOpts{
						AsyncTaskOpts: client.AsyncTaskOpts{
							RequestID: requestID.String(),
						},
						Cluster: cfg.cluster,
					},
				)
				if err != nil {
					if errors.Is(err, client.ErrTaskOngoing) {
						time.Sleep(1 * time.Second)
						continue
					}

					return fmt.Errorf("failed to reserve IP: %w", err)
				}

				switch len(ips) {
				case 1:
					fmt.Println(ips[0].String())
				default:
					ipSetBuilder := &netipx.IPSetBuilder{}
					for _, ip := range ips {
						netIP, err := netip.ParseAddr(ip.String())
						if err != nil {
							return fmt.Errorf("failed to parse IP address: %w", err)
						}
						ipSetBuilder.Add(netIP)
					}

					ipSet, err := ipSetBuilder.IPSet()
					if err != nil {
						return fmt.Errorf("failed to create IP set: %w", err)
					}

					ipRanges := ipSet.Ranges()
					if len(ipRanges) != 1 {
						return fmt.Errorf("expected exactly one IP range, got %d", len(ipRanges))
					}

					fmt.Println(ipRanges[0].String())
				}

				return nil
			}
		},
	}

	return cmd
}
