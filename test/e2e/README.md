# Prerequisites
- Amazon EFS file system
- kubernetes 1.14+ cluster whose workers (preferably 2 or more) can mount the Amazon EFS file system

# Run

## Run all CSI tests
```sh
go test ./test/e2e/ -v -kubeconfig=$HOME/.kube/config --region=us-west-2 --report-dir="./results" -ginkgo.focus="\[efs-csi\]" --cluster-name="cluster-name"
```

## Run single CSI test
```sh
go test ./test/e2e/ -v -kubeconfig=$HOME/.kube/config --region=us-west-2 --report-dir="./results" -ginkgo.focus="should continue reading/writing after the driver pod is upgraded from stable version" --cluster-name="cluster-name"
```

# Update dependencies
```sh
go mod edit -require=k8s.io/kubernetes@v1.15.3
./hack/update-gomod.sh v1.15.3
```
