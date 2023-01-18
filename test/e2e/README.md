## E2E Testing
E2E tests verify the functionality of the EFS CSI driver in the context of Kubernetes.


### Prerequisites
- Amazon EFS file system
- Kubernetes cluster, v1.22, whose workers (preferably 2 or more) can mount the Amazon EFS file system
- Golang v1.11+

### Presubmit Prow Jobs
We have Prow jobs defined in the Kubernetes test infrastructure repo that trigger the E2E tests to run on PRs against this repo.
See [aws-efs-csi-driver-presubmits.yaml](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/aws-efs-csi-driver/aws-efs-csi-driver-presubmits.yaml).

These will jobs will execute various targets defined in our [Makefile](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/Makefile), such as `test-e2e`.

### Trigger E2E Tests on a Local Cluster
If you have a v1.22 Kubernetes cluster, you can configure the E2E tests to execute against your cluster.

See the following example command which will build and deploy the csi driver to the cluster configured in your kube config,
and then will execute E2E tests against your existing filesystem.

```sh
export KUBECONFIG=$HOME/.kube/config
go test -v -timeout 0 ./... -report-dir=$ARTIFACTS -ginkgo.focus="\[efs-csi\]" -ginkgo.skip="\[Disruptive\]" \
  --file-system-id=$FS_ID --create-file-system=false --deploy-driver=true --region=$REGION
```

The E2E flags that you can pass to `go test` are defined in [e2e_test.go](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/test/e2e/e2e_test.go#L66-L75).
