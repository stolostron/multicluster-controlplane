# Copyright Contributors to the Open Cluster Management project

BINARYDIR := bin

KUBECTL ?= kubectl
KUSTOMIZE ?= kustomize

HUB_NAME ?= multicluster-controlplane
IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
IMAGE_NAME ?= $(IMAGE_REGISTRY)/multicluster-controlplane:$(IMAGE_TAG)
ETCD_NS ?= multicluster-controlplane-etcd

check-copyright: 
	@hack/check/check-copyright.sh

check: check-copyright 

verify-gocilint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.45.2
	go vet ./...
	golangci-lint run --timeout=3m ./...

verify: verify-gocilint

build: 
	$(shell if [ ! -e $(BINARYDIR) ];then mkdir -p $(BINARYDIR); fi)
	go build -o bin/multicluster-controlplane cmd/server/main.go 
.PHONY: build

build-image:
	docker build -f Dockerfile -t $(IMAGE_NAME) .
.PHONY: build-image

push-image:
	docker push $(IMAGE_NAME)
.PHONY: push-image

clean:
	rm -rf bin .embedded-etcd vendor
.PHONY: clean

vendor: 
	go mod tidy 
	go mod vendor
.PHONY: vendor

run:
	bash -x hack/start-multicluster-controlplane.sh
.PHONY: run

all: clean vendor build run
.PHONY: all

update-crd:
	bash -x hack/crd-update/copy-crds.sh
.PHONY: update-crd

deploy-etcd: 
	$(KUBECTL) get ns $(ETCD_NS); if [ $$? -ne 0 ] ; then $(KUBECTL) create ns $(ETCD_NS); fi
	hack/deploy-etcd.sh

deploy-with-external-etcd:
	$(KUBECTL) get ns $(HUB_NAME); if [ $$? -ne 0 ] ; then $(KUBECTL) create ns $(HUB_NAME); fi
	hack/deploy-multicluster-controlplane.sh false

deploy:
	$(KUBECTL) get ns $(HUB_NAME); if [ $$? -ne 0 ] ; then $(KUBECTL) create ns $(HUB_NAME); fi
	hack/deploy-multicluster-controlplane.sh

# test
export CONTROLPLANE_NUMBER ?= 2
export VERBOSE ?= 5
GO_TEST ?= go test -v

test-unit:
	${GO_TEST} `go list ./... | grep -v test`
.PHONY: test-unit

setup-dep:
	./test/scripts/setup-dep.sh
.PHONY: setup-dep

prow-e2e: 
	./test/scripts/prow-e2e.sh
.PHONY: prow-e2e

setup-e2e: setup-dep
	./test/scripts/setup-e2e.sh
.PHONY: setup-e2e

cleanup-e2e:
	./test/scripts/cleanup-e2e.sh
.PHONY: cleanup-e2e

test-e2e: vendor
	./test/scripts/test-e2e.sh -v $(VERBOSE)
.PHONY: test-e2e

setup-integration: setup-dep vendor build
	./test/scripts/setup-integration.sh
.PHONY: setup-integration

cleanup-integration:
	./test/scripts/cleanup-integration.sh
.PHONY: cleanup-integration

test-integration:
	./test/scripts/test-integration.sh -v $(VERBOSE)
.PHONY: test-integration