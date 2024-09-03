<!--
 Copyright 2024 Nutanix. All rights reserved.
 SPDX-License-Identifier: Apache-2.0
 -->

# cluster-api-ipam-provider-nutanix

![Version: v0.0.0-dev](https://img.shields.io/badge/Version-v0.0.0--dev-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.0.0-dev](https://img.shields.io/badge/AppVersion-v0.0.0--dev-informational?style=flat-square)

A Helm chart for cluster-api-ipam-provider-nutanix

**Homepage:** <https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| jimmidyson | <jimmidyson@gmail.com> | <https://eng.d2iq.com> |

## Source Code

* <https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| deployment.replicas | int | `1` |  |
| env | object | `{}` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.repository | string | `"ghcr.io/nutanix-cloud-native/cluster-api-ipam-provider-nutanix"` |  |
| image.tag | string | `""` |  |
| imagePullSecrets | list | `[]` | Optional secrets used for pulling the container image |
| leaderElection.enabled | bool | `true` |  |
| leaderElection.leaseID | string | `""` |  |
| leaderElection.leaseNamespace | string | `""` |  |
| maxConcurrentReconciles | int | `10` |  |
| maxRequeueDelay | string | `"10s"` |  |
| minRequeueDelay | string | `"500ms"` |  |
| nodeSelector | object | `{}` |  |
| priorityClassName | string | `"system-cluster-critical"` | Priority class to be used for the pod. |
| resources.limits.cpu | string | `"100m"` |  |
| resources.limits.memory | string | `"256Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"128Mi"` |  |
| securityContext.runAsUser | int | `65532` |  |
| tolerations | list | `[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Equal"},{"effect":"NoSchedule","key":"node-role.kubernetes.io/control-plane","operator":"Equal"}]` | Kubernetes pod tolerations |
