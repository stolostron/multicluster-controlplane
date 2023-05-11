#!/usr/bin/env bash
REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"
source ${REPO_DIR}/test/scripts/init.sh

set -o nounset
set -o pipefail
set -o errexit

echo "##### Create a management cluster with kind ..."
kind create cluster --kubeconfig $kubeconfig --name $management_cluster
if [ "$LOAD_IMAGE" = true ]; then
  echo "Load $IMAGE_NAME to the cluster $management_cluster ..."
  kind load docker-image $IMAGE_NAME --name $management_cluster
fi

echo "##### Deploy etcd in the cluster $management_cluster ..."
pushd ${REPO_DIR}
export KUBECONFIG=${kubeconfig}
STORAGE_CLASS_NAME="standard" make deploy-etcd
unset KUBECONFIG
popd

echo "##### Deploy multicluster controlplanes ..."
external_host_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${management_cluster}-control-plane)
for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  namespace=multicluster-controlplane-$i

  kubectl --kubeconfig $kubeconfig create namespace $namespace

  pushd ${REPO_DIR}
  export HUB_NAME=${namespace}
  export EXTERNAL_HOSTNAME=${external_host_ip}
  export NODE_PORT="3008$i"
  make deploy
  unset HUB_NAME
  unset EXTERNAL_HOSTNAME
  unset NODE_PORT
  popd

  kubectl --kubeconfig $kubeconfig apply -f ${REPO_DIR}/_output/controlplane/${namespace}.yaml

  wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $kubeconfig -n $namespace get secrets multicluster-controlplane-kubeconfig &> /dev/null" ; do sleep 1; done

  kubectl --kubeconfig $kubeconfig -n $namespace get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > ${cluster_dir}/controlplane$i.kubeconfig
done

echo "##### Create and import managed cluters ..."
for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  managed_cluster_name="controlplane$i-mc"
  kind create cluster --name $managed_cluster_name --kubeconfig $cluster_dir/$managed_cluster_name.kubeconfig
  if [ "$LOAD_IMAGE" = true ]; then
    echo "Load $IMAGE_NAME to the cluster $managed_cluster_name ..."
    kind load docker-image $IMAGE_NAME --name $managed_cluster_name
  fi
done

for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  managed_cluster_name="controlplane$i-mc"
  pushd ${REPO_DIR}
  export KUBECONFIG=$cluster_dir/$managed_cluster_name.kubeconfig
  export CONTROLPLANE_KUBECONFIG="${cluster_dir}/controlplane$i.kubeconfig"
  export CLUSTER_NAME=controlplane$i-mc
  ENABLE_MANAGED_SA=true make deploy-agent
  unset KUBECONFIG
  unset CONTROLPLANE_KUBECONFIG
  unset CLUSTER_NAME
  popd
done

sleep 120

echo "##### Create and import hosted cluters ..."
for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  hosted_cluster_name="controlplane$i-hosted-mc"
  kind create cluster --name $hosted_cluster_name --kubeconfig $cluster_dir/$hosted_cluster_name.kubeconfig
  if [ "$LOAD_IMAGE" = true ]; then
    echo "Load $IMAGE_NAME to the cluster $hosted_cluster_name ..."
    kind load docker-image $IMAGE_NAME --name $hosted_cluster_name
  fi
done

for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  hosted_cluster_name="controlplane$i-hosted-mc"
  hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
  kind_kubeconfig="${cluster_dir}/$hosted_cluster_name.kind-kubeconfig"

  # prepare hosted cluster kubeconfig secret
  cp ${cluster_dir}/$hosted_cluster_name.kubeconfig $kind_kubeconfig
  kubectl --kubeconfig $kind_kubeconfig config set-cluster kind-${hosted_cluster_name} --server=https://${hosted_cluster_name}-control-plane:6443
  kubectl --kubeconfig $hubkubeconfig create namespace $hosted_cluster_name
  kubectl --kubeconfig $hubkubeconfig -n $hosted_cluster_name create secret generic managedcluster-kubeconfig --from-file kubeconfig=$kind_kubeconfig

  # apply hosted klusterlet
  cat <<EOF | kubectl --kubeconfig $hubkubeconfig apply -f -
apiVersion: operator.open-cluster-management.io/v1
kind: Klusterlet
metadata:
  name: $hosted_cluster_name
spec:
  deployOption:
    mode: Hosted
EOF
done

echo "#### Prepare policies for each controlplane ..."
for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
  kubectl --kubeconfig $hubkubeconfig apply -f $REPO_DIR/test/e2e/testdata/limitrange-policy-placement.yaml
done
