# Usage

run.sh will build and push a driver image, create a kops cluster, helm install the driver pointing to the built image, run ginkgo tests, then clean everything up.

See below for an example.

KOPS_STATE_FILE is an S3 bucket you have write access to.

TEST_ID is a token used for idempotency.

For more details, see the script itself.

For more examples, see the top-level Makefile.

```
TEST_PATH=./tests/e2e-migration/... \
EBS_CHECK_MIGRATION=true \
TEST_ID=18512 \
CLEAN=false \
KOPS_STATE_FILE=s3://mattwon \
AWS_REGION=us-west-2 \
AWS_AVAILABILITY_ZONES=us-west-2a \
GINKGO_FOCUS=Dynamic.\*xfs.\*should.store.data \
GINKGO_NODES=1 \
./hack/e2e/run.sh
```

# git subtree

Reference: https://github.com/kubernetes-csi/csi-release-tools/#sharing-and-updating

How to consume this directory by subtreeing the ebs repo:

```
git remote add ebs git@github.com:kubernetes-sigs/aws-ebs-csi-driver.git
git fetch ebs
git checkout -b ebsmaster ebs/master
git subtree split --squash --prefix=hack/e2e -b ebsmasterhacke2e
git checkout master
git subtree add --squash --prefix=hack/e2e ebsmasterhacke2e
```

To commit changes and submit them as a PR back to the ebs repo:

```
git commit
git subtree push --prefix=hack/e2e git@github.com:<user>/aws-ebs-csi-driver.git <my-new-or-existing-branch>
```
