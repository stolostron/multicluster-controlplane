#!/usr/bin/env bash

set -euxo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/../.." ; pwd -P)"

IP="$(cat "${SHARED_DIR}/public_ip")"
HOST="ec2-user@${IP}"
KEY="${SHARED_DIR}/private.pem"
chmod 400 "${KEY}"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "${KEY}")
HOST_DIR="/tmp/multicluster-controlplane"

echo "export IMAGE_NAME=$IMAGE_NAME" >> ${PROJECT_ROOT}/test/env.list
echo "export OPENSHIFT_CI=$OPENSHIFT_CI" >> ${PROJECT_ROOT}/test/env.list
echo "export VERBOSE=6" >> ${PROJECT_ROOT}/test/env.list

cd $PROJECT_ROOT && cd ..
scp "${OPT[@]}" -r ./multicluster-controlplane "$HOST:$HOST_DIR"

echo "install tools"
ssh "${OPT[@]}" "$HOST" sudo yum install gcc git wget jq -y 

echo "setup e2e environment"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/env.list && sudo make setup-dep && make setup-e2e" > >(tee "$ARTIFACT_DIR/setup-e2e.log") 2>&1

echo "runn e2e"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/env.list && make test-e2e" > >(tee "$ARTIFACT_DIR/test-e2e.log") 2>&1
