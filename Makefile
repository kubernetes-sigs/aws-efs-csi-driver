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

PKG=github.com/kubernetes-sigs/aws-efs-csi-driver
IMAGE?=amazon/aws-efs-csi-driver
VERSION=v1.3.4-dirty
GIT_COMMIT?=$(shell git rev-parse HEAD)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
EFS_CLIENT_SOURCE?=k8s
IMAGE_PLATFORMS?=linux/arm64,linux/amd64
LDFLAGS?="-X ${PKG}/pkg/driver.driverVersion=${VERSION} \
		  -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} \
		  -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE} \
		  -X ${PKG}/pkg/driver.efsClientSource=${EFS_CLIENT_SOURCE}"
GO111MODULE=on
GOPROXY=direct
GOPATH=$(shell go env GOPATH)
GOOS=$(shell go env GOOS)

.EXPORT_ALL_VARIABLES:

.PHONY: aws-efs-csi-driver
aws-efs-csi-driver:
	mkdir -p bin
	@echo GOARCH:${GOARCH}
	CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags ${LDFLAGS} -o bin/aws-efs-csi-driver ./cmd/

bin /tmp/helm:
	@mkdir -p $@

bin/helm: | /tmp/helm bin
	@curl -o /tmp/helm/helm.tar.gz -sSL https://get.helm.sh/helm-v3.5.3-${GOOS}-amd64.tar.gz
	@tar -zxf /tmp/helm/helm.tar.gz -C bin --strip-components=1
	@rm -rf /tmp/helm/*

build-darwin:
	mkdir -p bin/darwin/
	CGO_ENABLED=0 GOOS=darwin go build -mod=vendor -ldflags ${LDFLAGS} -o bin/darwin/aws-efs-csi-driver ./cmd/

run-darwin: build-darwin
	bin/darwin/aws-efs-csi-driver --version

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
	./hack/e2e/run.sh

.PHONY: test-e2e-bin
test-e2e-bin:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go test -mod=vendor -ldflags ${LDFLAGS} -c -o bin/test-e2e ./test/e2e/

.PHONY: image
image:
	docker build -t $(IMAGE):master .

.PHONY: image-multi-arch--push
image-multi-arch-push:
	docker buildx build \
			  -t $(IMAGE):master \
			  --platform=$(IMAGE_PLATFORMS) \
			  --progress plain \
			  --push .

.PHONY: push
push: image
	docker push $(IMAGE):master

.PHONY: image-release
image-release:
	docker build -t $(IMAGE):$(VERSION) .

.PHONY: push-release
push-release:
	docker push $(IMAGE):$(VERSION)

.PHONY: generate-kustomize
generate-kustomize: bin/helm
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/csidriver.yaml > ../../deploy/kubernetes/base/csidriver.yaml
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/node-daemonset.yaml -f values.yaml > ../../deploy/kubernetes/base/node-daemonset.yaml
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/node-serviceaccount.yaml -f values.yaml > ../../deploy/kubernetes/base/node-serviceaccount.yaml
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/controller-deployment.yaml -f values.yaml > ../../deploy/kubernetes/base/controller-deployment.yaml
	cd charts/aws-efs-csi-driver && ../../bin/helm template kustomize . -s templates/controller-serviceaccount.yaml -f values.yaml > ../../deploy/kubernetes/base/controller-serviceaccount.yaml
