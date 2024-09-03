+++
title = "Cluster API IPAM Provider for Nutanix (CAIPAMX)"

# [[cascade]]
# type = "blog"
# toc_root = true

#   [cascade._target]
#   path = "/blog/**"

[[cascade]]
type = "docs"

  [cascade._target]
  path = "/**"
+++

Cluster API provides declarative APIs for provisioning, upgrading, and operating Kubernetes clusters across multiple
infrastructure providers. This project implements the [CAPI IPAM provider] contract to enable IP address management via
the declarative Kubernetes API for clusters running on the Nutanix platform.

This project has a restricted scope of only performing IPAM functionality, i.e. an IP address reservation is requested
via the API and CAIPAMX will do the necessary work to reserve the IP via Nutanix APIs and unreserve the IP when it is
no longer required.

It is envisaged that the following projects will integrate with CAIPAMX:

-   [CAPX] IP reservation for control plane endpoint when a cluster is created, simplifying the user experience by
    removing the need for the user to provide the control plane endpoint IP themselves at cluster creation time.
-   [Nutanix CCM] IP reservation for external load-balancer services, again simplifying the user experience by removing
    the need for the user to provide the external load-balancer provider (e.g. [MetalLB]) with an address range to
    manage itself.

[CAPI IPAM provider]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220125-ipam-integration.md#ipam-provider
[CAPX]: https://github.com/nutanix-cloud-native/cluster-api-provider-nutanix
[Nutanix CCM]: https://github.com/nutanix-cloud-native/cloud-provider-nutanix
