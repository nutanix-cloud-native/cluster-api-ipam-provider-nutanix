#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# shellcheck source=hack/common.sh
source "${SCRIPT_DIR}/common.sh"

readonly PC_ADDRESS_JSONPATH='.spec.versions[].schema.openAPIV3Schema.properties.spec.properties.prismCentral.properties.address'
readonly PC_ADDITIONAL_TRUST_BUNDLE_JSONPATH='.spec.versions[].schema.openAPIV3Schema.properties.spec.properties.prismCentral.properties.additionalTrustBundle'

readonly crd_files=(
  "${GIT_REPO_ROOT}"/charts/cluster-api-ipam-provider-nutanix/crds/ipam.cluster.x-k8s.io_nutanixippools.yaml
)

for crd_file in "${crd_files[@]}"; do

  cat <<EOF >"${crd_file}.tmp"
$(cat "${GIT_REPO_ROOT}/hack/license-header.yaml.txt")
---
$(gojq --yaml-input --yaml-output \
    "(${PC_ADDRESS_JSONPATH}).oneOf |= [{\"format\": \"ipv4\"},{\"format\": \"ipv6\"},{\"format\": \"hostname\"}] |
     (${PC_ADDITIONAL_TRUST_BUNDLE_JSONPATH}).oneOf |= [{\"required\": [\"trustBundleConfigMapRef\"]},{\"required\": [\"trustBundleData\"]}]" \
    "${crd_file}")
EOF

  mv "${crd_file}"{.tmp,}
done
