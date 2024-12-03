// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/component-base/version"
)

type prismConfiguration struct {
	username string
	password string
	endpoint string
	subnet   string
	cluster  string
}

func must(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "caipamx",
		Short: "CAIPAMX is a tool for reserving and unreserving IP addresses and IP address ranges in Nutanix IPAM subnets",
	}

	rootCmd.Version = version.Get().String()

	prismCfg := &prismConfiguration{}

	persistentFlags := rootCmd.PersistentFlags()
	persistentFlags.StringVar(
		&prismCfg.endpoint,
		"prism-endpoint",
		"",
		"Address of Nutanix Prism Central",
	)
	must(rootCmd.MarkPersistentFlagRequired("prism-endpoint"))
	persistentFlags.StringVar(
		&prismCfg.username,
		"username",
		"",
		"Username for Nutanix Prism Central",
	)
	must(rootCmd.MarkPersistentFlagRequired("username"))
	persistentFlags.StringVar(
		&prismCfg.password,
		"password",
		"",
		"Password for Nutanix Prism Central",
	)
	must(rootCmd.MarkPersistentFlagRequired("password"))
	persistentFlags.StringVar(
		&prismCfg.subnet,
		"subnet",
		"",
		"Subnet to reserve IPs in, either UUID or name",
	)
	must(rootCmd.MarkPersistentFlagRequired("subnet"))
	persistentFlags.StringVar(
		&prismCfg.cluster,
		"cluster",
		"",
		"Cluster to reserve IPs in, either UUID or name",
	)

	rootCmd.AddCommand(reserveCmd(prismCfg))
	rootCmd.AddCommand(unreserveCmd(prismCfg))

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
