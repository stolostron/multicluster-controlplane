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

external_host_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${management_cluster}-control-plane)

echo "##### Deploy etcd in the cluster $management_cluster ..."
cp $REPO_DIR/hack/deploy/etcd/statefulset.yaml $REPO_DIR/hack/deploy/etcd/statefulset.yaml.tmp
${SED} -i "s/gp2/standard/g" $REPO_DIR/hack/deploy/etcd/statefulset.yaml
pushd ${REPO_DIR}
export KUBECONFIG=${kubeconfig}
make deploy-etcd
unset KUBECONFIG
popd
mv $REPO_DIR/hack/deploy/etcd/statefulset.yaml.tmp $REPO_DIR/hack/deploy/etcd/statefulset.yaml

echo "##### Deploy multicluster controlplanes ..."
for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  name=multicluster-controlplane-$i
  deploy_dir=${controlplane_deploy_dir}/$name

  echo "Deploy multicluster controlplane $name ..."
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
etcd:
  mode: external
  prefix: $name
  caFile: /controlplane_config/etcd-ca.crt
  certFile: /controlplane_config/etcd-client.crt
  keyFile: /controlplane_config/etcd-client.key
  servers:
  - http://etcd-0.etcd.multicluster-controlplane-etcd:2379
  - http://etcd-1.etcd.multicluster-controlplane-etcd:2379
  - http://etcd-2.etcd.multicluster-controlplane-etcd:2379
EOF
  ${SED} -i "s@ocmconfig.yaml@${deploy_dir}/ocmconfig.yaml@g" $deploy_dir/kustomization.yaml

  kubectl --kubeconfig ${kubeconfig} delete ns $name --ignore-not-found --wait
  kubectl --kubeconfig ${kubeconfig} create ns $name

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

  kustomize build $deploy_dir | kubectl --kubeconfig ${kubeconfig} -n $name apply -f -

  wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "kubectl --kubeconfig $kubeconfig -n $name get secrets multicluster-controlplane-kubeconfig &> /dev/null" ; do sleep 1; done
  hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
  kubectl --kubeconfig $kubeconfig -n $name get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > ${hubkubeconfig}
  kubectl --kubeconfig ${hubkubeconfig} config set-cluster multicluster-controlplane --server=https://${external_host_ip}:3008$i
done

echo "##### Create multicluster controlplane managed cluters ..."
for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  managed_cluster_name="controlplane$i-mc"
  echo "Create a managed cluster $managed_cluster_name with kind ..."
  kind create cluster --name $managed_cluster_name --kubeconfig $cluster_dir/$managed_cluster_name.kubeconfig
if [ "$LOAD_IMAGE" = true ]; then
  echo "Load $IMAGE_NAME to the cluster $managed_cluster_name ..."
  kind load docker-image $IMAGE_NAME --name $managed_cluster_name
fi
done

echo "##### Deploy multicluster controlplane agnets on the managed cluters ..."
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

for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  hosted_cluster_name="controlplane$i-hosted-mc"
  
  hubkubeconfig="${cluster_dir}/controlplane$i.kubeconfig"
  spoke_kubeconfig="${cluster_dir}/spoke.kubeconfig"

  echo "##### Deploy hosted cluster $hosted_cluster_name on $hubkubeconfig ..."

  # prepare managed cluster kubeconfig secret
  cp $kubeconfig $spoke_kubeconfig
  kubectl --kubeconfig $spoke_kubeconfig config set-cluster kind-${management_cluster} --server=https://${management_cluster}-control-plane:6443
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
