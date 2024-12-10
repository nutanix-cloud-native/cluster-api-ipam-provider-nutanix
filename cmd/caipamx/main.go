// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/component-base/version"
)

func must(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	// Cleanup function to perform any necessary cleanup on exit.
	cleanup := func() error { return nil }

	rootCmd := &cobra.Command{
		Use:   "caipamx",
		Short: "CAIPAMX is a tool for reserving and unreserving IP addresses and IP address ranges in Nutanix IPAM subnets",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if u := viper.GetString("user"); u == "" {
				return fmt.Errorf(
					"user is required, either via the --user flag or the NUTANIX_USER environment variable",
				)
			}
			if p := viper.GetString("password"); p == "" {
				return fmt.Errorf(
					"password is required, either via the --password flag or the NUTANIX_PASSWORD environment variable",
				)
			}

			// If the verbose flag is not set, redirect all stderr to a file to hide PC API calls from client output.
			if !viper.GetBool("verbose") {
				tempFile, err := os.CreateTemp("", "caipamx-*")
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				// Cleanup function to remove the temporary file used to hide PC API calls from client output.
				cleanup = func() error {
					return os.Remove(tempFile.Name())
				}

				os.Stderr = tempFile
			}

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			return cleanup()
		},
		SilenceUsage: true,
	}

	rootCmd.Version = version.Get().String()
	// Set the root command's output to stderr to capture the pointer value at this point so it does not
	// use the redirected output on error even when verbose flag is false.
	rootCmd.SetErr(os.Stderr)

	persistentFlags := rootCmd.PersistentFlags()
	persistentFlags.String(
		"prism-endpoint",
		"",
		"Address of Nutanix Prism Central",
	)
	must(rootCmd.MarkPersistentFlagRequired("prism-endpoint"))
	persistentFlags.String(
		"user",
		"",
		"Username for Nutanix Prism Central (also configurable via NUTANIX_USER environment variable)",
	)
	persistentFlags.String(
		"password",
		"",
		"Password for Nutanix Prism Central (also configurable via NUTANIX_PASSWORD environment variable)",
	)
	persistentFlags.String(
		"subnet",
		"",
		"Subnet to reserve IPs in, either UUID or name",
	)
	must(rootCmd.MarkPersistentFlagRequired("subnet"))

	persistentFlags.String(
		"aos-cluster",
		"",
		"Nutanix AOS cluster to reserve IPs in, either UUID or name",
	)

	// Deprecated flag
	persistentFlags.String(
		"cluster",
		"",
		"Nutanix AOS cluster to reserve IPs in, either UUID or name",
	)
	must(persistentFlags.MarkDeprecated("cluster", "use --aos-cluster instead"))
	rootCmd.MarkFlagsMutuallyExclusive("cluster", "aos-cluster")

	persistentFlags.Bool(
		"insecure",
		false,
		"If true, the Prism Central server certificate will not be validated.",
	)

	persistentFlags.Bool(
		"verbose",
		false,
		"If true, show all PC API requests and responses.",
	)

	// Bind the flags to viper
	must(viper.BindPFlags(persistentFlags))
	// Set the viper environment variable prefix to "nutanix"
	viper.SetEnvPrefix("nutanix")
	// Bind the NUTANIX_USER and NUTANIX_PASSWORD environment variables to the viper configuration
	must(viper.BindEnv("user"))
	must(viper.BindEnv("password"))

	rootCmd.AddCommand(reserveCmd())
	rootCmd.AddCommand(unreserveCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
