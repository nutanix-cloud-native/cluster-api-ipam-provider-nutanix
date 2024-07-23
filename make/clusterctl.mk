# Copyright 2024 Nutanix. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

export CAPI_VERSION := $(shell GOWORK=off go list -m -f '{{ .Version }}' sigs.k8s.io/cluster-api)

# Leave Nutanix credentials empty here and set it when creating the clusters
.PHONY: clusterctl.init
clusterctl.init:
	clusterctl init \
	  --kubeconfig=$(KIND_KUBECONFIG) \
	  --core cluster-api:$(CAPI_VERSION) \
	  --bootstrap kubeadm:$(CAPI_VERSION) \
	  --control-plane kubeadm:$(CAPI_VERSION) \
	  --wait-providers

.PHONY: clusterctl.delete
clusterctl.delete:
	clusterctl delete --kubeconfig=$(KIND_KUBECONFIG) --all
