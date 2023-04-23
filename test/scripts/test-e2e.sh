#!/usr/bin/env bash
REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"
source ${REPO_DIR}/test/scripts/init.sh

set -o nounset
set -o pipefail
set -o errexit

echo "##### Build e2e test ..."
go test -c ${REPO_DIR}/test/e2e -mod=vendor -o ${REPO_DIR}/bin/e2e.test

for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  export CONTROLPLANE_NAME="controlplane$i"
  export MANAGED_CLUSTER_NAMESPACE="controlplane$i-mc"

  export CONTROLPLANE_KUBECONFIG="${cluster_dir}/controlplane$i.kubeconfig"
  export MANAGED_CLUSTER_KUBECONFIG="${cluster_dir}/controlplane$i-mc.kubeconfig"

  echo "##### Run e2e test on ${CONTROLPLANE_NAME} ..."
  ${REPO_DIR}/bin/e2e.test -test.v -ginkgo.v

  unset CONTROLPLANE_NAME
  unset MANAGED_CLUSTER_NAMESPACE
  unset CONTROLPLANE_KUBECONFIG
  unset MANAGED_CLUSTER_KUBECONFIG
done
