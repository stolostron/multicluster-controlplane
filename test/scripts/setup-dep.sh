#!/usr/bin/env bash

bin_dir="/usr/bin"

function check_golang() {
  export PATH=$PATH:/usr/local/go/bin
  if ! command -v go >/dev/null 2>&1; then
    wget https://dl.google.com/go/go1.19.3.linux-amd64.tar.gz >/dev/null 2>&1
    sudo tar -C /usr/local/ -xvf go1.19.3.linux-amd64.tar.gz >/dev/null 2>&1
    rm go1.19.3.linux-amd64.tar.gz
  fi
  if [[ $(go version) < "go version go1.19" ]]; then
    echo "go version is less than 1.19, update to 1.19"
    sudo rm -rf /usr/local/go
    wget https://dl.google.com/go/go1.19.3.linux-amd64.tar.gz >/dev/null 2>&1
    sudo tar -C /usr/local/ -xvf go1.19.3.linux-amd64.tar.gz >/dev/null 2>&1
    rm go1.19.3.linux-amd64.tar.gz
    sleep 2
  fi
  echo "go version: $(go version)"
}

function check_kind() {
  if ! command -v kind >/dev/null 2>&1; then 
    curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.14.0/kind-linux-amd64 
    chmod +x ./kind
    sudo mv ./kind ${bin_dir}/kind
  fi
  if [[ $(kind version |awk '{print $2}') < "v0.12.0" ]]; then
    sudo surm -rf $(which kind)
    curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.14.0/kind-linux-amd64 
    chmod +x ./kind
    sudo mv ./kind ${bin_dir}/kind
  fi
  echo "kind version: $(kind version)"
}

function check_kubectl() {
  if ! command -v kubectl >/dev/null 2>&1; then 
    echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
        curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/linux/amd64/kubectl
    elif [[ "$(uname)" == "Darwin" ]]; then
        curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/darwin/amd64/kubectl
    fi
    chmod +x ./kubectl
    sudo mv ./kubectl ${bin_dir}/kubectl
  fi
  echo "kubectl version: $(kubectl version --client --short)"
}

function check_kustomize() {
  if ! command -v kustomize >/dev/null 2>&1; then 
    echo "This script will install kustomize on your machine"
    curl -LO "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
    chmod +x ./install_kustomize.sh
    sudo ./install_kustomize.sh 4.5.7 ${bin_dir}
  fi
  echo "kustomize version: $(kustomize version)"
}

function check_clusteradm() {
  if ! command -v clusteradm >/dev/null 2>&1; then 
    curl -LO https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh
    chmod +x ./install.sh
    sudo ./install.sh 0.4.1
    sudo mv /usr/local/bin/clusteradm ${bin_dir}/clusteradm
    rm ./install.sh
  fi
  echo "clusteradm path: $(which clusteradm)"
}

function check_ginkgo() {
  if ! command -v ginkgo >/dev/null 2>&1; then 
    go install github.com/onsi/ginkgo/v2/ginkgo@v2.5.0
    sudo mv $(go env GOPATH)/bin/ginkgo ${bin_dir}/ginkgo
  fi 
  echo "ginkgo version: $(ginkgo version)"
}

function check_cfssl() {
  if ! command -v cfssl >/dev/null 2>&1; then 
    curl --retry 10 -L -o cfssl https://github.com/cloudflare/cfssl/releases/download/v1.5.0/cfssl_1.5.0_linux_amd64
    chmod +x cfssl || true
    sudo mv cfssl ${bin_dir}/cfssl
  fi 
  echo "cfssl path: $(which cfssl)"
  if ! command -v cfssljson >/dev/null 2>&1; then 
    curl --retry 10 -L -o cfssljson https://github.com/cloudflare/cfssl/releases/download/v1.5.0/cfssljson_1.5.0_linux_amd64
    chmod +x cfssljson || true
    sudo mv cfssljson ${bin_dir}/cfssljson
  fi 
  echo "cfssljson path: $(which cfssljson)"
}

check_golang
check_kind
check_kubectl
check_kustomize
check_clusteradm
check_ginkgo
check_cfssl