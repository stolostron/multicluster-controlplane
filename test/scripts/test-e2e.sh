#!/bin/bash

set -e

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." ; pwd -P)"
kubeconfig_dir=$project_dir/test/resources/kubeconfig
options_file="${project_dir}/test/resources/kubeconfig/options.yaml"

hosting=${HOST_CLUSTER_NAME:-"hosting"}  # hosting cluster
cp1=controlplane1
cp2=controlplane2
cp1_mc1=$cp1-mc1
cp2_mc1=$cp2-mc1

printf "options:" > $options_file
printf "\n  hosting:" >> $options_file
printf "\n    name: $hosting" >> $options_file
printf "\n    context: kind-$hosting" >> $options_file
printf "\n    kubeconfig: ${kubeconfig_dir}/${hosting}" >> $options_file
printf "\n  controlplanes:" >> $options_file
printf "\n    - name: $cp1" >> $options_file
printf "\n      context: multicluster-controlplane" >> $options_file
printf "\n      kubeconfig: ${kubeconfig_dir}/${cp1}" >> $options_file
printf "\n      managedclusters:" >> $options_file
printf "\n        - name: $cp1_mc1" >> $options_file
printf "\n          context: kind-$cp1_mc1" >> $options_file
printf "\n          kubeconfig: ${kubeconfig_dir}/${cp1_mc1}" >> $options_file
printf "\n    - name: $cp2" >> $options_file
printf "\n      context: multicluster-controlplane" >> $options_file
printf "\n      kubeconfig: ${kubeconfig_dir}/${cp2}" >> $options_file
printf "\n      managedclusters:" >> $options_file
printf "\n        - name: $cp2_mc1" >> $options_file
printf "\n          context: kind-$cp2_mc1" >> $options_file
printf "\n          kubeconfig: ${kubeconfig_dir}/${cp2_mc1}" >> $options_file

while getopts ":f:v:" opt; do
  case $opt in
    f) filter="$OPTARG"
    ;;
    v) verbose="$OPTARG"
    ;;
    \?) echo "Invalid option -$OPTARG" >&2
    exit 1
    ;;
  esac

  case $OPTARG in
    -*) echo "Option $opt needs a valid argument"
    exit 1
    ;;
  esac
done

echo "Build E2E Test ..."
go test -c ${project_dir}/test/e2e -mod=vendor -o ${project_dir}/test/e2e/e2e.test

echo "Run E2E Test ..."
cd ${project_dir}/test/e2e

if [ -z "${filter}" ]; then
  ./e2e.test --options=$options_file -v=$verbose
else
  ./e2e.test --ginkgo.label-filter=${filter} --ginkgo.v --options=$options_file -v=$verbose 
fi

rm ${project_dir}/test/e2e/e2e.test
