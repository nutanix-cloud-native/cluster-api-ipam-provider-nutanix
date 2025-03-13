# Changelog

## 0.5.0 (2025-03-13)

<!-- Release notes generated using configuration in .github/release.yaml at main -->

## What's Changed
### Exciting New Features 🎉
* feat: Add verbose flag and hide API calls by default by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/58
* feat: go 1.24.1 by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/71
* feat: Use kustomize instead of Helm for deployment by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/72
### Fixes 🔧
* fix: getByExt incorrectly expecting pointers by @dkoshkin in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/51
### Other Changes
* build: Update all tools by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/70

## New Contributors
* @dkoshkin made their first contribution in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/51

**Full Changelog**: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/compare/v0.4.0...v0.5.0

## 0.4.0 (2024-12-04)

<!-- Release notes generated using configuration in .github/release.yaml at main -->

## What's Changed
### Exciting New Features 🎉
* feat(cli): Support insecure flag for connecting to Prism by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/48
### Other Changes
* docs: Add caipamx CLI download instructions by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/45
* refactor: Rename cluster flag to aos-cluster by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/49


**Full Changelog**: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/compare/v0.3.1...v0.4.0

## 0.3.1 (2024-12-04)

<!-- Release notes generated using configuration in .github/release.yaml at main -->

## What's Changed
### Exciting New Features 🎉
* build: Fix up release archives by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/43


**Full Changelog**: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/compare/v0.3.0...v0.3.1

## 0.3.0 (2024-12-04)

<!-- Release notes generated using configuration in .github/release.yaml at main -->

## What's Changed
### Exciting New Features 🎉
* feat: Build with go 1.23.1 by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/31
* feat: go 1.23.2 and all other tooling upgrades by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/34
* feat: Add CLI tool to reserve and unreserve IP addresses by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/42
### Fixes 🔧
* fix: corrected the path in the docs by @deepakm-ntnx in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/32

## New Contributors
* @deepakm-ntnx made their first contribution in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/32

**Full Changelog**: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/compare/v0.2.0...v0.3.0

## 0.2.0 (2024-09-19)

<!-- Release notes generated using configuration in .github/release.yaml at main -->

## What's Changed
### Exciting New Features 🎉
* feat: Store ntnx API req ID as annotations on IPAddressClaim by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/27
### Fixes 🔧
* fix: Use recommended provider label from clusterctl by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/25


**Full Changelog**: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/compare/v0.1.2...v0.2.0

## 0.1.2 (2024-09-04)

<!-- Release notes generated using configuration in .github/release.yaml at main -->

## What's Changed
### Other Changes
* build: Add missing release metadata files to release artifacts by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/23


**Full Changelog**: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/compare/v0.1.1...v0.1.2

## 0.1.1 (2024-09-04)

<!-- Release notes generated using configuration in .github/release.yaml at main -->

## What's Changed
### Other Changes
* build: Fix up ko image build for release by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/21


**Full Changelog**: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/compare/v0.1.0...v0.1.1

## 0.1.0 (2024-09-04)

<!-- Release notes generated using configuration in .github/release.yaml at main -->

## What's Changed
### Exciting New Features 🎉
* feat: Build with go 1.23 by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/16
* feat: Configurable reconciliation (max concurrent and requeue delays) by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/15
### Fixes 🔧
* fix: Use least privileges for CM role by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/18
### Other Changes
* feat: Initial working Nutanix IPAM CAPI provider by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/1
* ci: Fix CI issues by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/3
* build: Add empty release-please manifest by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/4
* build: Ensure release-please manifest is valid JSON by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/5
* build: Latest prism-go-client by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/9
* refactor: Move v4 client sugar back to this project by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/13
* refactor: Update all short names from CAIPAMN to CAIPAMX by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/17
* build: Update deps to fix licensing issues by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/19
* build: Upgrade github.com/hashicorp/go-retryablehttp for CVE by @jimmidyson in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/20

## New Contributors
* @jimmidyson made their first contribution in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/1
* @dependabot made their first contribution in https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/pull/2

**Full Changelog**: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/commits/v0.1.0
