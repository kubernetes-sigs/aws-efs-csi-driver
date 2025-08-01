# Enhanced Health Monitoring for AWS EFS CSI Driver

This document describes the enhanced health monitoring capabilities added to the AWS EFS CSI Driver to improve reliability and observability of EFS mount points.

## Overview

The enhanced health monitoring system provides:

- **Real-time mount health checking**: Continuously monitors EFS mount points with actual I/O tests
- **Multiple health endpoints**: Different endpoints for various health check requirements
- **Prometheus-style metrics**: Integration with monitoring systems for alerting and dashboards
- **Graceful degradation**: Prevents pod crash-loops when EFS mounts become temporarily unavailable

## Health Endpoints

The health monitoring system exposes several HTTP endpoints:

### `/healthz` - Overall Health Check
Returns the overall health status of the CSI driver. This endpoint returns:
- `200 OK` if all registered mounts are healthy
- `503 Service Unavailable` if any mount is unhealthy

**Example Response:**
```
CSI Driver is healthy
```

### `/healthz/ready` - Readiness Check
Strict readiness check that requires all mounts to be healthy. Suitable for Kubernetes readiness probes.
- `200 OK` if all mounts are healthy and responsive
- `503 Service Unavailable` if any mount fails health checks

### `/healthz/live` - Liveness Check
Lenient liveness check that only fails if the health monitoring system itself is broken. Suitable for Kubernetes liveness probes.
- `200 OK` if the health monitoring system is running
- `503 Service Unavailable` only if the monitoring system has failed

### `/healthz/mounts` - Detailed Mount Health
Returns detailed JSON information about all registered mount points and their health status.

**Example Response:**
```json
{
  "/var/lib/kubelet/pods/abc123/volumes/efs-pv/mount": {
    "mountPath": "/var/lib/kubelet/pods/abc123/volumes/efs-pv/mount",
    "isHealthy": true,
    "lastCheckTime": "2025-01-11T15:30:45Z",
    "responseTimeMs": 15,
    "errorCount": 0,
    "consecutiveErrors": 0,
    "lastError": ""
  }
}
```

## Configuration

The health monitoring system can be configured through environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `HEALTH_CHECK_INTERVAL` | `30s` | How often to perform health checks |
| `HEALTH_CHECK_TIMEOUT` | `5s` | Timeout for individual health checks |
| `HEALTH_PORT` | `9910` | Port for health endpoints |

## Integration with Kubernetes

### Liveness Probe Configuration

Add the following to your pod specification:

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: efs-plugin
    image: amazon/aws-efs-csi-driver:latest
    livenessProbe:
      httpGet:
        path: /healthz/live
        port: 9910
      initialDelaySeconds: 10
      timeoutSeconds: 3
      periodSeconds: 10
      failureThreshold: 5
```

### Readiness Probe Configuration

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: efs-plugin
    image: amazon/aws-efs-csi-driver:latest
    readinessProbe:
      httpGet:
        path: /healthz/ready
        port: 9910
      initialDelaySeconds: 5
      timeoutSeconds: 3
      periodSeconds: 5
      failureThreshold: 3
```

## Monitoring and Alerting

### Prometheus Metrics

The health monitoring system exposes Prometheus-compatible metrics at `/metrics`:

```
# HELP efs_mount_health_status Health status of EFS mounts (1=healthy, 0=unhealthy)
# TYPE efs_mount_health_status gauge
efs_mount_health_status{mount_path="/var/lib/kubelet/pods/abc123/volumes/efs-pv/mount"} 1

# HELP efs_mount_response_time_ms Response time for mount health checks in milliseconds
# TYPE efs_mount_response_time_ms gauge
efs_mount_response_time_ms{mount_path="/var/lib/kubelet/pods/abc123/volumes/efs-pv/mount"} 15

# HELP efs_mount_error_total Total number of errors for mount health checks
# TYPE efs_mount_error_total counter
efs_mount_error_total{mount_path="/var/lib/kubelet/pods/abc123/volumes/efs-pv/mount"} 0

# HELP efs_csi_driver_healthy Overall health status of the CSI driver (1=healthy, 0=unhealthy)
# TYPE efs_csi_driver_healthy gauge
efs_csi_driver_healthy 1
```

### Grafana Dashboard

Example queries for Grafana dashboards:

**Mount Health Status:**
```promql
efs_mount_health_status
```

**Average Response Time:**
```promql
avg(efs_mount_response_time_ms)
```

**Error Rate:**
```promql
rate(efs_mount_error_total[5m])
```

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
- name: efs-csi-driver
  rules:
  - alert: EFSMountUnhealthy
    expr: efs_mount_health_status == 0
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "EFS mount is unhealthy"
      description: "Mount {{ $labels.mount_path }} has been unhealthy for more than 2 minutes"

  - alert: EFSMountHighLatency
    expr: efs_mount_response_time_ms > 1000
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "EFS mount has high latency"
      description: "Mount {{ $labels.mount_path }} has response time > 1s for more than 5 minutes"

  - alert: EFSMountErrors
    expr: rate(efs_mount_error_total[5m]) > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "EFS mount experiencing errors"
      description: "Mount {{ $labels.mount_path }} has error rate > 0.1/sec for more than 2 minutes"
```

## Troubleshooting

### Health Check Failures

If health checks are failing:

1. **Check mount point accessibility:**
   ```bash
   curl http://localhost:9910/healthz/mounts
   ```

2. **Verify EFS connectivity:**
   ```bash
   # Check if the mount point is accessible
   ls -la /path/to/mount/point
   
   # Test write operations
   echo "test" > /path/to/mount/point/health_test.tmp
   rm /path/to/mount/point/health_test.tmp
   ```

3. **Check logs for specific errors:**
   ```bash
   kubectl logs <pod-name> | grep -i health
   ```

### Common Issues

**Mount appears healthy but applications fail:**
- The health check only tests basic I/O operations
- Application-specific permissions or file locks may still cause issues
- Consider implementing application-specific health checks

**Health checks timeout:**
- EFS may be experiencing high latency
- Check EFS performance mode and throughput settings
- Consider increasing `HEALTH_CHECK_TIMEOUT`

**Frequent health check failures:**
- Network connectivity issues to EFS
- EFS file system may be in a degraded state
- Check AWS EFS console for file system status

## Migration from Basic Health Checks

The enhanced health monitoring is backward compatible with existing CSI livenessprobe configurations. The original `/healthz` endpoint continues to work as before, but now provides more meaningful health status.

### Before
```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 9909  # Original port
  failureThreshold: 5
```

### After (Recommended)
```yaml
livenessProbe:
  httpGet:
    path: /healthz/live
    port: 9910  # New dedicated health port
  failureThreshold: 5
readinessProbe:
  httpGet:
    path: /healthz/ready
    port: 9910
  failureThreshold: 3
```

## Performance Considerations

The health monitoring system is designed to be lightweight:

- **Memory usage**: ~10KB per registered mount point
- **CPU overhead**: <1% during health checks
- **I/O operations**: Small temporary files (1KB) written/read/deleted during checks
- **Network**: No additional network calls beyond local health checks

The system automatically adjusts check frequency based on mount health status and uses context cancellation to prevent blocking operations.
