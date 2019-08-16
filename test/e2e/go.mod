module github.com/kubernetes-sigs/aws-efs-csi-driver/test/e2e

go 1.12

replace k8s.io/api => k8s.io/api v0.0.0-20190805141119-fdd30b57c827

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190805143126-cdb999c96590

replace k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719

replace k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190805142138-368b2058237c

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190805143448-a07e59fb081d

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190805141520-2fe0317bcee0

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190805144409-8484242760e7

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20190805144246-c01ee70854a1

replace k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b

replace k8s.io/component-base => k8s.io/component-base v0.0.0-20190805141645-3a5e5ac800ae

replace k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190531030430-6117653b35f1

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20190805144531-3985229e1802

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190805142416-fd821fbbb94e

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20190805144128-269742da31dd

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20190805143734-7f1675b90353

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20190805144012-2a1ed1f3d8a4

replace k8s.io/kubectl => k8s.io/kubectl v0.0.0-20190602132728-7075c07e78bf

replace k8s.io/kubelet => k8s.io/kubelet v0.0.0-20190805143852-517ff267f8d1

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20190805144654-3d5bf3a310c1

replace k8s.io/metrics => k8s.io/metrics v0.0.0-20190805143318-16b07057415d

replace k8s.io/node-api => k8s.io/node-api v0.0.0-20190805144819-9dd62e4d5327

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20190805142637-3b65bc4bb24f

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.0.0-20190805143616-1485e5142db3

replace k8s.io/sample-controller => k8s.io/sample-controller v0.0.0-20190805142825-b16fad786282

require (
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/kubernetes v1.15.2
)
