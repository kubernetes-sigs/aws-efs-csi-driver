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

# Hard-coded platform to `linux/amd64` because
# 1) go mod download is super slow with arm64 on x86 host since it requires QEMU simulation at software level.
# 2) a better approach with `--platform={BUILDPLATFORM}` is only supported by docker buildx not docker build.
FROM --platform=linux/amd64 golang:1.13.4-stretch as builder
WORKDIR /go/src/github.com/kubernetes-sigs/aws-efs-csi-driver

ARG TARGETOS
ARG TARGETARCH
RUN echo "TARGETOS:$TARGETOS, TARGETARCH:$TARGETARCH"
RUN echo "I am running on $(uname -s)/$(uname -m)"

ADD . .

# Default client source is `k8s` which can be overriden with –build-arg when building the Docker image
ARG client_source=k8s
ENV EFS_CLIENT_SOURCE=$client_source

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} make aws-efs-csi-driver

FROM amazonlinux:2.0.20210219.0

# TODO: Remove this once PR is merged and a new amazon-efs-utils release is cut.
RUN yum install git make rpm-build python3-pip -y \
  && git clone https://github.com/chrishenzie/efs-utils \
  && cd efs-utils \
  && git checkout mount-by-ip-with-tls \
  && make rpm \
  && yum localinstall build/rpmbuild/RPMS/noarch/amazon-efs-utils-1.30.1-1.amzn2.noarch.rpm -y \
  && cd / \
  && rm -rf efs-utils \
  && pip3 install botocore==1.20.44

# At image build time, static files installed by efs-utils in the config directory, i.e. CAs file, need
# to be saved in another place so that the other stateful files created at runtime, i.e. private key for
# client certificate, in the same config directory can be persisted to host with a host path volume.
# Otherwise creating a host path volume for that directory will clean up everything inside at the first time.
# Those static files need to be copied back to the config directory when the driver starts up.
RUN mv /etc/amazon/efs /etc/amazon/efs-static-files

COPY --from=builder /go/src/github.com/kubernetes-sigs/aws-efs-csi-driver/bin/aws-efs-csi-driver /bin/aws-efs-csi-driver
COPY THIRD-PARTY /

ENTRYPOINT ["/bin/aws-efs-csi-driver"]
