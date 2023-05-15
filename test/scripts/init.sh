#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"

# only render the controlplane helm chart
export ONLY_RENDER="true"

IMAGE_NAME=${IMAGE_NAME:-quay.io/stolostron/multicluster-controlplane:latest}
LOAD_IMAGE=${LOAD_IMAGE:-false}

CONTROLPLANE_NUMBER=${CONTROLPLANE_NUMBER:-2}

output="${REPO_DIR}/_output"
cluster_dir="${output}/kubeconfig"
etd_ca_dir=${output}/etcd/deploy/cert-etcd
etc_ca="${etd_ca_dir}/ca.pem"
etc_cert="${etd_ca_dir}/client.pem"
etc_key="${etd_ca_dir}/client-key.pem"

management_cluster="management"

kubeconfig="${cluster_dir}/${management_cluster}.kubeconfig"

mkdir -p ${cluster_dir}

echo "Controlplane number : $CONTROLPLANE_NUMBER"
