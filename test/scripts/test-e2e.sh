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
helm template multicluster-controlplane charts/multicluster-controlplane \
    -n multicluster-controlplane \
    --set route.enabled=false \
    --set nodeport.enabled=true \
    --set nodeport.port=30080 \
    --set apiserver.externalHostname=${external_host_ip} \
    --set image=${IMAGE_NAME} \
    --set enableSelfManagement=true \
    --set autoApprovalBootstrapUsers="system:admin" \
    --set etcd.mode=external \
    --set 'etcd.servers={"http://etcd-0.etcd.multicluster-controlplane-etcd:2379","http://etcd-1.etcd.multicluster-controlplane-etcd:2379","http://etcd-2.etcd.multicluster-controlplane-etcd:2379"}' \
    --set-file etcd.ca="${etc_ca}" \
    --set-file etcd.cert="${etc_cert}" \
    --set-file etcd.certkey="${etc_key}" | kubectl --kubeconfig $kubeconfig apply -f - 

wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $kubeconfig -n multicluster-controlplane get secrets multicluster-controlplane-kubeconfig &> /dev/null" ; do sleep 1; done

kubectl --kubeconfig $kubeconfig -n multicluster-controlplane get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > "${cluster_dir}"/controlplane.kubeconfig
kubectl --kubeconfig "${cluster_dir}"/controlplane.kubeconfig config set-cluster multicluster-controlplane --server=https://${external_host_ip}:30080

wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig "${cluster_dir}"/controlplane.kubeconfig get crds policies.policy.open-cluster-management.io &> /dev/null" ; do sleep 1; done

# prepare policies
kubectl --kubeconfig "${cluster_dir}"/controlplane.kubeconfig apply -f $REPO_DIR/test/e2e/testdata/limitrange-policy-placement.yaml

export MANAGEMENT_KUBECONFIG="${kubeconfig}"
export SELF_CONTROLPLANE_KUBECONFIG="${cluster_dir}/controlplane.kubeconfig"
${REPO_DIR}/bin/e2e.test --ginkgo.v --ginkgo.focus-file="selfmanagement_loopback_test.go"
unset SELF_CONTROLPLANE_KUBECONFIG
unset MANAGEMENT_KUBECONFIG
