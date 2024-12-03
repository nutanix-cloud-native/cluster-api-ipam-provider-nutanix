<!--
 Copyright 2024 Nutanix. All rights reserved.
 SPDX-License-Identifier: Apache-2.0
 -->

# CAPI IPAM Provider For Nutanix (CAIPAMX)

For user docs, please see [https://nutanix-cloud-native.github.io/cluster-api-ipam-provider-nutanix/].

See [upstream documentation](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220125-ipam-integration.md#ipam-provider).

## Development

Install tools

- [Devbox](https://github.com/jetpack-io/devbox?tab=readme-ov-file#installing-devbox)
- [Direnv](https://direnv.net/docs/installation.html)
- Container Runtime for your Operating System

To deploy a local build, either an initial install or to update an existing deployment, run:

```shell
make dev.run-on-kind
eval $(make kind.kubeconfig)
```

Pro-tip: to redeploy without rebuilding the binaries, images, etc (useful if you have only changed the Helm chart for
example), run:

```shell
make SKIP_BUILD=true dev.run-on-kind
```

Check the pod logs:

```shell
kubectl logs deployment/cluster-api-ipam-provider-nutanix -f
```

To delete the dev KinD cluster, run:

```shell
make kind.delete
```

### CLI

CAIPAMX provides a binary for reservation and unreservation via the CLI. The `caipamx` binary can be downloaded from
the releases page.

#### Reserving an IP

```bash
$ caipamx reserve --help

Usage:
  caipamx reserve [flags]

Flags:
  -h, --help   help for reserve

Global Flags:
      --cluster string          Cluster to reserve IPs in, either UUID or name
      --password string         Password for Nutanix Prism Central
      --prism-endpoint string   Address of Nutanix Prism Central
      --subnet string           Subnet to reserve IPs in, either UUID or name
      --username string         Username for Nutanix Prism Central
```

All flags other than `--cluster` are required.

##### Reserve a single IP in the specified subnet

```shell
caipamx reserve <FLAGS>
```

##### Reserve specific IPs in the specified subnet

```shell
caipamx reserve <FLAGS> <IP> [<IP>...]
```

##### Reserve a specific range of IPs in the specified subnet

```shell
caipamx reserve <FLAGS> <IP_FROM>-<IP-TO>
```

#### Unreserve an IP

```shell
$ caipamx unreserve --help
Unreserve IP addresses in a subnet

Usage:
  caipamx unreserve [flags]

Flags:
  -h, --help   help for unreserve

Global Flags:
      --cluster string          Cluster to reserve IPs in, either UUID or name
      --password string         Password for Nutanix Prism Central
      --prism-endpoint string   Address of Nutanix Prism Central
      --subnet string           Subnet to reserve IPs in, either UUID or name
      --username string         Username for Nutanix Prism Central
```

##### Unreserve specific IPs in the specified subnet

```shell
caipamx unreserve <FLAGS> <IP> [<IP>...]
```

##### Unreserve a specific range of IPs in the specified subnet

```shell
caipamx unreserve <FLAGS> <IP_FROM>-<IP-TO>
```
