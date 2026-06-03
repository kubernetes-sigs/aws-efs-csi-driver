# Driver Metrics

## Overview

The EFS CSI Driver supports emitting metrics via an HTTP endpoint from the controller pod in the standard [Prometheus exposition format](https://prometheus.io/docs/instrumenting/exposition_formats/). Most metrics systems support ingesting the Prometheus format, including [Prometheus](https://prometheus.io/), [CloudWatch](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/ContainerInsights-Prometheus-metrics.html), [InfluxDB](https://docs.influxdata.com/influxdb/v1/supported_protocols/prometheus/), and many others.

When installing via Helm, metrics can be configured via Helm parameters:
- Metrics may be enabled by setting `controller.enableMetrics` to `true`.
  - This deploys a `Service` object for the controller pod.
  - By default, the controller pod is annotated with `prometheus.io/scrape` and `prometheus.io/port`. This can be controlled via `controller.enablePrometheusAnnotations`.
  - Prometheus Operator `ServiceMonitor` resources can be enabled by setting `controller.serviceMonitor.enabled` to `true`. This requires the Prometheus Operator CRDs to be installed.

## AWS API Metrics (`efs-csi-controller`)

The EFS CSI Driver emits [AWS EFS API](https://docs.aws.amazon.com/efs/latest/ug/api-reference.html) metrics to `0.0.0.0:3301/metrics` when `controller.enableMetrics: true` is set in the Helm chart.

The following metrics are supported:

| Metric name | Metric type | Description | Labels |
|-------------|-------------|-------------|--------|
| `aws_efs_csi_api_request_duration_seconds` | Histogram | AWS SDK API request duration by request type in seconds | `request=<API operation name>`, `service=<efs\|s3files>` |
| `aws_efs_csi_api_request_errors_total` | Counter | Total number of AWS SDK API errors by error code and request type | `request=<API operation name>`, `service=<efs\|s3files>`, `code=<error code>` |
| `aws_efs_csi_api_request_throttles_total` | Counter | Total number of throttled AWS SDK API requests per request type | `request=<API operation name>`, `service=<efs\|s3files>` |

Instrumented EFS API operations (`service="efs"`): `CreateAccessPoint`, `DeleteAccessPoint`, `DescribeAccessPoints`, `DescribeFileSystems`, `DescribeMountTargets`.

Instrumented S3Files API operations (`service="s3files"`): `CreateAccessPoint`, `DeleteAccessPoint`, `ListAccessPoints`, `ListFileSystems`.

## TLS

The metrics endpoint can be served over TLS by providing `--metrics-cert-file` and `--metrics-key-file` flags to the driver.
