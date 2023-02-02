#!/usr/bin/env bash
set -e -o pipefail

# environment variables that must be passed down to this script
ENV_VARS="MULTICLUSTER_CONTROLPLANE_VERSION RPM_RELEASE"
for env in $ENV_VARS ; do
  if [[ -z "${!env}" ]] ; then
    echo "Error: Mandatory environment variable '${env}' is missing"
    echo ""
    echo "Run 'make rpm' or 'make srpm' instead of this script"
    exit 1
  fi
done

MULTICLUSTER_CONTROLPLANE_VERSION=$(echo ${MULTICLUSTER_CONTROLPLANE_VERSION} | sed s/-/_/g)
GIT_SHA=$(git rev-parse HEAD)
GIT_SHORT_SHA="${GIT_SHA:0:7}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
RPMBUILD_DIR="$(git rev-parse --show-toplevel)/_output/rpmbuild/"
TARBALL_FILE="multicluster-controlplane-${GIT_SHORT_SHA}.tar.gz"

create_local_tarball() {
  echo "Creating local tarball"
  tar -czf "${RPMBUILD_DIR}/SOURCES/${TARBALL_FILE}" \
            --exclude='.git' \
            --exclude='_output' \
            --transform="s|^|multicluster-controlplane-${GIT_SHA}/|" \
            --exclude="${TARBALL_FILE}" "${SCRIPT_DIR}/../../"
}

download_commit_tarball() {
  echo "Downloading tarball with commit $1"
  TARGET_GIT_SHA=${1:-$GIT_SHA}
  spectool -g --define "_topdir ${RPMBUILD_DIR}" \
          --define="release ${RPM_RELEASE}" \
          --define="version ${MULTICLUSTER_CONTROLPLANE_VERSION}" \
          --define "commit ${TARGET_GIT_SHA}" \
          -R "${SCRIPT_DIR}/multicluster-controlplane.spec"
}

build_commit_rpm() {
  TARGET_GIT_SHA=${1:-$GIT_SHA}
  # using --defines works for rpm building, but not for an srpm
  cat >"${RPMBUILD_DIR}"SPECS/multicluster-controlplane.spec <<EOF
%global release ${RPM_RELEASE}
%global version ${MULTICLUSTER_CONTROLPLANE_VERSION}
%global commit ${TARGET_GIT_SHA}
EOF
  cat "${SCRIPT_DIR}/multicluster-controlplane.spec" >> "${RPMBUILD_DIR}SPECS/multicluster-controlplane.spec"

  echo "Building RPM packages"
  rpmbuild --quiet "${RPMBUILD_OPT}" --define "_topdir ${RPMBUILD_DIR}" "${RPMBUILD_DIR}"SPECS/multicluster-controlplane.spec
}

usage() {
  echo "Usage: $(basename $0) <all | rpm | srpm> < local | commit <commit-id> >"
  exit 1
}

[ $# -lt 2 ] && usage

case $1 in
  all)  RPMBUILD_OPT=-ba ;;
  rpm)  RPMBUILD_OPT=-bb ;;
  srpm) RPMBUILD_OPT=-bs ;;
  *)    usage
esac
shift

# prepare the rpmbuild env
mkdir -p "${RPMBUILD_DIR}"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

case $1 in
    local)
      create_local_tarball
      build_commit_rpm "${GIT_SHA}"
      ;;
    commit)
      download_commit_tarball "$2"
      build_commit_rpm "$2"
      ;;
    *)
      usage
esac
