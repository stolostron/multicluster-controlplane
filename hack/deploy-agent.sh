#!/usr/bin/env bash

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/.." ; pwd -P)"

set -o nounset
set -o pipefail
set -o errexit

source ${REPO_DIR}/hack/lib/deps.sh

check_kubectl
check_kustomize

SED=sed
if [ "$(uname)" = 'Darwin' ]; then
  # run `brew install gnu-${SED}` to install gsed
  SED=gsed
fi

managed_cluster_name=${CLUSTER_NAME:-"cluster1"}
managed_service_account=${ENABLE_MANAGED_SA:-false}
image=${IMAGE_NAME:-"quay.io/stolostron/multicluster-controlplane:latest"}
agent_namespace="multicluster-controlplane-agent"
deploy_dir=${REPO_DIR}/_output/agent/deploy/$managed_cluster_name

echo "Deploy multicluster-controlplane agent on the namespace ${agent_namespace} in the cluster ${KUBECONFIG}"
echo "Image: $image"

mkdir -p ${deploy_dir}
cp -r ${REPO_DIR}/hack/deploy/agent/* $deploy_dir

kubectl delete ns ${agent_namespace} --ignore-not-found
kubectl create ns ${agent_namespace}

cp -f ${CONTROLPLANE_KUBECONFIG} ${deploy_dir}/hub-kubeconfig

pushd $deploy_dir
kustomize edit set image quay.io/stolostron/multicluster-controlplane=${image}
${SED} -i "s/cluster1/$managed_cluster_name/" $deploy_dir/deployment.yaml
popd

kustomize build ${deploy_dir} | kubectl -n ${agent_namespace} apply -f -
