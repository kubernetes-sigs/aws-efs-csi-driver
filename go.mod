module github.com/kubernetes-sigs/aws-efs-csi-driver

require (
	github.com/aws/aws-sdk-go-v2 v1.31.0
	github.com/aws/aws-sdk-go-v2/config v1.27.35
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.13
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.178.0
	github.com/aws/aws-sdk-go-v2/service/efs v1.31.8
	github.com/aws/smithy-go v1.21.0
	github.com/container-storage-interface/spec v1.7.0
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.3.1
	github.com/kubernetes-csi/csi-test/v5 v5.0.0
	github.com/mitchellh/go-ps v0.0.0-20170309133038-4fdf99ab2936
	github.com/onsi/ginkgo/v2 v2.9.0
	github.com/onsi/gomega v1.27.1
	golang.org/x/exp v0.0.0-20230817173708-d852ddb80c63
	google.golang.org/grpc v1.59.0
	k8s.io/api v0.26.15
	k8s.io/apimachinery v0.26.15
	k8s.io/client-go v0.26.15
	k8s.io/klog/v2 v2.90.1
	k8s.io/kubernetes v1.27.16
	k8s.io/mount-utils v0.26.15
	k8s.io/pod-security-admission v0.26.15
)

require (
	github.com/aws/aws-sdk-go v1.50.3 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.33 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.18 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.18 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.22.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.26.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.30.8 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/emicklei/go-restful/v3 v3.12.0 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.18.0 // indirect
	github.com/imdario/mergo v0.3.6 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/selinux v1.10.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.14.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/spf13/cobra v1.6.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.45.0 // indirect
	go.opentelemetry.io/otel v1.19.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.19.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.19.0 // indirect
	go.opentelemetry.io/otel/metric v1.19.0 // indirect
	go.opentelemetry.io/otel/sdk v1.19.0 // indirect
	go.opentelemetry.io/otel/trace v1.19.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	golang.org/x/crypto v0.23.0 // indirect
	golang.org/x/net v0.23.0 // indirect
	golang.org/x/oauth2 v0.12.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/term v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.12.1-0.20230815132531-74c255bcf846 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231030173426-d783a09b4405 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231030173426-d783a09b4405 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.26.11 // indirect
	k8s.io/apiserver v0.26.11 // indirect
	k8s.io/cloud-provider v0.26.11 // indirect
	k8s.io/component-base v0.26.11 // indirect
	k8s.io/component-helpers v0.26.11 // indirect
	k8s.io/csi-translation-lib v0.26.11 // indirect
	k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280 // indirect
	k8s.io/kubectl v0.26.11 // indirect
	k8s.io/utils v0.0.0-20221107191617-1a15be271d1d // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.37 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.26.11
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.26.11
	k8s.io/apimachinery => k8s.io/apimachinery v0.26.11
	k8s.io/apiserver => k8s.io/apiserver v0.26.11
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.26.11
	k8s.io/client-go => k8s.io/client-go v0.26.11
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.26.11
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.26.11
	k8s.io/code-generator => k8s.io/code-generator v0.26.11
	k8s.io/component-base => k8s.io/component-base v0.26.11
	k8s.io/component-helpers => k8s.io/component-helpers v0.26.11
	k8s.io/controller-manager => k8s.io/controller-manager v0.26.11
	k8s.io/cri-api => k8s.io/cri-api v0.26.11
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.26.11
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.26.11
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.26.11
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.26.11
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.26.11
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.26.11
	k8s.io/kubectl => k8s.io/kubectl v0.26.11
	k8s.io/kubelet => k8s.io/kubelet v0.26.11
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.26.11
	k8s.io/metrics => k8s.io/metrics v0.26.11
	k8s.io/mount-utils => k8s.io/mount-utils v0.26.11
	k8s.io/node-api => k8s.io/node-api v0.26.11
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.26.11
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.26.11
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.26.11
	k8s.io/sample-controller => k8s.io/sample-controller v0.26.11
	vbom.ml/util => github.com/fvbommel/util v0.0.0-20180919145318-efcd4e0f9787
)

go 1.22.5
