+++
title = "Deploying CAIPAMX"
icon = "fa-solid fa-truck-fast"
weight = 1
+++

We can deploy CAIPAMX and other necessary providers (update infrastructure providers for your needs):

```shell
clusterctl init \
  --infrastructure nutanix \
  --ipam nutanix \
  --wait-providers
```
