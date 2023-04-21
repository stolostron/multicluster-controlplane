#!/usr/bin/env bash
REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"
source ${REPO_DIR}/test/scripts/init.sh

set -o nounset
set -o pipefail
set -o errexit

kind delete cluster --name $management_cluster

for i in $(seq 1 "${CONTROLPLANE_NUMBER}"); do
  kind delete cluster --name controlplane$i-mc
done

rm -rf $REPO_DIR/multicluster_ca
rm -rf $REPO_DIR/_output
