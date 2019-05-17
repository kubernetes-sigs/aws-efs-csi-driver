# Copyright 2018 The Kubernetes Authors.
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
IMAGE=amazon/aws-efs-csi-driver
VERSION=0.1.0

FLAG_COMMIT=-X github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/info.commitSha=`git log --pretty=format:'%H' -n 1`
FLAG_DRIVER=-X github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver.vendorVersion=${VERSION} -X github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/info.driver=${VERSION}
FLAG_ARCH=-X github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/info.arch=`uname -m`
FLAG_OS=-X github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/info.os=`uname -r`
FLAG_DATE=-X 'github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/info.buildDate=`date -u '+%Y-%m-%d %H:%M:%S'`'
FLAG_GO_VERSION=-X 'github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/info.goVersion=`go version`'

LDFLAGS=-ldflags "${FLAG_DRIVER} ${FLAG_COMMIT} ${FLAG_ARCH} ${FLAG_OS} ${FLAG_DATE} ${FLAG_GO_VERSION}"

.PHONY: aws-efs-csi-driver
aws-efs-csi-driver:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build ${LDFLAGS} -o bin/aws-efs-csi-driver ./cmd/

.PHONY: verify
verify:
	./hack/verify-all

.PHONY: test
test:
	go test -v -race ./pkg/...

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
