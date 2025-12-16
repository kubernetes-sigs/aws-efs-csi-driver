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

FROM public.ecr.aws/eks-distro-build-tooling/golang:1.25.5 as go-builder
WORKDIR /go/src/github.com/kubernetes-sigs/aws-efs-csi-driver

ARG TARGETOS
ARG TARGETARCH
RUN echo "TARGETOS:$TARGETOS, TARGETARCH:$TARGETARCH"
RUN echo "I am running on $(uname -s)/$(uname -m)"

ADD . .

# Default client source is `k8s` which can be overriden with –-build-arg when building the Docker image
ARG client_source=k8s
ENV EFS_CLIENT_SOURCE=$client_source

RUN OS=${TARGETOS} ARCH=${TARGETARCH} make $TARGETOS/$TARGETARCH

FROM public.ecr.aws/eks-distro-build-tooling/python:3.11-gcc-al23 as rpm-provider

# Install efs-utils from github by default. It can be overriden to `yum` with --build-arg when building the Docker image.
# If value of `EFSUTILSSOURCE` build arg is overriden with `yum`, docker will install efs-utils from Amazon Linux 2's yum repo.
ARG EFSUTILSSOURCE=github
RUN mkdir -p /tmp/rpms && \
    if [ "$EFSUTILSSOURCE" = "yum" ]; \
    then echo "Installing efs-utils from Amazon Linux 2 yum repo" && \
         yum -y install --downloadonly --downloaddir=/tmp/rpms amazon-efs-utils-1.35.0-1.amzn2.noarch; \
    else echo "Installing efs-utils from github using the latest git tag" && \
         yum -y install systemd git rpm-build make openssl-devel curl golang cmake && \
         curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y && \
         source $HOME/.cargo/env && \
         rustup update && \
         rustup default stable && \
         git clone https://github.com/aws/efs-utils && \
         cd efs-utils && \
         make rpm-without-system-rust && mv build/amazon-efs-utils*rpm /tmp/rpms && \
         # clean up efs-utils folder after install
         cd .. && rm -rf efs-utils && \
         yum clean all; \
    fi

# Install botocore required by efs-utils for cross account mount
RUN pip3 install --user botocore

# This image is equivalent to the eks-distro-minimal-base-python image but with pip installed as well
FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base-python-builder:3.11-al23 as rpm-installer

COPY --from=rpm-provider /tmp/rpms/* /tmp/download/

# second param indicates to skip installing dependency rpms, these will be installed manually
# cd, ls, cat, vim, tcpdump, are for debugging
RUN clean_install amazon-efs-utils true && \
    clean_install crypto-policies true && \
    clean_install "openssl openssl-libs openssl-fips-provider-latest" true && \
    install_binary \
        /usr/bin/cat \
        /usr/bin/cd \
        /usr/bin/df \
        /usr/bin/env \
        /usr/bin/find \
        /usr/bin/grep \
        /usr/bin/ln \
        /usr/bin/ls \
        /usr/bin/mount \
        /usr/bin/umount \
        /sbin/mount.nfs4 \
        /usr/bin/sed \
        /usr/bin/stat \
        /usr/bin/stunnel \
        /usr/sbin/tcpdump \
        /usr/bin/which && \
    cleanup "efs-csi"

# At image build time, static files installed by efs-utils in the config directory, i.e. CAs file, need
# to be saved in another place so that the other stateful files created at runtime, i.e. private key for
# client certificate, in the same config directory can be persisted to host with a host path volume.
# Otherwise creating a host path volume for that directory will clean up everything inside at the first time.
# Those static files need to be copied back to the config directory when the driver starts up.
RUN mv /newroot/etc/amazon/efs /newroot/etc/amazon/efs-static-files

FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base-python:3.11-al23 AS linux-amazon

COPY --from=rpm-installer /newroot /
COPY --from=rpm-provider /root/.local/lib/python3.11/site-packages/ /usr/lib/python3.11/site-packages/

COPY --from=go-builder /go/src/github.com/kubernetes-sigs/aws-efs-csi-driver/bin/aws-efs-csi-driver /bin/aws-efs-csi-driver
COPY THIRD-PARTY /

# Create a symbolic link for stunnel5 to stunnel (for backward compatibility)
RUN if [ -f /usr/bin/stunnel ]; then ln -s /usr/bin/stunnel /usr/bin/stunnel5; fi

ENTRYPOINT ["/bin/aws-efs-csi-driver"]
