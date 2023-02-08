#!/usr/bin/env bash
set -o nounset
set -o pipefail

KUBECTL=${KUBECTL:-kubectl}

project_dir="$(pwd)"
source "$project_dir/hack/lib/util.sh"
source "$project_dir/hack/lib/logging.sh"

kube::util::ensure-gnu-sed
kube::util::test_openssl_installed
kube::util::ensure-cfssl

controlplane_bin=${CONTROPLANE_BIN:-"${project_dir}/bin"}
network_interface=${NETWORK_INTERFACE:-"eth0"}
api_host_ip=$(ip addr show dev $network_interface | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*')
if [ ! $api_host_ip ] ; then
    echo "api_host_ip should be set"
    exit 1
fi
api_secure_port=${API_SECURE_PORT:-"9443"}

cert_dir=${cert_dir:-"${project_dir}/hack/certs"}
service_account_key="${cert_dir}/kube-serviceaccount.key"

CONTROLPLANE_SUDO=$(test -w "${cert_dir}" || echo "sudo -E")
function start_apiserver {
    apiserver_log=${project_dir}/hack/certs/kube-apiserver.log

    ${CONTROLPLANE_SUDO} "${controlplane_bin}/multicluster-controlplane" \
    --authorization-mode="RBAC"  \
    --v="7" \
    --enable-bootstrap-token-auth \
    --enable-priority-and-fairness="false" \
    --api-audiences="" \
    --external-hostname="${api_host_ip}" \
    --client-ca-file="${cert_dir}/client-ca.crt" \
    --client-key-file="${cert_dir}/client-ca.key" \
    --service-account-key-file="${service_account_key}" \
    --service-account-lookup="true" \
    --service-account-issuer="https://kubernetes.default.svc" \
    --service-account-signing-key-file="${service_account_key}" \
    --enable-admission-plugins="NamespaceLifecycle,ServiceAccount,MutatingAdmissionWebhook,ValidatingAdmissionWebhook,ResourceQuota,ManagedClusterMutating,ManagedClusterValidating,ManagedClusterSetBindingValidating" \
    --disable-admission-plugins="" \
    --bind-address="0.0.0.0" \
    --secure-port="${api_secure_port}" \
    --tls-cert-file="${cert_dir}/serving-kube-apiserver.crt" \
    --tls-private-key-file="${cert_dir}/serving-kube-apiserver.key" \
    --storage-backend="etcd3" \
    --feature-gates="DefaultClusterSet=true,OpenAPIV3=false,AddonManagement=true" \
    --enable-embedded-etcd="true" \
    --etcd-servers="http://localhost:2379" \
    --service-cluster-ip-range="10.0.0.0/24" >"$apiserver_log" 2>&1 &
    apiserver_pid=$!
    echo "$apiserver_pid" > ${project_dir}/test/resources/integration/controlpane_pid

    # echo "Waiting for apiserver to come up"
    kube::util::wait_for_url "https://${api_host_ip}:${api_secure_port}/healthz" "apiserver: " 1 120 1 \
    || { echo "check apiserver logs: $apiserver_log" ; exit 1 ; }
    
    cp ${cert_dir}/kube-aggregator.kubeconfig ${cert_dir}/kubeconfig
    echo "use 'kubectl --kubeconfig=${cert_dir}/kubeconfig' to use the aggregated API server" 
}

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

cleanup()
{
    echo "Cleaning up..."
    # Check if the API server is still running
    [[ -n "${apiserver_pid-}" ]] && kube::util::read-array apiserver_pid < <(pgrep -P "${apiserver_pid}" ; ps -o pid= -p "${apiserver_pid}")
    [[ -n "${apiserver_pids-}" ]] && kill -9 "${apiserver_pids[@]}" 2>/dev/null
    exit 0
}

function healthcheck {
    if [[ -n "${apiserver_pid-}" ]] && ! kill -0 "${apiserver_pid}" 2>/dev/null; then
        warning_log "API server terminated unexpectedly, see ${apiserver_log}"
        apiserver_pid=
    fi
}

function warning_log {
    print_color "$1" "W$(date "+%m%d %H:%M:%S")]" 1
}

trap cleanup EXIT

set_service_accounts $service_account_key
generate_certs $cert_dir $api_host_ip $api_secure_port
start_apiserver

while true; do sleep 10; healthcheck; done