+++
title = "Deploying CAIPAMN"
icon = "fa-solid fa-truck-fast"
weight = 1
+++

CAIPAMN is implemented as a CAPI IPAM provider, which means it can be deployed alongside all other CAPI
providers in the same way [using `clusterctl`]({{< ref "via-clusterctl" >}}). However, as CAIPAMN is not yet integrated
into `clusterctl`, it is necessary to first configure `clusterctl` to know about CAIPAMN before we can deploy it.

Alternatively, you can install CAIPAMN [via Helm]({{< ref "via-helm" >}}).
