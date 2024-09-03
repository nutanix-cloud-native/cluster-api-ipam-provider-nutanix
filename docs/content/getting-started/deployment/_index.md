+++
title = "Deploying CAIPAMX"
icon = "fa-solid fa-truck-fast"
weight = 1
+++

CAIPAMX is implemented as a CAPI IPAM provider, which means it can be deployed alongside all other CAPI
providers in the same way [using `clusterctl`]({{< ref "via-clusterctl" >}}). However, as CAIPAMX is not yet integrated
into `clusterctl`, it is necessary to first configure `clusterctl` to know about CAIPAMX before we can deploy it.

Alternatively, you can install CAIPAMX [via Helm]({{< ref "via-helm" >}}).
