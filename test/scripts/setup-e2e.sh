#!/bin/bash

set -o nounset
set -o pipefail

SED=sed
if [ "$(uname)" = 'Darwin' ]; then
  # run `brew install gnu-${SED}` to install gsed
  SED=gsed
fi

KUBECTL=${KUBECTL:-kubectl}
KUSTOMIZE=kustomize

IMAGE_NAME=${IMAGE_NAME:-quay.io/stolostron/multicluster-controlplane:latest}
REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"
number=${CONTROLPLANE_NUMBER:-2}

output="${REPO_DIR}/_output"
cluster_dir="${output}/kubeconfig"
controlplane_deploy_dir="${output}/controlplane/deploy"
agent_deploy_dir="${output}/agent/deploy"

mkdir -p ${cluster_dir}
mkdir -p ${controlplane_deploy_dir}
mkdir -p ${agent_deploy_dir}

echo "Create a cluster with kind ..."
cluster="controlplane-hosting"
external_host_port="3008"
kubeconfig="${cluster_dir}/${cluster}.kubeconfig"
kind create cluster --kubeconfig $kubeconfig --name ${cluster}
echo "Load $IMAGE_NAME to the cluster $cluster ..."
kind load docker-image $IMAGE_NAME --name $cluster
external_host_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${cluster}-control-plane)
for i in $(seq 1 "${number}"); do
  kind create cluster --name controlplane$i-mc1 --kubeconfig $cluster_dir/controlplane$i-mc1.kubeconfig &
done
wait

echo "Deploy etcd in the cluster $cluster ..."
cp $REPO_DIR/hack/deploy/etcd/statefulset.yaml $REPO_DIR/hack/deploy/etcd/statefulset.yaml.tmp
${SED} -i "s/gp2/standard/g" $REPO_DIR/hack/deploy/etcd/statefulset.yaml
pushd ${REPO_DIR}
export KUBECONFIG=${kubeconfig}
make deploy-etcd
unset KUBECONFIG
popd
mv $REPO_DIR/hack/deploy/etcd/statefulset.yaml.tmp $REPO_DIR/hack/deploy/etcd/statefulset.yaml

for i in $(seq 1 "${number}"); do
  namespace=multicluster-controlplane-$i
  deploy_dir=${controlplane_deploy_dir}/$namespace
  mkdir -p ${deploy_dir}
  echo "Deploy standalone controlplane in namespace $namespace ..."
  cp -r ${REPO_DIR}/hack/deploy/controlplane/* $deploy_dir/

  kubectl --kubeconfig ${kubeconfig} delete ns $namespace --ignore-not-found
  kubectl --kubeconfig ${kubeconfig} create ns $namespace

  # expose apiserver
  ${SED} -i 's/ClusterIP/NodePort/' $deploy_dir/service.yaml
  ${SED} -i '/route\.yaml/d' $deploy_dir/kustomization.yaml
  ${SED} -i "/targetPort.*/a  \ \ \ \ \ \ nodePort: 3008$i" $deploy_dir/service.yaml

  # append etcd certs
  certs_dir=$deploy_dir/certs
  mkdir -p ${certs_dir}
  cp -f ${REPO_DIR}/multicluster_ca/ca.pem ${certs_dir}/etcd-ca.crt
  cp -f ${REPO_DIR}/multicluster_ca/client.pem ${certs_dir}/etcd-client.crt
  cp -f ${REPO_DIR}/multicluster_ca/client-key.pem ${certs_dir}/etcd-client.key
  ${SED} -i "$(${SED} -n  '/  - ocmconfig.yaml/=' $deploy_dir/kustomization.yaml) a \  - ${certs_dir}/etcd-client.key" $deploy_dir/kustomization.yaml
  ${SED} -i "$(${SED} -n  '/  - ocmconfig.yaml/=' $deploy_dir/kustomization.yaml) a \  - ${certs_dir}/etcd-client.crt" $deploy_dir/kustomization.yaml
  ${SED} -i "$(${SED} -n  '/  - ocmconfig.yaml/=' $deploy_dir/kustomization.yaml) a \  - ${certs_dir}/etcd-ca.crt" $deploy_dir/kustomization.yaml

  # create multicluster-controlplane configfile
  cat > ${deploy_dir}/ocmconfig.yaml <<EOF
dataDirectory: /.ocm
apiserver:
  externalHostname: $external_host_ip
  port: 9443
etcd:
  mode: external
  prefix: $namespace
  caFile: /controlplane_config/etcd-ca.crt
  certFile: /controlplane_config/etcd-client.crt
  keyFile: /controlplane_config/etcd-client.key
  servers:
  - http://etcd-0.etcd.multicluster-controlplane-etcd:2379
  - http://etcd-1.etcd.multicluster-controlplane-etcd:2379
  - http://etcd-2.etcd.multicluster-controlplane-etcd:2379
EOF
  ${SED} -i "s@ocmconfig.yaml@${deploy_dir}/ocmconfig.yaml@g" $deploy_dir/kustomization.yaml

  pushd $deploy_dir
  kustomize edit set image quay.io/stolostron/multicluster-controlplane=${IMAGE_NAME}
  ${SED} -i "s/AddonManagement=true/AddonManagement=true,ManagedServiceAccount=true/" $deploy_dir/deployment.yaml
  rm -f ${deploy_dir}/clusterrolebinding.yaml
  cat > ${deploy_dir}/clusterrolebinding.yaml <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: open-cluster-management:multicluster-controlplane-$i
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: open-cluster-management:multicluster-controlplane
subjects:
- kind: ServiceAccount
  name: multicluster-controlplane-sa
  namespace: multicluster-controlplane-$i
EOF
  popd

  kustomize build $deploy_dir | kubectl --kubeconfig ${kubeconfig} -n $namespace apply -f -

  wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $kubeconfig -n $namespace get secrets multicluster-controlplane-kubeconfig &> /dev/null" ; do sleep 1; done
  hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
  kubectl --kubeconfig $kubeconfig -n $namespace get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > ${hubkubeconfig}
  kubectl --kubeconfig ${hubkubeconfig} config set-cluster multicluster-controlplane --server=https://${external_host_ip}:3008$i

  echo "Deploy multicluster controlplane agents ..."
  managed_cluster_name="controlplane$i-mc1"
  kind load docker-image $IMAGE_NAME --name $managed_cluster_name
  deploy_dir=${agent_deploy_dir}/$managed_cluster_name
  mkdir -p ${deploy_dir}
  cp -r ${REPO_DIR}/hack/deploy/agent/* $deploy_dir

  agent_namespace="multicluster-controlplane-agent"
  kubectl --kubeconfig $cluster_dir/$managed_cluster_name.kubeconfig delete ns ${agent_namespace} --ignore-not-found
  kubectl --kubeconfig $cluster_dir/$managed_cluster_name.kubeconfig create ns ${agent_namespace}

  cp -f ${hubkubeconfig} ${deploy_dir}/hub-kubeconfig
  pushd $deploy_dir
  kustomize edit set image quay.io/stolostron/multicluster-controlplane=${IMAGE_NAME}
  ${SED} -i "s/cluster-name=cluster1/cluster-name=$managed_cluster_name/" $deploy_dir/deployment.yaml
  ${SED} -i "s/AddonManagement=true/AddonManagement=true,ManagedServiceAccount=true/" $deploy_dir/deployment.yaml
  popd
  kustomize build ${deploy_dir} | kubectl --kubeconfig $cluster_dir/$managed_cluster_name.kubeconfig -n ${agent_namespace} apply -f -

  wait_seconds="300"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $hubkubeconfig get crds klusterlets.operator.open-cluster-management.io &> /dev/null" ; do sleep 1; done

  echo "Deploy hosted cluster"
  hosted_cluster_name="controlplane$i-hosted-mc1"
  spoke_kubeconfig="${cluster_dir}/spoke.kubeconfig"
  internal_kubeconfig="${cluster_dir}/internal_controlplane$i.kubeconfig"

  # only for kind env, need an accessable bootstrap secret
  cp $hubkubeconfig $internal_kubeconfig
  kubectl --kubeconfig $internal_kubeconfig config set-cluster multicluster-controlplane --server=https://multicluster-controlplane.$namespace.svc:443
  kubectl --kubeconfig $kubeconfig -n $namespace delete secrets multicluster-controlplane-kubeconfig
  kubectl --kubeconfig $kubeconfig -n $namespace create secret generic multicluster-controlplane-kubeconfig --from-file kubeconfig=$internal_kubeconfig

  # prepare managed cluster kubeconfig secret
  cp $kubeconfig $spoke_kubeconfig
  kubectl --kubeconfig $spoke_kubeconfig config set-cluster kind-${cluster} --server=https://${cluster}-control-plane:6443
  kubectl --kubeconfig $hubkubeconfig create namespace $hosted_cluster_name
  kubectl --kubeconfig $hubkubeconfig -n $hosted_cluster_name create secret generic managedcluster-kubeconfig --from-file kubeconfig=$spoke_kubeconfig
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