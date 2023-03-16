# Copyright Contributors to the Open Cluster Management project
#!/usr/bin/env bash

#################################
# include the -=magic=-
# you can pass command line args
#
# example:
# to disable simulated typing
# . ../demo-magic.sh -d
#
# pass -h to see all options
#################################
. ./demo-magic.sh


########################
# Configure the options
########################

#
# speed at which to simulate typing. bigger num = faster
#
TYPE_SPEED=60

#
# custom prompt
#
# see http://www.tldp.org/HOWTO/Bash-Prompt-HOWTO/bash-prompt-escape-sequences.html for escape sequences
#
DEMO_PROMPT="${GREEN}➜ ${CYAN}\W ${COLOR_RESET}"
ROOT_DIR="$(pwd)"
controlplane_number=${1:-1}
managedcluster_number=${2:-1}

SED=sed
if [ "$(uname)" = 'Darwin' ]; then
  # run `brew install gnu-${SED}` to install gsed
  SED=gsed
fi

if [[ "$3" == "clean" ]]; then
  for i in $(seq 1 "${controlplane_number}"); do
    namespace=multicluster-controlplane-$i
    oc delete ns $namespace
    for j in $(seq 1 "$managedcluster_number"); do
      kind delete cluster --name $namespace-mc-$j
    done
    rm -rf ${ROOT_DIR}/../deploy/cert-${namespace}
  done
  oc delete -k multicluster-global-hub-lite/deploy/server -n default
  rm -rf multicluster-global-hub-lite
  exit
fi

# text color
DEMO_CMD_COLOR=$BLACK

# hide the evidence
clear

for i in $(seq 1 "${controlplane_number}"); do

  # put your demo awesomeness here
  namespace=multicluster-controlplane-$i
  p "deploy standalone controlplane and addons(policy and managedserviceaccount) in namespace ${namespace}"
  export HUB_NAME="${namespace}"
  rm -rf ${ROOT_DIR}/../deploy/controlplane/ocmconfig.yaml
  pei "cd ../.. && make deploy"
  cd ${ROOT_DIR}
  pei "oc get pod -n ${namespace}"

  CERTS_DIR=${ROOT_DIR}/../deploy/cert-${namespace}
  mkdir ${CERTS_DIR}
  cp ${ROOT_DIR}/../../$namespace.kubeconfig ${CERTS_DIR}/controlplane-kubeconfig
  hubkubeconfig=${CERTS_DIR}/controlplane-kubeconfig
  for j in $(seq 1 "$managedcluster_number"); do
    managed_cluster_name="$namespace-mc-$j"
    p "create a KinD cluster as a managedcluster"
    pei "kind create cluster --name $managed_cluster_name --kubeconfig ${CERTS_DIR}/mc-$j-kubeconfig"
  
    agent_namespace="multicluster-controlplane-agent"
    agent_deploy_dir="${ROOT_DIR}/../deploy/agent"
    cp -f $hubkubeconfig $agent_deploy_dir/hub-kubeconfig

    # temporary solution. will be replaced once clusteradm supports it
    cp $agent_deploy_dir/deployment.yaml $agent_deploy_dir/deployment.yaml.tmp
    ${SED} -i "s/cluster-name=cluster1/cluster-name=$managed_cluster_name/" $agent_deploy_dir/deployment.yaml
    kustomize build ${ROOT_DIR}/../deploy/agent | oc --kubeconfig ${CERTS_DIR}/mc-$j-kubeconfig -n ${agent_namespace} apply -f -
    cp $agent_deploy_dir/deployment.yaml.tmp $agent_deploy_dir/deployment.yaml

    wait_seconds="90"; until [[ $((wait_seconds--)) -eq 0 ]] || eval "oc --kubeconfig $hubkubeconfig get csr --ignore-not-found | grep ^$managed_cluster_name &> /dev/null" ; do sleep 1; done
    oc --kubeconfig $hubkubeconfig get csr --ignore-not-found -oname | grep ^certificatesigningrequest.certificates.k8s.io/$managed_cluster_name | xargs -n 1 oc --kubeconfig $hubkubeconfig adm certificate approve
    oc --kubeconfig $hubkubeconfig patch managedcluster $managed_cluster_name -p='{"spec":{"hubAcceptsClient":true}}' --type=merge
  
    pei "oc --kubeconfig=$hubkubeconfig get managedcluster"
    oc --kubeconfig=$hubkubeconfig label managedcluster $managed_cluster_name "cluster.open-cluster-management.io/clusterset"=default
    
  done
done

# show a prompt so as not to reveal our true nature after
# the demo has concluded

p "deploy the global hub in default namespace"
rm -rf multicluster-global-hub-lite
git clone git@github.com:clyang82/multicluster-global-hub-lite.git
pei "cd multicluster-global-hub-lite && make deploy && cd .."

for i in $(seq 1 "${controlplane_number}"); do

  namespace=multicluster-controlplane-$i
  p "deploy syncer into namespace ${namespace}"
  oc create secret generic multicluster-global-hub-kubeconfig --from-file=kubeconfig=multicluster-global-hub-lite/deploy/server/certs/kube-aggregator.kubeconfig -n ${namespace}
  pei "oc apply -n ${namespace} -k multicluster-global-hub-lite/deploy/syncer"
  oc --kubeconfig multicluster-global-hub-lite/deploy/server/certs/kube-aggregator.kubeconfig create ns ${namespace}

done

cp multicluster-global-hub-lite/deploy/server/certs/kube-aggregator.kubeconfig /tmp/global-hub-kubeconfig
p "Use oc --kubeconfig /tmp/global-hub-kubeconfig to access the global hub"

p ""
