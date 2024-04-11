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


### Running Upgrade Test
In order to test upgrades from previous releases to the current development version of the driver, the following steps can be followed:

1. Ensure the EFS CSI Driver is not currently deployed on your cluster.
2. Ensure that the EFS CSI Node and Controller service accounts are deployed on your cluster. Ex:
```
$ cat ~/efs-service-account.yaml 
---
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/name: aws-efs-csi-driver
  name: efs-csi-controller-sa
  namespace: kube-system
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789:role/test-cluster-iam-sa-role
---
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/name: aws-efs-csi-driver
  name: efs-csi-node-sa
  namespace: kube-system
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789:role/test-cluster-iam-sa-role

$ kubectl apply -f ~/efs-service-account.yaml
```
3. Run the upgrade script, for example:
```sh
# The release version of the driver you would like to test upgrading from. 
# Pulls most recent image from the release-$PREV_RELEASE branch
PREV_RELEASE=1.7
# Region of private ECR
REGION=us-east-1
# Account of private ECR
ACCOUNT=123456789
chmod +x ./upgrade_driver_version.sh
./upgrade_driver_version.sh $PREV_RELEASE $REGION $ACCOUNT
```
4. Run the e2e tests, note that $REGION should be that of the EKS cluster.
```sh
export KUBECONFIG=$HOME/.kube/config
go test -v -timeout 0 ./... -report-dir=$ARTIFACTS -ginkgo.focus="\[efs-csi\]" -ginkgo.skip="\[Disruptive\]" \
  --file-system-id=$FS_ID --create-file-system=false --deploy-driver=false --region=$REGION
```
5. Clean Up: The driver + kubernetes service accounts can be cleaned up via the following command:
```sh
kubectl delete -k github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=master
```