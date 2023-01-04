#!/bin/bash

KUBECTL=${KUBECTL:-"kubectl"}
KUSTOMIZE=${KUSTOMIZE:-"kustomize"}
ETCD_IMAGE_NAME=${ETCD_IMAGE_NAME:-"quay.io/coreos/etcd"}

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." ; pwd -P)"
etcdns=${ETCD_NS:-"multicluster-controlplane-etcd"}
etcd_cert=${ETCD_CERT:-"$project_dir/test/resources/etcd/cert"}
mkdir -p $etcd_cert || true

cd $etcd_cert
echo '{"CN":"multicluster-controlplane","key":{"algo":"rsa","size":2048}}' | cfssl gencert -initca - | cfssljson -bare ca -
echo '{"signing":{"default":{"expiry":"43800h","usages":["signing","key encipherment","server auth","client auth"]}}}' > ca-config.json
export ADDRESS=
export NAME=client
echo '{"CN":"'$NAME'","hosts":[""],"key":{"algo":"rsa","size":2048}}' | cfssl gencert -config=ca-config.json -ca=ca.pem -ca-key=ca-key.pem -hostname="$ADDRESS" - | cfssljson -bare $NAME

mkdir -p ${project_dir}/hack/deploy/etcd/cert-etcd
cp -f ${etcd_cert}/ca.pem ${project_dir}/hack/deploy/etcd/cert-etcd/ca.pem

# copy cert to controlplane dir
CONTROLPLANE_ETCD_CERT=${project_dir}/hack/deploy/controlplane/cert-etcd
mkdir -p ${CONTROLPLANE_ETCD_CERT}
cp -f ${etcd_cert}/ca.pem ${CONTROLPLANE_ETCD_CERT}/ca.pem
cp -f ${etcd_cert}/client.pem ${CONTROLPLANE_ETCD_CERT}/client.pem
cp -f ${etcd_cert}/client-key.pem ${CONTROLPLANE_ETCD_CERT}/client-key.pem

cd $project_dir
cp hack/deploy/etcd/kustomization.yaml  hack/deploy/etcd/kustomization.yaml.tmp
cd hack/deploy/etcd && ${KUSTOMIZE} edit set namespace ${etcdns} && ${KUSTOMIZE} edit set image quay.io/coreos/etcd=${ETCD_IMAGE_NAME}
cd ../../../
${KUSTOMIZE} build ${project_dir}/hack/deploy/etcd | ${KUBECTL} apply -f -
mv hack/deploy/etcd/kustomization.yaml.tmp hack/deploy/etcd/kustomization.yaml

echo "#### etcd deployed ####" 
echo ""
