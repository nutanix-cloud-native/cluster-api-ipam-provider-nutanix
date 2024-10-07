+++
title = "Via clusterctl"
icon = "fa-solid fa-circle-nodes"
weight = 1
+++

Add the following to your `clusterctl.yaml` file, which is normally found at
`${XDG_CONFIG_HOME}/.cluster-api/clusterctl.yaml` (or `${HOME}/cluster-api/.clusterctl.yaml`). See [clusterctl
configuration file] for more details. If the `providers` section already exists, add the entry and omit the `providers`
key from this block below:

```yaml
providers:
  - name: "caipamx"
    url: "https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/releases/download/v{{< param "version" >}}/ipam-components.yaml"
    type: "IPAMProvider"
```

we can deploy CAIPAMX and other necessary providers (update infrastructure providers for your needs), leaving all
configuration values blank as we will specify these when creating clusters:

```shell
clusterctl init \
  --infrastructure nutanix \
  --ipam caipamx:v{{< param "version" >}} \
  --wait-providers
```

[clusterctl configuration file]: https://cluster-api.sigs.k8s.io/clusterctl/configuration.html?highlight=clusterctl%20config#variables
