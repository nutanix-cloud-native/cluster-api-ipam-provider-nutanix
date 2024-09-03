+++
title = "Via Helm"
icon = "fa-solid fa-helmet-safety"
weight = 2
+++

When installing CAIPAMX via Helm, we need to deploy Cluster API core providers and any other required infrastructure
providers to our management cluster via `clusterctl`:

```shell
clusterctl init \
  --infrastructure nutanix \
  --wait-providers
```

We can then deploy CAIPAMX via Helm by adding the Helm repo and installing in the usual way via Helm:
Add the CAIPAMX Helm repo:

```shell
helm upgrade cluster-api-ipam-provider-nutanix cluster-api-ipam-provider-nutanix \
  --install \
  --repo https://nutanix-cloud-native.github.io/cluster-api-ipam-provider-nutanix/helm \
  --version v{{< param "version" >}} \
  --namespace caipamx-system \
  --create-namespace \
  --wait \
  --wait-for-jobs
```
