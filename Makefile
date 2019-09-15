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
IMAGE=amazon/aws-efs-csi-driver
VERSION=0.3.0-dirty
GIT_COMMIT?=$(shell git rev-parse HEAD)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS?="-X ${PKG}/pkg/driver.driverVersion=${VERSION} -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE}"
GO111MODULE=on
GOPROXY=direct

.EXPORT_ALL_VARIABLES:

.PHONY: aws-efs-csi-driver
aws-efs-csi-driver:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -ldflags ${LDFLAGS} -o bin/aws-efs-csi-driver ./cmd/

.PHONY: verify
verify:
	./hack/verify-all

.PHONY: test
test:
	go test -v -race $$(go list ./pkg/... | grep -v /driver)
	# TODO stop skipping controller tests when controller is implemented
	go test -v -race ./pkg/driver/... -ginkgo.skip='\[Controller.Server\]'

.PHONY: test-e2e
test-e2e:
	AWS_REGION=us-west-2 AWS_AVAILABILITY_ZONES=us-west-2a,us-west-2b,us-west-2c ./hack/run-e2e-test

.PHONY: image
image:
	docker build -t $(IMAGE):latest .

.PHONY: push
push:
	docker push $(IMAGE):latest

.PHONY: image-release
image-release:
	docker build -t $(IMAGE):$(VERSION) .

.PHONY: push-release
push-release:
	docker push $(IMAGE):$(VERSION)
