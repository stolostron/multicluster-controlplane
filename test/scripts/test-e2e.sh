#!/usr/bin/env bash
REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"
source ${REPO_DIR}/test/scripts/init.sh

set -o nounset
set -o pipefail
set -o errexit

echo "##### Build e2e test ..."
go test -c ${REPO_DIR}/test/e2e -mod=vendor -o ${REPO_DIR}/bin/e2e.test

echo "##### Run e2e test ..."
for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  export CONTROLPLANE_NAMESPACE="multicluster-controlplane-$i"
  export MANAGED_CLUSTER="controlplane$i-mc"
  export HOSTED_MANAGED_CLUSTER="controlplane$i-hosted-mc"

  export MANAGEMENT_KUBECONFIG="${kubeconfig}"
  export CONTROLPLANE_KUBECONFIG="${cluster_dir}/controlplane$i.kubeconfig"
  export MANAGED_CLUSTER_KUBECONFIG="${cluster_dir}/${MANAGED_CLUSTER}.kubeconfig"
  export HOSTED_MANAGED_CLUSTER_KUBECONFIG="${cluster_dir}/${HOSTED_MANAGED_CLUSTER}.kubeconfig"

  echo "##### Run default e2e test on ${CONTROLPLANE_NAMESPACE} ..."
  ${REPO_DIR}/bin/e2e.test -test.v -ginkgo.v

  unset HOSTED_MANAGED_CLUSTER_KUBECONFIG
  unset MANAGED_CLUSTER_KUBECONFIG
  unset CONTROLPLANE_KUBECONFIG
  unset MANAGEMENT_KUBECONFIG
  unset HOSTED_MANAGED_CLUSTER
  unset MANAGED_CLUSTER
  unset CONTROLPLANE_NAMESPACE
done
