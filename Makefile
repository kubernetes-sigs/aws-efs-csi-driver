# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

VERSION=v2.1.12

PKG=github.com/kubernetes-sigs/aws-efs-csi-driver
GIT_COMMIT?=$(shell git rev-parse HEAD)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

EFS_CLIENT_SOURCE?=k8s
LDFLAGS?="-X ${PKG}/pkg/driver.driverVersion=${VERSION} \
		  -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} \
		  -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE} \
		  -X ${PKG}/pkg/driver.efsClientSource=${EFS_CLIENT_SOURCE}"

GO111MODULE=on
GOPROXY=direct
GOPATH=$(shell go env GOPATH)
GOOS=$(shell go env GOOS)

REGISTRY?=amazon
IMAGE?=$(REGISTRY)/aws-efs-csi-driver
TAG?=$(GIT_COMMIT)

OUTPUT_TYPE?=docker

OS?=linux
ARCH?=amd64
OSVERSION?=amazon

ALL_OS?=linux
ALL_ARCH_linux?=amd64 arm64
ALL_OSVERSION_linux?=amazon
ALL_OS_ARCH_OSVERSION_linux=$(foreach arch, $(ALL_ARCH_linux), $(foreach osversion, ${ALL_OSVERSION_linux}, linux-$(arch)-${osversion}))

ALL_OS_ARCH_OSVERSION=$(foreach os, $(ALL_OS), ${ALL_OS_ARCH_OSVERSION_${os}})

PLATFORM?=linux/amd64,linux/arm64

# split words on hyphen, access by 1-index
word-hyphen = $(word $2,$(subst -, ,$1))

.EXPORT_ALL_VARIABLES:

.PHONY: linux/$(ARCH) bin/aws-efs-csi-driver
linux/$(ARCH): bin/aws-efs-csi-driver
bin/aws-efs-csi-driver: | bin
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -mod=vendor -ldflags ${LDFLAGS} -o bin/aws-efs-csi-driver ./cmd/

.PHONY: all
all: all-image-docker

.PHONY: all-push
all-push:
	docker buildx build \
		--no-cache-filter=linux-amazon \
		--platform=$(PLATFORM) \
		--progress=plain \
		--target=$(OS)-$(OSVERSION) \
		--output=type=registry \
		-t=$(IMAGE):$(TAG) \
		.
	touch $@

.PHONY: all-image-docker
all-image-docker: $(addprefix sub-image-docker-,$(ALL_OS_ARCH_OSVERSION_linux))

sub-image-%:
	$(MAKE) OUTPUT_TYPE=$(call word-hyphen,$*,1) OS=$(call word-hyphen,$*,2) ARCH=$(call word-hyphen,$*,3) OSVERSION=$(call word-hyphen,$*,4) image

.PHONY: image
image: .image-$(TAG)-$(OS)-$(ARCH)-$(OSVERSION)
.image-$(TAG)-$(OS)-$(ARCH)-$(OSVERSION):
	docker buildx build \
		--no-cache-filter=linux-amazon \
		--platform=$(OS)/$(ARCH) \
		--progress=plain \
		--target=$(OS)-$(OSVERSION) \
		--output=type=$(OUTPUT_TYPE) \
		-t=$(IMAGE):$(TAG)-$(OS)-$(ARCH)-$(OSVERSION) \
		.
	touch $@


.PHONY: clean
clean:
	rm -rf .*image-* bin/

bin /tmp/helm:
	@mkdir -p $@

bin/helm: | /tmp/helm bin
	@curl -o /tmp/helm/helm.tar.gz -sSL https://get.helm.sh/helm-v3.5.3-${GOOS}-amd64.tar.gz
	@tar -zxf /tmp/helm/helm.tar.gz -C bin --strip-components=1
	@rm -rf /tmp/helm/*

.PHONY: verify
verify:
	./hack/verify-all

.PHONY: test
test:
	go test -v -race ./pkg/...

.PHONY: test-e2e
test-e2e:
	DRIVER_NAME=aws-efs-csi-driver \
	CONTAINER_NAME=efs-plugin \
	TEST_EXTRA_FLAGS='--cluster-name=$$CLUSTER_NAME' \
	AWS_REGION=us-west-2 \
	AWS_AVAILABILITY_ZONES=us-west-2a,us-west-2b,us-west-2c \
	TEST_PATH=./test/e2e/... \
	GINKGO_FOCUS="\[efs-csi\]" \
	GINKGO_SKIP="\[Disruptive\]|\[Serial\]" \
	./hack/e2e/run.sh

.PHONY: test-e2e-external-eks
test-e2e-external-eks:
	CLUSTER_TYPE=eksctl \
	K8S_VERSION="1.33" \
	DRIVER_NAME=aws-efs-csi-driver \
	HELM_VALUES_FILE="./hack/values_eksctl.yaml" \
	CONTAINER_NAME=efs-plugin \
	TEST_EXTRA_FLAGS='--cluster-name=$$CLUSTER_NAME' \
	AWS_REGION=us-west-2 \
	AWS_AVAILABILITY_ZONES=us-west-2a,us-west-2b,us-west-2c \
	TEST_PATH=./test/e2e/... \
	GINKGO_FOCUS="\[efs-csi\]" \
	GINKGO_SKIP="\[Disruptive\]|\[Serial\]" \
	EKSCTL_ADMIN_ROLE="Infra-prod-KopsDeleteAllLambdaServiceRoleF1578477-1ELDFIB4KCMXV" \
	./hack/e2e/run.sh

.PHONY: test-e2e-bin
test-e2e-bin:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go test -mod=vendor -ldflags ${LDFLAGS} -c -o bin/test-e2e ./test/e2e/

.PHONY: generate-kustomize
generate-kustomize: bin/helm
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/csidriver.yaml > ../../deploy/kubernetes/base/csidriver.yaml
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/node-daemonset.yaml -f values.yaml > ../../deploy/kubernetes/base/node-daemonset.yaml
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/node-serviceaccount.yaml -f values.yaml > ../../deploy/kubernetes/base/node-serviceaccount.yaml
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/controller-deployment.yaml -f values.yaml > ../../deploy/kubernetes/base/controller-deployment.yaml
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/controller-serviceaccount.yaml -f values.yaml > ../../deploy/kubernetes/base/controller-serviceaccount.yaml
