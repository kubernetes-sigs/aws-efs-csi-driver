# Development Guide

How to build, test, and contribute changes to the Amazon EFS CSI driver.

## Project overview

The Amazon EFS CSI driver implements the [Container Storage Interface](https://github.com/container-storage-interface/spec/blob/master/spec.md) so Kubernetes can manage Amazon EFS file systems. It's written in Go (see `go.mod` for the version) and ships as a container image and a Helm chart.

| Path | Contents |
|---|---|
| `cmd/` | Driver entry point (`main.go`) |
| `pkg/driver/` | CSI controller, node, and identity services, mounting, access point / GID allocation |
| `pkg/cloud/` | AWS API clients, instance and task metadata, retries; mocks in `pkg/cloud/mocks` |
| `pkg/metrics/`, `pkg/util/` | Metrics and shared helpers |
| `charts/aws-efs-csi-driver/` | Helm chart and its helm-unittest tests |
| `deploy/kubernetes/` | Kustomize manifests generated from the chart |
| `test/e2e/` | Ginkgo end-to-end tests (see `test/e2e/README.md`) |
| `hack/` | Verify, update, release, and e2e helper scripts |
| `docs/`, `examples/`, `troubleshooting/` | User documentation, examples, log collection |
| `vendor/` | Vendored dependencies (builds use `-mod=vendor`) |

## Build and test

```sh
make bin/aws-efs-csi-driver   # build the driver binary
make verify                   # gofmt, go vet, golint (hack/verify-all)
make test                     # helm unittest + go test -race ./pkg/...
make image                    # build a container image
make generate-kustomize       # regenerate deploy/kubernetes from the Helm chart
```

E2E tests need a Kubernetes cluster and EFS resources. See `test/e2e/README.md`.

## Conventions

- Run `make verify` and `make test` before opening a PR. CI and Prow presubmits run the same checks.
- Format with `gofmt`. Follow the existing style in the package you're editing.
- Add or update unit tests next to the code you change (`*_test.go` in the same package).
- If the PR is complex, also add e2e test cases to `test/e2e/e2e.go`. That includes new features, mount or provisioning behavior changes, changes across the controller and node paths, and anything unit tests with mocks can't fully cover. Add `ginkgo.It(...)` cases under the existing `[efs-csi]` `Describe` block, following the patterns already there. For a test that restarts pods or disrupts the cluster, put `[Disruptive]` (or `[Serial]`) in the test name so the default run skips it: `make test-e2e` passes `GINKGO_SKIP="\[Disruptive\]|\[Serial\]"`, and ginkgo matches that regex against the spec's text, not its decorators. Add the `ginkgo.Serial` decorator as well if the test also must not run alongside others; it controls parallelism only and does not affect skipping. Use the existing e2e flags for environment-specific settings so the tests run on any cluster.
- When you change an interface that has a mock (`pkg/cloud/mocks`, `pkg/driver/mocks`), regenerate the mock with `hack/update-gomock`. Don't hand-edit it.
- When you add or bump a dependency, update `go.mod`/`go.sum` and run `go mod vendor` so `vendor/` stays in sync.
- When you change chart templates or values, update the chart's helm-unittest tests and run `make generate-kustomize` if the rendered manifests change.
- When you change driver parameters or behavior, update the matching docs under `docs/` (for example `docs/parameters.md`).
- When you change the IAM permissions the driver needs, update `docs/iam-policy-example.json` (see `docs/CONTRIBUTING.md`).
- Don't edit `CHANGELOG-*.md` unless you're preparing a release.

## Pull requests

- Fill in `.github/PULL_REQUEST_TEMPLATE.md`: bug fix or feature, what and why, and what testing you did.
- Reference GitHub issues (`Fixes #123`) where they apply.
- Keep commits focused and write clear commit messages. Contributors must have signed the Kubernetes CLA.
