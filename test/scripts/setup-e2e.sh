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
etd_ca_dir=${output}/etcd_ca
etc_ca="${etd_ca_dir}/ca.pem"
etc_cert="${etd_ca_dir}/client.pem"
etc_key="${etd_ca_dir}/client-key.pem"
cp $REPO_DIR/hack/deploy/etcd/statefulset.yaml $REPO_DIR/hack/deploy/etcd/statefulset.yaml.tmp
${SED} -i "s/gp2/standard/g" $REPO_DIR/hack/deploy/etcd/statefulset.yaml
pushd ${REPO_DIR}
export KUBECONFIG=${kubeconfig}
export CFSSL_DIR=${etd_ca_dir}
make deploy-etcd
unset KUBECONFIG
popd
mv $REPO_DIR/hack/deploy/etcd/statefulset.yaml.tmp $REPO_DIR/hack/deploy/etcd/statefulset.yaml

echo "##### Deploy multicluster controlplanes ..."
external_host_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${management_cluster}-control-plane)

for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  namespace=multicluster-controlplane-$i
  external_host_port="3008$i"

  kubectl --kubeconfig $kubeconfig create namespace $namespace

  helm template multicluster-controlplane charts/multicluster-controlplane \
    -n $namespace \
    --set route.enabled=false \
    --set nodeport.enabled=true \
    --set nodeport.port=${external_host_port} \
    --set apiserver.externalHostname=${external_host_ip} \
    --set image=${IMAGE_NAME} \
    --set autoApprovalBootstrapUsers="system:admin" \
    --set etcd.mode=external \
    --set 'etcd.servers={"http://etcd-0.etcd.multicluster-controlplane-etcd:2379","http://etcd-1.etcd.multicluster-controlplane-etcd:2379","http://etcd-2.etcd.multicluster-controlplane-etcd:2379"}' \
    --set-file etcd.ca="${etc_ca}" \
    --set-file etcd.cert="${etc_cert}" \
    --set-file etcd.certkey="${etc_key}" | kubectl --kubeconfig $kubeconfig apply -f - 

  wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $kubeconfig -n $namespace get secrets multicluster-controlplane-kubeconfig &> /dev/null" ; do sleep 1; done

  hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
  kubectl --kubeconfig $kubeconfig -n $namespace get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > ${hubkubeconfig}
  kubectl --kubeconfig ${hubkubeconfig} config set-cluster multicluster-controlplane --server=https://${external_host_ip}:${external_host_port}
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
  deploy_dir=${output}/agent/deploy/$managed_cluster_name
  mkdir -p ${deploy_dir}
  cp -r ${REPO_DIR}/hack/deploy/agent/* $deploy_dir

  agent_namespace="multicluster-controlplane-agent"
  kubectl --kubeconfig $cluster_dir/$managed_cluster_name.kubeconfig delete ns ${agent_namespace} --ignore-not-found
  kubectl --kubeconfig $cluster_dir/$managed_cluster_name.kubeconfig create ns ${agent_namespace}

  hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
  cp -f ${hubkubeconfig} ${deploy_dir}/hub-kubeconfig

  pushd $deploy_dir
  kustomize edit set image quay.io/stolostron/multicluster-controlplane=${IMAGE_NAME}
  ${SED} -i "s/cluster-name=cluster1/cluster-name=$managed_cluster_name/" $deploy_dir/deployment.yaml
  cat >> ${deploy_dir}/kustomization.yaml <<EOF
patches:
- patch: |-
    - op: add
      path: /spec/template/spec/containers/0/args/-
      value: --feature-gates=ManagedServiceAccount=true
  target:
    kind: Deployment
EOF
  popd

  kustomize build ${deploy_dir} | kubectl --kubeconfig $cluster_dir/$managed_cluster_name.kubeconfig -n ${agent_namespace} apply -f -
done

wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $hubkubeconfig get crds klusterlets.operator.open-cluster-management.io &> /dev/null" ; do sleep 1; done
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
