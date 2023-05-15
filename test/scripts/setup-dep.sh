#!/usr/bin/env bash
REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"

source ${REPO_DIR}/hack/lib/deps.sh

check_golang
check_ginkgo
check_kind
check_kubectl
check_kustomize
check_helm
check_cfssl
