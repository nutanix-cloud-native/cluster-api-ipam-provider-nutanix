# Copyright 2024 Nutanix. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

.PHONY: dev.run-on-kind
dev.run-on-kind: export KUBECONFIG := $(KIND_KUBECONFIG)
dev.run-on-kind: kind.create clusterctl.init
ifndef SKIP_BUILD
dev.run-on-kind: release-snapshot
endif
dev.run-on-kind: SNAPSHOT_IMAGE = $(shell gojq -r '.[] | select(.type == "Docker Manifest").name | ltrimstr("index.docker.io/library/")' dist/artifacts.json)
dev.run-on-kind:
	kind load docker-image --name $(KIND_CLUSTER_NAME) $(SNAPSHOT_IMAGE)
	kustomize build ./config/default | \
	  sed 's|image: .\+$$|image: $(SNAPSHOT_IMAGE)|' | \
	  sed 's|imagePullPolicy: .\+$$|imagePullPolicy: IfNotPresent|' | \
	  kubectl apply --server-side -f -

.PHONY: release-please
release-please:
ifneq ($(GIT_CURRENT_BRANCH),main)
	$(error "release-please should only be run on the main branch")
else
	release-please release-pr \
	  --repo-url $(GITHUB_ORG)/$(GITHUB_REPOSITORY) --token "$$(gh auth token)"
endif
