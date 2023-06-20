#!/usr/bin/env bash

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"
source ${REPO_DIR}/test/scripts/init.sh

HOSTED_CLUSTER_NUMBER=${HOSTED_CLUSTER_NUMBER:-1}

set -o nounset
set -o pipefail
set -o errexit

echo "##### Create a management cluster with kind ..."
kind create cluster --kubeconfig $kubeconfig --name $management_cluster
if [ "$LOAD_IMAGE" = true ]; then
  echo "Load $IMAGE_NAME to the cluster $management_cluster ..."
  kind load docker-image $IMAGE_NAME --name $management_cluster
fi

echo "##### Deploy multicluster controlplanes ..."
external_host_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${management_cluster}-control-plane)

for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  namespace=multicluster-controlplane-$i
  external_host_port="3008$i"

  helm --kubeconfig $kubeconfig upgrade --install --set args={--kubelet-insecure-tls} metrics-server metrics-server/metrics-server --namespace kube-system

  kubectl --kubeconfig $kubeconfig create namespace $namespace

  helm template multicluster-controlplane charts/multicluster-controlplane \
    -n $namespace \
    --set route.enabled=false \
    --set nodeport.enabled=true \
    --set nodeport.port=${external_host_port} \
    --set apiserver.externalHostname=${external_host_ip} \
    --set image=${IMAGE_NAME} \
    --set autoApprovalBootstrapUsers="system:admin" | kubectl --kubeconfig $kubeconfig apply -f - 

  wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $kubeconfig -n $namespace get secrets multicluster-controlplane-kubeconfig &> /dev/null" ; do sleep 1; done

  hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
  kubectl --kubeconfig $kubeconfig -n $namespace get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > ${hubkubeconfig}
  kubectl --kubeconfig ${hubkubeconfig} config set-cluster multicluster-controlplane --server=https://${external_host_ip}:${external_host_port}

  echo "##### Create and import hosted cluters ..."
  for j in $(seq 1 "${HOSTED_CLUSTER_NUMBER}"); do
    hosted_cluster_name="controlplane$i-hosted-mc$j"
    kind create cluster --name $hosted_cluster_name --kubeconfig $cluster_dir/$hosted_cluster_name.kubeconfig
    if [ "$LOAD_IMAGE" = true ]; then
      echo "Load $IMAGE_NAME to the cluster $hosted_cluster_name ..."
      kind load docker-image $IMAGE_NAME --name $hosted_cluster_name
    fi
    hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
    kind_kubeconfig="${cluster_dir}/$hosted_cluster_name.kind-kubeconfig"

    # prepare hosted cluster kubeconfig secret
    cp ${cluster_dir}/$hosted_cluster_name.kubeconfig $kind_kubeconfig

    kubectl --kubeconfig $kind_kubeconfig create ns openshift-monitoring
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-must-gather-operator
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-backplane-managed-scripts
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-config
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-route-monitor-operator
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-console
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-customer-monitoring
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-backplane
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-logging
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-operators
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-marketplace
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-dns
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-ingress
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-ingress-operator
    kubectl --kubeconfig $kind_kubeconfig create ns openshift-user-workload-monitoring
    kubectl --kubeconfig $kind_kubeconfig apply -f https://gist.githubusercontent.com/clyang82/7738fe10d687ce35f79c1d2c5e40a339/raw/a0b2f8fdd3b7b5fc37507998eb6985474b5164ec/consoles.operator.openshift.io.yaml
    kubectl --kubeconfig $kind_kubeconfig apply -f https://gist.githubusercontent.com/clyang82/7738fe10d687ce35f79c1d2c5e40a339/raw/a0b2f8fdd3b7b5fc37507998eb6985474b5164ec/monitoring.openshift.io.yaml
    kubectl --kubeconfig $kind_kubeconfig apply -f https://gist.githubusercontent.com/clyang82/7738fe10d687ce35f79c1d2c5e40a339/raw/a0b2f8fdd3b7b5fc37507998eb6985474b5164ec/oauths.config.openshift.io.yaml
    kubectl --kubeconfig $kind_kubeconfig apply -f https://gist.githubusercontent.com/clyang82/7738fe10d687ce35f79c1d2c5e40a339/raw/a0b2f8fdd3b7b5fc37507998eb6985474b5164ec/operatorgroups.operators.coreos.com.yaml
    kubectl --kubeconfig $kind_kubeconfig apply -f https://gist.githubusercontent.com/clyang82/7738fe10d687ce35f79c1d2c5e40a339/raw/a0b2f8fdd3b7b5fc37507998eb6985474b5164ec/securitycontextconstraints.security.openshift.io.yaml

    kubectl --kubeconfig $kind_kubeconfig config set-cluster kind-${hosted_cluster_name} --server=https://${hosted_cluster_name}-control-plane:6443
    kubectl --kubeconfig $hubkubeconfig create namespace $hosted_cluster_name
    kubectl --kubeconfig $hubkubeconfig -n $hosted_cluster_name create secret generic managedcluster-kubeconfig --from-file kubeconfig=$kind_kubeconfig
  done

  for j in $(seq 1 "${HOSTED_CLUSTER_NUMBER}"); do
    hosted_cluster_name="controlplane$i-hosted-mc$j"
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

  kubectl --kubeconfig ${hubkubeconfig} apply -f ${REPO_DIR}/test/performance/policies

done
