## E2E Testing
E2E tests verify the functionality of the EFS CSI driver in the context of Kubernetes.

### Prerequisites
- Kubernetes cluster (v1.30+) whose nodes can reach the EFS/S3Files mount targets
- `KUBECONFIG` set, or `~/.kube/config` present
- Golang v1.25+

### Presubmit Prow Jobs
Prow jobs are defined in the Kubernetes test infrastructure repo and trigger on PRs.
See [aws-efs-csi-driver-presubmits.yaml](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/aws-efs-csi-driver/aws-efs-csi-driver-presubmits.yaml).

### Trigger E2E Tests on a Local Cluster
If you have a v1.30 Kubernetes cluster, you can configure the E2E tests to execute against your cluster.

See the following example command which will build and deploy the csi driver to the cluster configured in your kube config,
and then will execute E2E tests against your existing filesystem.

```sh
export KUBECONFIG=$HOME/.kube/config
go test -v -timeout 0 ./... -report-dir=$ARTIFACTS -ginkgo.focus="\[efs-csi\]" -ginkgo.skip="\[Disruptive\]" \
  --file-system-id=$FS_ID --create-file-system=false --deploy-driver=true --region=$REGION
```

The E2E flags that you can pass to `go test` are defined in [e2e_test.go](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/test/e2e/e2e_test.go#L62-L78).


### Running Cross-Account E2E Tests
Cross-account tests verify EFS behaviors when the filesystem is in a different AWS account than the EKS cluster. The same standard e2e test suite runs against a cluster pre-configured with cross-account Secret and StorageClass.

### Running Upgrade Test

The upgrade test verifies that existing mounts survive a driver upgrade and that new mounts work after upgrading. It installs the latest released driver from the public Helm chart, writes data to a volume, upgrades to the new version using the local chart, then confirms that pre-upgrade data is still readable, writes to the existing mount still work, and new mounts can be created and used.

Both EFS and S3Files upgrade paths are tested sequentially.

#### Prerequisites
- EFS filesystem (`--file-system-id`) and/or S3Files filesystem (`--s3files-file-system-id`)
- EFS CSI controller/node service accounts already deployed on the cluster (the test manages the driver itself via Helm)
- `helm` installed and on your `$PATH`

#### Run

```sh
export KUBECONFIG=$HOME/.kube/config
go test -v -timeout 0 ./test/e2e/... \
  -ginkgo.focus="\[efs-csi\]" --label-filter="Disruptive" \
  --file-system-id=$FS_ID \
  --upgrade-new-image-tag=$NEW_TAG \
  --region=$REGION
```

The old driver version is always the latest public release, installed automatically from the public Helm chart. The new version is installed from the local chart at `charts/aws-efs-csi-driver`. The test cleans up by uninstalling the driver at the end.
