# Prerequisites
- Amazon EFS file system
- kubernetes 1.14+ cluster whose workers (preferably 2 or more) can mount the Amazon EFS file system

# Run
```sh
go test -v -timeout 0 ./... -kubeconfig=$HOME/.kube/config -report-dir=$ARTIFACTS -ginkgo.focus="\[efs-csi\]" -ginkgo.skip="\[Disruptive\]" \
  -file-system-id=fs-c2a43e69
```

# Make binary
```sh
make test-e2e-bin
```
