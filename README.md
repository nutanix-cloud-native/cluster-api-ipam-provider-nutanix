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
