#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# shellcheck source=hack/common.sh
source "${SCRIPT_DIR}/common.sh"

readonly PC_ADDRESS_JSONPATH='.spec.versions[].schema.openAPIV3Schema.properties.spec.properties.prismCentral.properties.address'

for crd_file in "${GIT_REPO_ROOT}"/charts/cluster-api-ipam-provider-nutanix/crds/ipam.cluster.x-k8s.io_{nutanixippools,globalnutanixippools}.yaml; do
  cat <<EOF >"${crd_file}.tmp"
$(cat "${GIT_REPO_ROOT}/hack/license-header.yaml.txt")
---
$(gojq --yaml-input --yaml-output \
    "(${PC_ADDRESS_JSONPATH}).oneOf |= [{\"format\": \"ipv4\"},{\"format\": \"ipv6\"},{\"format\": \"hostname\"}]" \
    "${crd_file}")
EOF

  mv "${crd_file}"{.tmp,}
done
