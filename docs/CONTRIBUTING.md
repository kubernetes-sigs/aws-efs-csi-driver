# Development
* Please go through [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and [Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs) to get some basic understanding of CSI driver before you start.

* If you are about to update iam policy file, please also update efs policy in weaveworks/eksctl
https://github.com/weaveworks/eksctl/blob/main/pkg/cfn/builder/statement.go
*/

## Requirements
* Golang 1.13.4+

## Dependency
Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, to build the project run: `make`

## Testing
To execute all unit tests, run: `make test`

## Troubleshooting
To pull logs and troubleshoot the driver, see [troubleshooting/README.md](../troubleshooting/README.md).

## License
This library is licensed under the Apache 2.0 License.