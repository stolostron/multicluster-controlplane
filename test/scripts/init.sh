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
controlplane_deploy_dir="${output}/controlplane/deploy"

management_cluster="controlplane-management"

kubeconfig="${cluster_dir}/${management_cluster}.kubeconfig"

SED=sed
if [ "$(uname)" = 'Darwin' ]; then
  # run `brew install gnu-${SED}` to install gsed
  SED=gsed
fi

mkdir -p ${cluster_dir}
mkdir -p ${controlplane_deploy_dir}

for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  deploy_dir=${controlplane_deploy_dir}/multicluster-controlplane-$i
  mkdir -p ${deploy_dir}
  cp -r ${REPO_DIR}/hack/deploy/controlplane/* $deploy_dir
done

echo "Controlplane number : $CONTROLPLANE_NUMBER"
