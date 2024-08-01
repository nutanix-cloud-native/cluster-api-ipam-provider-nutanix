+++
title = "Via Helm"
icon = "fa-solid fa-helmet-safety"
weight = 2
+++

When installing CAIPAMN via Helm, we need to deploy Cluster API core providers and any other required infrastructure
providers to our management cluster via `clusterctl`:

```shell
clusterctl init \
  --infrastructure nutanix \
  --wait-providers
```

We can then deploy CAIPAMN via Helm by adding the Helm repo and installing in the usual way via Helm:
Add the CAIPAMN Helm repo:

```shell
helm repo add caipamn https://nutanix-cloud-native.github.io/cluster-api-ipam-provider-nutanix/helm
helm repo update caipamn
helm upgrade --install caipamn caipamn/cluster-api-ipam-provider-nutanix \
  --version v{{< param "version" >}} \
  --namespace caipamn-system \
  --create-namespace \
  --wait \
  --wait-for-jobs
```
