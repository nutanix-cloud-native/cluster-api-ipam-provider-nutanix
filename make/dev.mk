# Copyright 2024 Nutanix. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

.PHONY: dev.run-on-kind
dev.run-on-kind: export KUBECONFIG := $(KIND_KUBECONFIG)
dev.run-on-kind: kind.create clusterctl.init
ifndef SKIP_BUILD
dev.run-on-kind: release-snapshot
endif
dev.run-on-kind: SNAPSHOT_VERSION = $(shell gojq -r '.version+"-"+.runtime.goarch' dist/metadata.json)
dev.run-on-kind:
	kind load docker-image --name $(KIND_CLUSTER_NAME) \
		ko.local/$(GITHUB_REPOSITORY):$(SNAPSHOT_VERSION)
	helm upgrade --install $(GITHUB_REPOSITORY) ./charts/$(GITHUB_REPOSITORY) \
		--set-string image.repository=ko.local/$(GITHUB_REPOSITORY) \
		--set-string image.tag=$(SNAPSHOT_VERSION) \
		--wait --wait-for-jobs
	kubectl rollout restart deployment $(GITHUB_REPOSITORY)
	kubectl rollout status deployment $(GITHUB_REPOSITORY)

.PHONY: release-please
release-please:
ifneq ($(GIT_CURRENT_BRANCH),main)
	$(error "release-please should only be run on the main branch")
else
	release-please release-pr \
	  --repo-url $(GITHUB_ORG)/$(GITHUB_REPOSITORY) --token "$$(gh auth token)"
endif
