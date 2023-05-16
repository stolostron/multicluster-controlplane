# Copyright Contributors to the Open Cluster Management project
#!/usr/bin/env bash

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"

# speed at which to simulate typing. bigger num = faster
TYPE_SPEED=120

# custom prompt
# see http://www.tldp.org/HOWTO/Bash-Prompt-HOWTO/bash-prompt-escape-sequences.html for escape sequences
DEMO_PROMPT="${GREEN}➜ ${CYAN}\W ${COLOR_RESET}"

# text color
DEMO_CMD_COLOR=$BLACK

source ${REPO_DIR}/hack/demo/demo-magic.sh

function comment() {
  echo -e '\033[0;33m>>> '$1' <<<\033[0m'
}

set -o nounset

clear

management_kubeconfig=${MANAGEMENT_KUBECONFIG:-"clusters/management.kubeconfig"}
hosted_cluster1_kubeconfig=${HOSTED_CLUSTER1_KUBECONFIG:-"clusters/hosted_cluster1.kubeconfig"}
hosted_cluster2_kubeconfig=${HOSTED_CLUSTER2_KUBECONFIG:-"clusters/hosted_cluster2.kubeconfig"}
kind_cluster_kubeconfig=${KIND_CLUSTER_KUBECONFIG:-"clusters/kind_cluster.kubeconfig"}
controlplane_kubeconfig="./controlplane.kubeconfig"

###### start the demo
comment "Demo 1: Run controlplane as a black box in an OpenShift cluster."

p "There is a controlplane with self-management enabled on the multicluster-controlplane namespace in a cluster (management cluster)"
pe "kubectl --kubeconfig ${management_kubeconfig} -n multicluster-controlplane get deploy,routes,secrets"

p "Expose the controlplane kubeconfig from secret multicluster-controlplane-kubeconfig"
pe "kubectl --kubeconfig ${management_kubeconfig} -n multicluster-controlplane get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > ${controlplane_kubeconfig}"

p "The apirescources of this controlplane"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} api-resources"

p "The crds of this controlplane"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} get crds"

p "There is a self management cluster in the controlplane"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} get managedcluster -l multicluster-controlplane.open-cluster-management.io/selfmanagement"

cluster_name=$(kubectl --kubeconfig ${controlplane_kubeconfig} get managedclusters -l multicluster-controlplane.open-cluster-management.io/selfmanagement | awk '{print $1}' | tail -n 1)

p "The cluster has required cluster claims in the controlplane"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} get managedcluster ${cluster_name} -ojsonpath={.status.clusterClaims}"
echo ""

p "Enforce the rescoruces with policy in the management cluster"
pe "cat ./policy/limitrange.yaml"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} apply -f ./policy/limitrange.yaml"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} get policy --all-namespaces -w"
pe "kubectl --kubeconfig ${management_kubeconfig} -n default get limitranges"

p "The resouce usage of controlplane"
pe "kubectl --kubeconfig ${management_kubeconfig} -n multicluster-controlplane top pods --use-protocol-buffers"

comment "###### Demo 2: Run controlplane with agent in hosted mode. ######"

p "Import an openshift cluster 'hosted-cluster1' in hosted mode to the controlplane"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} create namespace hosted-cluster1"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} -n hosted-cluster1 create secret generic managedcluster-kubeconfig --from-file kubeconfig=${hosted_cluster1_kubeconfig}"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} apply -f ./klusterlet/hosted-cluster1.yaml"

p "Import the other openshift cluster 'hosted-cluster2' in hosted mode to the controlplane"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} create namespace hosted-cluster2"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} -n hosted-cluster2 create secret generic managedcluster-kubeconfig --from-file kubeconfig=${hosted_cluster2_kubeconfig}"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} apply -f ./klusterlet/hosted-cluster2.yaml"

p "Two agents are deployed by the controlplane to connect the openshift clusters in hosted mode"
pe "kubectl --kubeconfig ${management_kubeconfig} -n multicluster-controlplane get pods -w"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} get managedclusters -w"
pe "kubectl --kubeconfig ${controlplane_kubeconfig} get policy --all-namespaces -w"

comment "###### Demo 3: Run multiple controlplane instances in a cluster. ######"
controlplane_1_kubeconfig="./controlplane-1.kubeconfig"
p "Deploy the other controlplane in this cluster"
pe "kubectl --kubeconfig ${management_kubeconfig} create namespace multicluster-controlplane-1"
pe "helm template multicluster-controlplane ${REPO_DIR}/charts/multicluster-controlplane -n multicluster-controlplane-1 | kubectl --kubeconfig ${management_kubeconfig} apply -f -"
pe "kubectl --kubeconfig ${management_kubeconfig} -n multicluster-controlplane-1 get pods -w"
pe "kubectl --kubeconfig ${management_kubeconfig} -n multicluster-controlplane-1 get secrets multicluster-controlplane-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > ${controlplane_1_kubeconfig}"

p "Import a KinD cluster in default mode"
pushd ${REPO_DIR}
export KUBECONFIG=${REPO_DIR}/hack/demo/${kind_cluster_kubeconfig}
export CONTROLPLANE_KUBECONFIG=${REPO_DIR}/hack/demo/${controlplane_1_kubeconfig}
export CLUSTER_NAME="kind"
pe "make deploy-agent"
unset KUBECONFIG
unset CONTROLPLANE_KUBECONFIG
unset CLUSTER_NAME
popd

p "The agent in the KinD cluster"
pe "kubectl --kubeconfig ${kind_cluster_kubeconfig} -n multicluster-controlplane-agent get pods -w"

p "The managed cluster in the multicluster-controlplane-1"
pe "kubectl --kubeconfig ${controlplane_1_kubeconfig} get managedclusters -w"

p "Enforce the resouces with policy in the kind cluster"
pe "kubectl --kubeconfig ${controlplane_1_kubeconfig} apply -f ./policy/limitrange.yaml"
pe "kubectl --kubeconfig ${controlplane_1_kubeconfig} get policy --all-namespaces -w"
pe "kubectl --kubeconfig ${kind_cluster_kubeconfig} -n default get limitranges"
