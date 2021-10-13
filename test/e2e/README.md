# Prerequisites
- Amazon EFS file system or AWS credentials to create one
- kubernetes 1.14+ cluster whose workers (preferably 2 or more) can mount the Amazon EFS file system or AWS credentials to create one

# Run
Via make rule from repository root to create the file system and cluster:
```sh
TEST_ID=0 \
CLEAN=false \
KOPS_STATE_FILE=s3://aws-efs-csi-driver-e2e \
  make test-e2e
```
or from this directory with an existing file system and cluster:
```sh
go test -v -timeout 0 ./... -kubeconfig=$HOME/.kube/config -report-dir=$ARTIFACTS -ginkgo.focus="\[efs-csi\]" -ginkgo.skip="\[Disruptive\]" \
  -file-system-id=fs-c2a43e69
```

# Make binary
Via make rule from repository root:
```sh
make test-e2e-bin
```
