# Copyright 2024 Nutanix. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# The GOPRIVATE environment variable controls which modules the go command considers
# to be private (not available publicly) and should therefore not use the proxy or checksum database
export GOPRIVATE ?=

ALL_GO_SUBMODULES := $(shell sh -c 'for f in */go.mod; do [ -e "$$f" ] && printf "%s\n" "$$f"; done | sort')
GO_SUBMODULES_NO_DOCS := $(filter-out $(addsuffix /go.mod,docs),$(ALL_GO_SUBMODULES))

# Always ensure that GOOS and GOARCH are unset in the evironment, otherwise this can cause issues
# with goreleaser and ko building images for the wrong platform.
override undefine GOOS
override undefine GOARCH
export CGO_ENABLED := 0

define go_test
	source <(setup-envtest use -p env $(ENVTEST_VERSION)) && \
	gotestsum \
		--jsonfile test.json \
		--junitfile junit-report.xml \
		--junitfile-testsuite-name=relative \
		--junitfile-testcase-classname=short \
		-- \
		-covermode=atomic \
		-coverprofile=coverage.out \
		-short \
		-v \
		$(if $(GOTEST_RUN),-run "$(GOTEST_RUN)") \
		./... && \
	go tool cover \
		-html=coverage.out \
		-o coverage.html
endef

.PHONY: test
test: ## Runs go tests for all modules in repository
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
test: go-generate test.root
endif
ifneq ($(words $(GO_SUBMODULES_NO_DOCS)),0)
test: go-generate $(addprefix test.,$(GO_SUBMODULES_NO_DOCS:/go.mod=))
endif

.PHONY: test.%
test.%: ## Runs go tests for a specific module
test.%: go-generate ; $(info $(M) running tests$(if $(GOTEST_RUN), matching "$(GOTEST_RUN)") for $* module)
	$(if $(filter-out root,$*),cd $* && )$(call go_test)

.PHONY: integration-test
integration-test: ## Runs integration tests for all modules in repository
integration-test: GOTEST_RUN := Integration
integration-test: test

.PHONY: integration-test.%
integration-test.%: ## Runs integration tests for a specific module
integration-test.%: GOTEST_RUN := Integration
integration-test.%: test.$*

.PHONY: bench
bench: ## Runs go benchmarks for all modules in repository
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
bench: bench.root
endif
ifneq ($(words $(GO_SUBMODULES_NO_DOCS)),0)
bench: $(addprefix bench.,$(GO_SUBMODULES_NO_DOCS:/go.mod=))
endif

.PHONY: bench.%
bench.%: ## Runs go benchmarks for a specific module
bench.%: ; $(info $(M) running benchmarks$(if $(GOTEST_RUN), matching "$(GOTEST_RUN)") for $* module)
	$(if $(filter-out root,$*),cd $* && )go test $(if $(GOTEST_RUN),-run "$(GOTEST_RUN)") -cover -v ./...

E2E_DRYRUN ?= false
E2E_VERBOSE ?= $(filter $(E2E_DRYRUN),true) # If dry-run, enable verbosity
E2E_PARALLEL_NODES ?= $(if $(filter $(E2E_DRYRUN),true),1,$(AVAILABLE_PARALLELISM)) # Ginkgo cannot dry-run in parallel
E2E_FLAKE_ATTEMPTS ?= 1
E2E_CONF_FILE ?= $(REPO_ROOT)/test/e2e/config/caipamx.yaml
E2E_CONF_FILE_ENVSUBST ?= $(basename $(E2E_CONF_FILE))-envsubst.yaml
E2E_DIR ?= $(REPO_ROOT)/.local/e2e
# Use an unreleased semver-looking provider version for the local clusterctl repository.
E2E_PROVIDER_VERSION ?= v0.5.99
E2E_IPAM_COMPONENTS ?= $(E2E_DIR)/ipam-components.yaml
E2E_IMAGE_TAG ?= e2e-$(shell git rev-parse --short HEAD)
E2E_IMAGE_REPOSITORY ?= $(LOCAL_IMAGE_REGISTRY)/cluster-api-ipam-provider-nutanix
E2E_IMAGE ?= $(E2E_IMAGE_REPOSITORY):$(E2E_IMAGE_TAG)
E2E_IMAGE_REGISTRY_SERVER ?= $(shell printf '%s' '$(LOCAL_IMAGE_REGISTRY)' | cut -d/ -f1)
export E2E_DEFAULT_KUBERNETES_VERSION ?= $(KINDEST_IMAGE_TAG)
ARTIFACTS ?= ${REPO_ROOT}/_artifacts

.PHONY: e2e-build-image
e2e-build-image: ## Builds and pushes the e2e controller image
	@test -n "$(LOCAL_IMAGE_REGISTRY)" || (echo "LOCAL_IMAGE_REGISTRY is required" && exit 1)
	@test -n "$$LOCAL_IMAGE_REGISTRY_USERNAME" || (echo "LOCAL_IMAGE_REGISTRY_USERNAME is required" && exit 1)
	@test -n "$$LOCAL_IMAGE_REGISTRY_PASSWORD" || (echo "LOCAL_IMAGE_REGISTRY_PASSWORD is required" && exit 1)
	@mkdir -p $(E2E_DIR)
	@echo "$$LOCAL_IMAGE_REGISTRY_PASSWORD" | docker login "$(E2E_IMAGE_REGISTRY_SERVER)" \
	  --username "$$LOCAL_IMAGE_REGISTRY_USERNAME" \
	  --password-stdin
	KO_DOCKER_REPO=$(E2E_IMAGE_REPOSITORY) ko build --bare --tags $(E2E_IMAGE_TAG) ./cmd/controller

.PHONY: e2e-ipam-components
e2e-ipam-components: ## Generates local IPAM provider components for e2e
	@test -n "$(LOCAL_IMAGE_REGISTRY)" || (echo "LOCAL_IMAGE_REGISTRY is required" && exit 1)
	@test -n "$$LOCAL_IMAGE_REGISTRY_USERNAME" || (echo "LOCAL_IMAGE_REGISTRY_USERNAME is required" && exit 1)
	@test -n "$$LOCAL_IMAGE_REGISTRY_PASSWORD" || (echo "LOCAL_IMAGE_REGISTRY_PASSWORD is required" && exit 1)
	@mkdir -p $(dir $(E2E_IPAM_COMPONENTS))
	kustomize build ./config/default | \
	  sed 's|image: .\+$$|image: $(E2E_IMAGE)|' | \
	  gojq --yaml-input --yaml-output \
	    'if .kind == "Deployment" and .metadata.name == "caipamx-manager" then .spec.template.spec.imagePullSecrets = [{"name": "local-image-registry"}] else . end' \
	    >$(E2E_IPAM_COMPONENTS)
	printf '%s\n' '---' >>$(E2E_IPAM_COMPONENTS)
	kubectl create secret docker-registry local-image-registry \
	  --namespace caipamx-system \
	  --docker-server="$(E2E_IMAGE_REGISTRY_SERVER)" \
	  --docker-username="$$LOCAL_IMAGE_REGISTRY_USERNAME" \
	  --docker-password="$$LOCAL_IMAGE_REGISTRY_PASSWORD" \
	  --dry-run=client \
	  --output=yaml >>$(E2E_IPAM_COMPONENTS)

.PHONY: e2e-test
e2e-test: ## Runs e2e tests
ifneq ($(wildcard test/e2e/*),)
e2e-test: e2e-build-image e2e-ipam-components
	$(info $(M) $(if $(filter $(E2E_DRYRUN), true),dry-,)running e2e tests$(if $(E2E_LABEL), labelled "$(E2E_LABEL)")$(if $(E2E_FOCUS), matching "$(E2E_FOCUS)"))
	env CAPI_VERSION="$(CAPI_VERSION)" \
	  E2E_IMAGE_TAG="$(E2E_IMAGE_TAG)" \
	  E2E_PROVIDER_VERSION="$(E2E_PROVIDER_VERSION)" \
	  E2E_IPAM_COMPONENTS="$(E2E_IPAM_COMPONENTS)" \
	  envsubst -no-unset -no-empty -i '$(E2E_CONF_FILE)' -o '$(E2E_CONF_FILE_ENVSUBST)'
	CGO_ENABLED=0 go run github.com/onsi/ginkgo/v2/ginkgo run \
		--r \
		--show-node-events \
		--trace \
		--randomize-all \
		--randomize-suites \
		--fail-on-pending \
		--keep-going \
		$(if $(filter $(E2E_VERBOSE),true),--vv) \
		--covermode=atomic \
		--coverprofile coverage-e2e.out \
		$(if $(filter $(E2E_DRYRUN), true),--dry-run) \
		--procs=$(E2E_PARALLEL_NODES) \
		--compilers=$(E2E_PARALLEL_NODES) \
		--flake-attempts=$(E2E_FLAKE_ATTEMPTS) \
		$(if $(E2E_FOCUS),--focus="$(E2E_FOCUS)") \
		$(if $(E2E_SKIP),--skip="$(E2E_SKIP)") \
		$(if $(E2E_LABEL),--label-filter="$(E2E_LABEL)") \
		$(E2E_GINKGO_FLAGS) \
		--junit-report=junit-e2e.xml \
		--json-report=report-e2e.json \
		--tags e2e \
		test/e2e/... -- \
			-e2e.artifacts-folder="$(ARTIFACTS)" \
			-e2e.bootstrap-cluster-name="$(KIND_CLUSTER_NAME)-e2e" \
			-e2e.bootstrap-kind-image="$(KINDEST_IMAGE)" \
			-e2e.config="$(E2E_CONF_FILE_ENVSUBST)" \
			$(if $(filter $(E2E_SKIP_CLEANUP),true),-e2e.skip-resource-cleanup)
	go tool cover \
	  -html=coverage-e2e.out \
	  -o coverage-e2e.html
endif

GOLANGCI_CONFIG_FILE ?= $(wildcard $(REPO_ROOT)/.golangci.y*ml)

.PHONY: lint
lint: ## Runs golangci-lint for all modules in repository
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
lint: lint.root
endif
ifneq ($(words $(GO_SUBMODULES_NO_DOCS)),0)
lint: $(addprefix lint.,$(GO_SUBMODULES_NO_DOCS:/go.mod=))
endif

.PHONY: lint.%
lint.%: ## Runs golangci-lint for a specific module
lint.%: golines.% ; $(info $(M) linting $* module)
	$(if $(filter-out root,$*),cd $* && )golangci-lint run --fix --config=$(GOLANGCI_CONFIG_FILE)
	$(MAKE) golines.$*

.PHONY: golines
golines: ## Runs golines for all modules in repository
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
golines: golines.root
endif
ifneq ($(words $(GO_SUBMODULES_NO_DOCS)),0)
golines: $(addprefix golines.,$(GO_SUBMODULES_NO_DOCS:/go.mod=))
endif

.PHONY: golines.%
golines.%: ## Runs golines for a specific module
golines.%:
	$(if $(filter-out root,$*),cd $* && )golines -w --ignored-dirs external $$(GOWORK=off go list -tags e2e ./... | sed "s|^$$(GOWORK=off go list -m)|.|")

.PHONY: mod-tidy
mod-tidy: ## Run go mod tidy for all modules
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
mod-tidy: mod-tidy.root
endif
ifneq ($(words $(GO_SUBMODULES_NO_DOCS)),0)
mod-tidy: $(addprefix mod-tidy.,$(GO_SUBMODULES_NO_DOCS:/go.mod=))
endif

.PHONY: mod-tidy.%
mod-tidy.%: ## Runs go mod tidy for a specific module
mod-tidy.%: ; $(info $(M) running go mod tidy for $* module)
	$(if $(filter-out root,$*),cd $(@:mod-tidy.%=%) && )go mod tidy -v
	$(if $(filter-out root,$*),cd $(@:mod-tidy.%=%) && )go mod verify

.PHONY: go-clean
go-clean: ## Cleans go build, test and modules caches for all modules
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
go-clean: go-clean.root
endif
ifneq ($(words $(ALL_GO_SUBMODULES)),0)
go-clean: $(addprefix go-clean.,$(ALL_GO_SUBMODULES:/go.mod=))
endif

.PHONY: go-clean.%
go-clean.%: ## Cleans go build, test and modules caches for a specific module
go-clean.%: ; $(info $(M) running go clean for $* module)
	$(if $(filter-out root,$*),cd $* && )go clean -r -i -cache -testcache -modcache

.PHONY: go-fix
go-fix: ## Runs go fix for all modules in repository
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
go-fix: go-fix.root
endif
ifneq ($(words $(GO_SUBMODULES_NO_DOCS)),0)
go-fix: $(addprefix go-fix.,$(GO_SUBMODULES_NO_DOCS:/go.mod=))
endif

.PHONY: go-fix.%
go-fix.%: ## Runs golangci-lint for a specific module
go-fix.%: ; $(info $(M) go fixing $* module)
	$(if $(filter-out root,$*),cd $* && )go fix ./...

.PHONY: go-generate
go-generate: ## Runs go generate
go-generate: ; $(info $(M) running go generate)
	go generate -x ./...
	controller-gen paths="./..." \
		rbac:headerFile="hack/license-header.yaml.txt",roleName=manager-role
	controller-gen paths="./api/v1alpha1" \
	  object:headerFile="hack/license-header.go.txt" \
	  crd:headerFile=hack/license-header.yaml.txt \
		output:crd:dir=config/crd/bases
	controller-gen paths="./..." \
	  webhook:headerFile="hack/license-header.yaml.txt"
	$(MAKE) golines

.PHONY: go-mod-upgrade
go-mod-upgrade: ## Interactive check for direct module dependency upgrades
go-mod-upgrade: ; $(info $(M) checking for direct module dependency upgrades)
	go-mod-upgrade

.PHONY: govulncheck
govulncheck: ## Runs go fix for all modules in repository
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
govulncheck: govulncheck.root
endif
ifneq ($(words $(GO_SUBMODULES_NO_DOCS)),0)
govulncheck: $(addprefix govulncheck.,$(GO_SUBMODULES_NO_DOCS:/go.mod=))
endif

.PHONY: govulncheck.%
govulncheck.%: ## Runs golangci-lint for a specific module
govulncheck.%: ; $(info $(M) running govulncheck on $* module)
	$(if $(filter-out root .,$*),cd $* && )govulncheck ./...

.PHONY: go-mod-edit-toolchain
go-mod-edit-toolchain: ## Edits the go.mod file of all modules in repository to use the toolchain version
ifneq ($(wildcard $(REPO_ROOT)/go.mod),)
go-mod-edit-toolchain: go-mod-edit-toolchain.root
endif
ifneq ($(words $(GO_SUBMODULES_NO_DOCS)),0)
go-mod-edit-toolchain: $(addprefix go-mod-edit-toolchain.,$(GO_SUBMODULES_NO_DOCS:/go.mod=))
endif

.PHONY: go-mod-edit-toolchain.%
go-mod-edit-toolchain.%: ## Edits the go.mod file of a specifc module in repository to use the toolchain version
go-mod-edit-toolchain.%: ; $(info $(M) setting go toolchain for $* module)
ifndef GO_TOOLCHAIN_VERSION
	$(error GO_TOOLCHAIN_VERSION is not set: please set GO_TOOLCHAIN_VERSION to the desired version, e.g. go1.22.5)
endif
	$(if $(filter-out root .,$*),cd $* && )go mod edit -toolchain=$(GO_TOOLCHAIN_VERSION)
