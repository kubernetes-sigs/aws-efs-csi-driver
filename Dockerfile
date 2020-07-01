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

FROM golang:1.13.4-stretch as builder
WORKDIR /go/src/github.com/kubernetes-sigs/aws-efs-csi-driver

# Cache go modules
ENV GOPROXY=direct
COPY go.mod .
COPY go.sum .
RUN go mod download

ADD . .
RUN make aws-efs-csi-driver

FROM amazonlinux:2.0.20200406.0
RUN yum install util-linux-2.30.2-2.amzn2.0.4.x86_64 amazon-efs-utils-1.26-2.amzn2.noarch -y

# Default client source is k8s which can be overriden with –build-arg when building the Docker image
ARG client_source=k8s
RUN echo "client_source:${client_source}"
RUN printf "\n\
\n\
[client-info] \n\
source=${client_source} \n\
" >> /etc/amazon/efs/efs-utils.conf

COPY --from=builder /go/src/github.com/kubernetes-sigs/aws-efs-csi-driver/bin/aws-efs-csi-driver /bin/aws-efs-csi-driver
COPY THIRD-PARTY /

ENTRYPOINT ["/bin/aws-efs-csi-driver"]
