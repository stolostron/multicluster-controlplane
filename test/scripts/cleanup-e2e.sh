#!/bin/bash

set -o nounset
set -o pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." ; pwd -P)"

host=${HOST_CLUSTER_NAME:-"controlplane-hosting"}
kind delete cluster --name $host

number=${CONTROLPLANE_NUMBER:-2}
echo "Controlplane number : $number"
for i in $(seq 1 "${number}"); do
  namespace=controlplane$i
  kind delete cluster --name ${namespace}-mc1
done

rm -rf $project_dir/multicluster_ca
rm -rf $project_dir/_output
