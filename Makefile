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
IMAGE=chengpan/aws-efs-csi-driver
VERSION=0.1.0

.PHONY: aws-efs-csi-driver
aws-efs-csi-driver:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -ldflags "-X github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver.vendorVersion=${VERSION}" -o bin/aws-efs-csi-driver ./cmd/

.PHONY: test
test:
	go test -v -race ./pkg/...

.PHONY: image
image:
	docker build -t $(IMAGE):testing .

.PHONY: push
push:
	docker push $(IMAGE):testing
