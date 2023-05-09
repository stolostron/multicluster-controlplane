#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"

IMAGE_NAME=${IMAGE_NAME:-quay.io/stolostron/multicluster-controlplane:latest}
LOAD_IMAGE=${LOAD_IMAGE:-false}

CONTROLPLANE_NUMBER=${CONTROLPLANE_NUMBER:-2}

output="${REPO_DIR}/_output"
cluster_dir="${output}/kubeconfig"
etd_ca_dir=${output}/etcd_ca
etc_ca="${etd_ca_dir}/ca.pem"
etc_cert="${etd_ca_dir}/client.pem"
etc_key="${etd_ca_dir}/client-key.pem"

management_cluster="management"

kubeconfig="${cluster_dir}/${management_cluster}.kubeconfig"

SED=sed
if [ "$(uname)" = 'Darwin' ]; then
  # run `brew install gnu-${SED}` to install gsed
  SED=gsed
fi

mkdir -p ${cluster_dir}

echo "Controlplane number : $CONTROLPLANE_NUMBER"
