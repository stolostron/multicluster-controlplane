#!/usr/bin/env bash
set -o nounset
set -o pipefail

WORK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
source "$WORK_DIR/multicluster-controlplane-pre-install-util.sh"
kube::util::ensure-gnu-sed
kube::util::test_openssl_installed
kube::util::ensure-cfssl

network_interface=${NETWORK_INTERFACE:-"eth0"}
# TODO: generate certs within multicluster-controlplane, the api host IP will be used in certificate subject, use loopback IP for now.
# api_host_ip=$(ip addr show dev $network_interface | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*')
# if [ ! $api_host_ip ] ; then
#     echo "api_host_ip should be set"
#     exit 1
# fi
api_host_ip=127.0.0.1
api_secure_port=${API_SECURE_PORT:-"9443"}

cert_dir=${cert_dir:-"${WORK_DIR}/certs"}
service_account_key="${cert_dir}/kube-serviceaccount.key"

function generate_certs {
    CERT_DIR=$1
    API_HOST_IP=$2
    API_HOST_PORT=$3

    # Ensure CERT_DIR is created for auto-generated crt/key and kubeconfig
    mkdir -p "${CERT_DIR}"
    CONTROLPLANE_SUDO=$(test -w "${CERT_DIR}" || echo "sudo -E")

    # in the flags of apiserver --service-cluster-ip-range
    FIRST_SERVICE_CLUSTER_IP=${FIRST_SERVICE_CLUSTER_IP:-10.0.0.1}

    # create ca
    kube::util::create_signing_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" server '"server auth"'
    kube::util::create_signing_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" client '"client auth"'
    kube::util::create_signing_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" request-header '"proxy client auth"'
    
    # serving cert for kube-apiserver
    ROOT_CA_FILE="serving-kube-apiserver.crt"
    kube::util::create_serving_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" "server-ca" kube-apiserver kubernetes.default kubernetes.default.svc "localhost" "${API_HOST_IP}" "${FIRST_SERVICE_CLUSTER_IP}"
    
    # create client certs signed with client-ca, given id, given CN and a number of groups
    kube::util::create_client_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" 'client-ca' admin system:admin system:masters
    kube::util::create_client_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" 'client-ca' kube-apiserver kube-apiserver
    kube::util::create_client_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" 'client-ca' kube-aggregator system:kube-aggregator system:masters
    
    # create matching certificates for kube-aggregator
    kube::util::create_serving_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" "server-ca" kube-aggregator api.kube-public.svc "localhost" "${API_HOST_IP}"
    kube::util::create_client_certkey "${CONTROLPLANE_SUDO}" "${CERT_DIR}" "request-header-ca" auth-proxy system:auth-proxy
    
    # generate kubeconfig
    kube::util::write_client_kubeconfig "${CONTROLPLANE_SUDO}" "${CERT_DIR}" "${ROOT_CA_FILE}" "${API_HOST_IP}" "${API_HOST_PORT}" kube-aggregator
}

function set_service_accounts {
    output_file=$1
    # Generate ServiceAccount key if needed
    if [[ ! -f "${output_file}" ]]; then
      mkdir -p "$(dirname "${output_file}")"
      openssl genrsa -out "${output_file}" 2048 2>/dev/null     # create user private key
    fi
}

set_service_accounts $service_account_key
generate_certs $cert_dir $api_host_ip $api_secure_port
