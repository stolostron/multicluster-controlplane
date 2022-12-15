# Copyright Contributors to the Open Cluster Management project
FROM golang:1.19 AS builder

ARG OS=linux
ARG ARCH=amd64
ENV DIRPATH /workspace/multicluster-controlplane
WORKDIR ${DIRPATH}

COPY . .

RUN apt-get update && apt-get install net-tools && make vendor 
RUN GOOS=${OS} \
    GOARCH=${ARCH} \
    make build

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ENV USER_UID=10001

COPY --from=builder /workspace/multicluster-controlplane/bin/multicluster-controlplane /

USER ${USER_UID}
