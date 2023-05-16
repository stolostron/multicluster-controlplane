# Copyright Contributors to the Open Cluster Management project
#!/usr/bin/env bash

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"

set -o nounset

management_kubeconfig=${MANAGEMENT_KUBECONFIG:-"clusters/management.kubeconfig"}
hosted_cluster1_kubeconfig=${HOSTED_CLUSTER1_KUBECONFIG:-"clusters/hosted_cluster1.kubeconfig"}
hosted_cluster2_kubeconfig=${HOSTED_CLUSTER2_KUBECONFIG:-"clusters/hosted_cluster2.kubeconfig"}
kind_cluster_kubeconfig=${KIND_CLUSTER_KUBECONFIG:-"clusters/kind_cluster.kubeconfig"}

clear

kubectl --kubeconfig ${management_kubeconfig} delete ns multicluster-controlplane --ignore-not-found
kubectl --kubeconfig ${management_kubeconfig} delete ns multicluster-controlplane-1 --ignore-not-found
kubectl --kubeconfig ${management_kubeconfig} delete ns hosted-cluster1 --ignore-not-found
kubectl --kubeconfig ${management_kubeconfig} delete ns hosted-cluster2 --ignore-not-found
kubectl --kubeconfig ${management_kubeconfig} delete limitranges container-mem-limit-range --ignore-not-found

kubectl --kubeconfig ${hosted_cluster1_kubeconfig} delete ns open-cluster-management-hosted-cluster1 --ignore-not-found
kubectl --kubeconfig ${hosted_cluster1_kubeconfig} delete limitranges container-mem-limit-range --ignore-not-found

kubectl --kubeconfig ${hosted_cluster2_kubeconfig} delete ns open-cluster-management-hosted-cluster2 --ignore-not-found
kubectl --kubeconfig ${hosted_cluster2_kubeconfig} delete limitranges container-mem-limit-range --ignore-not-found

kubectl --kubeconfig ${kind_cluster_kubeconfig} delete ns multicluster-controlplane-agent --ignore-not-found
kubectl --kubeconfig ${kind_cluster_kubeconfig} delete limitranges container-mem-limit-range --ignore-not-found
