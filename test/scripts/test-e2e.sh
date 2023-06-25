#!/usr/bin/env bash
REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"
source ${REPO_DIR}/test/scripts/init.sh

set -o nounset
set -o pipefail
set -o errexit

echo "##### Build e2e test ..."
go test -c ${REPO_DIR}/test/e2e -mod=vendor -o ${REPO_DIR}/bin/e2e.test

echo "##### Run default and hosted loopback test ..."
for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  export CONTROLPLANE_NAMESPACE="multicluster-controlplane-$i"
  export MANAGED_CLUSTER="controlplane$i-mc"
  export HOSTED_MANAGED_CLUSTER="controlplane$i-hosted-mc"

  export MANAGEMENT_KUBECONFIG="${kubeconfig}"
  export CONTROLPLANE_KUBECONFIG="${cluster_dir}/controlplane$i.kubeconfig"
  export MANAGED_CLUSTER_KUBECONFIG="${cluster_dir}/${MANAGED_CLUSTER}.kubeconfig"
  export HOSTED_MANAGED_CLUSTER_KUBECONFIG="${cluster_dir}/${HOSTED_MANAGED_CLUSTER}.kubeconfig"

  echo "##### Run default e2e test on ${CONTROLPLANE_NAMESPACE} ..."
  ${REPO_DIR}/bin/e2e.test --ginkgo.v --ginkgo.skip-file="selfmanagement_loopback_test.go"

  unset HOSTED_MANAGED_CLUSTER_KUBECONFIG
  unset MANAGED_CLUSTER_KUBECONFIG
  unset CONTROLPLANE_KUBECONFIG
  unset MANAGEMENT_KUBECONFIG
  unset HOSTED_MANAGED_CLUSTER
  unset MANAGED_CLUSTER
  unset CONTROLPLANE_NAMESPACE
done

# Run the self management loopback test lastly to avoid to escalate the controlplane permissions
echo "##### Run self management loopback test ..."
# deploy a controlplane with self-management
external_host_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${management_cluster}-control-plane)

kubectl --kubeconfig $kubeconfig create namespace multicluster-controlplane

pushd ${REPO_DIR}
export EXTERNAL_HOSTNAME=${external_host_ip}
export NODE_PORT="30080"
export SELF_MANAGEMENT="true"
make deploy
unset EXTERNAL_HOSTNAME
unset NODE_PORT
unset SELF_MANAGEMENT
popd

kubectl --kubeconfig $kubeconfig apply -f ${REPO_DIR}/_output/controlplane/multicluster-controlplane.yaml

wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $kubeconfig -n multicluster-controlplane get secrets multicluster-controlplane-kubeconfig &> /dev/null" ; do sleep 1; done

kubectl --kubeconfig $kubeconfig -n multicluster-controlplane get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > "${cluster_dir}"/controlplane.kubeconfig

wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig "${cluster_dir}"/controlplane.kubeconfig get crds policies.policy.open-cluster-management.io &> /dev/null" ; do sleep 1; done

# waiting the self agent is started
# TODO find a way to indicate this from controlplane
sleep 60

# prepare policies
kubectl --kubeconfig "${cluster_dir}"/controlplane.kubeconfig apply -f $REPO_DIR/test/e2e/testdata/limitrange-policy-placement.yaml

export MANAGEMENT_KUBECONFIG="${kubeconfig}"
export SELF_CONTROLPLANE_KUBECONFIG="${cluster_dir}/controlplane.kubeconfig"
${REPO_DIR}/bin/e2e.test --ginkgo.v --ginkgo.focus-file="selfmanagement_loopback_test.go"
unset SELF_CONTROLPLANE_KUBECONFIG
unset MANAGEMENT_KUBECONFIG
