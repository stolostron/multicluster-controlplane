# Copyright Contributors to the Open Cluster Management project

BINARYDIR := bin

HUB_NAME ?= multicluster-controlplane
IMAGE_REGISTRY ?= quay.io/stolostron
IMAGE_TAG ?= latest
IMAGE_NAME ?= $(IMAGE_REGISTRY)/multicluster-controlplane:$(IMAGE_TAG)

# verify code
golint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.45.2
	go vet ./...
	golangci-lint run --timeout=3m ./...
.PHONY: golint

verify: golint
.PHONY: verify

# build code
vendor:
	go mod tidy 
	go mod vendor
.PHONY: vendor

build: vendor
	mkdir -p $(BINARYDIR)
	go build -ldflags="-s -w"  -o bin/multicluster-controlplane cmd/manager/manager.go
	go build -ldflags="-s -w" -o bin/multicluster-agent cmd/agent/agent.go
.PHONY: build

build-image:
	docker build -f Dockerfile -t $(IMAGE_NAME) .
.PHONY: build-image

# run controlplane
clean:
	rm -rf bin .embedded-etcd vendor
.PHONY: clean

run: clean build
	hack/start-multicluster-controlplane.sh
.PHONY: run

# deploy controlplane
deploy-etcd:
	hack/deploy-etcd.sh
.PHONY: deploy-etcd

deploy:
	HUB_NAME=$(HUB_NAME) hack/deploy-multicluster-controlplane.sh
.PHONY: deploy

deploy-agent:
	hack/deploy-agent.sh
.PHONY: deploy-agent

destroy:
	HUB_NAME=$(HUB_NAME) hack/deploy-multicluster-controlplane.sh uninstall
.PHONY: destroy

# test code
export CONTROLPLANE_NUMBER ?= 2
GO_TEST ?= go test -v

test-unit:
	${GO_TEST} `go list ./... | grep -v test`
.PHONY: test-unit

prow-e2e:
	./test/scripts/prow-e2e.sh
.PHONY: prow-e2e

setup-dep:
	./test/scripts/setup-dep.sh
.PHONY: setup-dep

setup-e2e: setup-dep
	./test/scripts/setup-e2e.sh
.PHONY: setup-e2e

test-e2e: vendor
	./test/scripts/test-e2e.sh
.PHONY: test-e2e

cleanup-e2e:
	./test/scripts/cleanup-e2e.sh
.PHONY: cleanup-e2e
