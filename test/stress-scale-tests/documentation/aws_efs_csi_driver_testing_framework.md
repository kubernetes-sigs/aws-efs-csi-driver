# AWS EFS CSI Driver Stress & Scale Testing Framework Documentation

## 1. System Overview

The AWS EFS CSI Driver stress & scale testing framework is designed to rigorously test the Elastic File System CSI driver in Kubernetes environments. It simulates various workloads and scenarios to verify the driver's reliability, performance, and behavior under different conditions.

## 2. Architecture

The framework is built with a modular architecture consisting of the following components:

```
aws-efs-csi-driver/test/stress-scale-tests/
├── run_tests.py              # Main entry point script
├── tests/
│   └── orchestrator.py       # Core test orchestration logic
├── utils/
│   ├── metrics_collector.py  # Collects performance metrics
│   ├── log_integration.py    # Handles log collection
│   └── report_generator.py   # Generates test reports
├── monitoring/
│   ├── setup_monitoring.sh   # Sets up monitoring stack
│   ├── prometheus_exporter.py# Exports metrics to Prometheus
│   ├── dashboards/           # Grafana dashboards
│   └── kubernetes/           # K8s monitoring manifests
├── config/
│   └── orchestrator_config.yaml # Test configuration
└── logs/                     # Test logs directory
```

## 3. Execution Flow

### 3.1 Test Initialization

1. The process begins when a user runs `run_tests.py`, typically with configuration options.
2. The script parses command-line arguments and loads the configuration from YAML files.
3. The system verifies AWS credentials and sets up logging.
4. It initializes the metrics collector and report generators.
5. Based on the selected test suite (orchestrator in this case), it initializes the `EFSCSIOrchestrator`.

### 3.2 Test Execution

1. The orchestrator sets up a Kubernetes storage class if needed.
2. It runs initial "warm-up" operations to ensure coverage of all operation types.
3. The main testing loop begins, where it:
   - Randomly selects operations based on configured weights
   - Executes operations (create PVCs, attach pods, test read/write, etc.)
   - Collects metrics and logs for each operation
   - Continues until the specified test duration is reached
4. During execution, specific test scenarios are also run periodically:
   - Many-to-one: Multiple pods mounting a single PVC
   - One-to-one: One pod per PVC
   - Concurrent PVC: Rapid creation and deletion of PVCs

### 3.3 Test Completion

1. After the test completes or is interrupted, a comprehensive cleanup is performed.
2. Test results and metrics are compiled into a report.
3. The report is saved to the reports directory.

## 4. Key Components

### 4.1 EFSCSIOrchestrator

The core component that manages test execution. Key responsibilities:

- **Resource Management**: Creates, tracks, and cleans up Kubernetes resources (PVCs, pods).
- **Operation Execution**: Performs operations like creating PVCs, attaching pods, verifying read/write.
- **Scenario Execution**: Runs specific test scenarios (many-to-one, one-to-one, concurrent PVC).
- **Metrics Collection**: Tracks performance metrics for operations.
- **Error Handling**: Detects and logs failures, collects diagnostic information.

#### Detailed Class Structure

The `EFSCSIOrchestrator` class is initialized with:

```python
def __init__(self, config_file='config/orchestrator_config.yaml', namespace=None, metrics_collector=None, driver_pod_name=None):
```

The initialization process is modularized into several helper methods:
- `_init_configuration`: Loads configuration from file
- `_init_kubernetes_clients`: Sets up Kubernetes API clients
- `_init_metrics_collector`: Initializes metrics collection
- `_init_logging`: Configures logging infrastructure
- `_init_test_parameters`: Sets test runtime parameters
- `_init_resource_tracking`: Initializes tracking structures

Key methods include:
- `run_test()`: Main entry point that runs the test for the configured duration
- `_get_operations_and_weights()`: Sets up weighted operations for random selection
- `_run_random_operation()`: Selects and executes a random operation based on weights
- Resource operations: `_create_pvc()`, `_attach_pod()`, `_delete_pod()`, `_delete_pvc()`
- Validation methods: `_verify_readwrite()`, `_wait_for_pod_ready()`, `_wait_for_pvc_bound()`

### 4.2 Operations Framework

The orchestrator executes operations based on weighted probabilities:

1. **Create PVC** (`_create_pvc`): Creates a PersistentVolumeClaim using EFS CSI driver.
2. **Attach Pod** (`_attach_pod`): Attaches a pod to a PVC with appropriate volume mounts.
3. **Delete Pod** (`_delete_pod`): Removes pods and tracks resource cleanup.
4. **Delete PVC** (`_delete_pvc`): Removes PVCs after ensuring no pods are using them.
5. **Verify ReadWrite** (`_verify_readwrite`): Tests read/write operations between pods.
6. **Run Specific Scenario** (`_run_specific_scenario`): Executes one of the defined test scenarios.

#### Pod Manifest Building

Pod manifests are built in a modular fashion using configuration values:

- `_build_pod_manifest`: Orchestrates the pod manifest creation process
- `_build_container_spec`: Creates container specifications using values from config
- `_build_readiness_probe`: Sets up readiness probe with configured parameters
- `_build_container_resources`: Sets resource limits based on config values
- `_build_pod_spec`: Creates the complete pod spec with volume mounts, tolerations, etc.

#### PVC Creation

PVC creation is also modularized to use configuration values:

- `_build_pvc_manifest`: Creates the PVC manifest based on configuration
- `_add_pvc_metadata`: Adds metadata like annotations and labels
- `_create_and_wait_for_pvc`: Creates the PVC and waits for it to be bound

#### Operation Selection Process

Operations are selected using a weighted random selection process:

```python
def _run_random_operation(self, operations, cumulative_weights, total_weight, operation_counts):
    random_val = random.uniform(0, total_weight)
    for i, (operation, _) in enumerate(operations):
        if random_val <= cumulative_weights[i]:
            op_name = operation.__name__
            operation_counts[op_name] = operation_counts.get(op_name, 0) + 1
            self.logger.info(f"Selected operation: {op_name} (selected {operation_counts[op_name]} times)")
            operation()
            break
```

### 4.3 Test Scenarios

Special scenarios that test specific aspects of the EFS CSI driver:

1. **Many-to-One** (`_scenario_many_to_one`): Tests multiple pods mounting a single PVC.
   - Creates a dedicated PVC
   - Creates multiple pods all mounting the same volume
   - Tests read/write operations between pods to verify data consistency
   - Collects diagnostics if failures occur
   - Uses configuration values from `scenarios.many_to_one.min_pods` and `max_pods`

2. **One-to-One** (`_scenario_one_to_one`): Tests one pod per PVC.
   - Creates multiple PVCs
   - Attaches one pod to each PVC
   - Verifies each pod can write to its volume
   - Uses configuration values from `scenarios.one_to_one.min_pairs` and `max_pairs`
   
3. **Concurrent PVC** (`_scenario_concurrent_pvc`): Tests rapid PVC creation and deletion.
   - Creates multiple PVCs in quick succession
   - Creates pods for some PVCs
   - Deletes PVCs rapidly to test resource cleanup
   - Uses configuration values from `scenarios.concurrent_pvc.min_pvcs` and `max_pvcs`

#### Modular Scenario Implementation

The many-to-one scenario is implemented in a modular fashion, breaking down the process into logical steps:

```python
def _scenario_many_to_one(self):
    # Record scenario run
    self.scenarios['many_to_one']['runs'] += 1
    
    try:
        # Step 1: Create PVC
        pvc_name = self._create_many_to_one_pvc()
        if not pvc_name:
            self.scenarios['many_to_one']['fail'] += 1
            return

        # Step 2: Create multiple pods
        pod_names = self._create_many_to_one_pods(pvc_name)
        if len(pod_names) < 2:
            self.scenarios['many_to_one']['fail'] += 1
            return
        
        # Step 3: Test read/write between pods
        success = self._test_many_to_one_readwrite(pvc_name, pod_names)
        if success:
            self.scenarios['many_to_one']['success'] += 1
        else:
            self.scenarios['many_to_one']['fail'] += 1
            # Collect diagnostic information
            if len(pod_names) >= 2:
                writer_pod = pod_names[0]
                reader_pod = pod_names[1]
                self._collect_many_to_one_failure_logs(pvc_name, writer_pod, reader_pod)
    
    except Exception as e:
        self.scenarios['many_to_one']['fail'] += 1
        # Log exception details
    
    # Clean up will be handled by the orchestrator's cleanup method
```

#### Configuration-Driven Scenarios

All scenarios use configuration values instead of hardcoded values:

```yaml
scenarios:
  concurrent_pvc:
    enabled: true
    max_pvcs: 30
    min_pvcs: 20
  many_to_one:
    enabled: true
    max_pods: 20
    min_pods: 10
  one_to_one:
    enabled: true
    max_pairs: 20
    min_pairs: 10
```

These values control the number of resources created during each scenario, allowing for easy adjustment without code changes.

### 4.4 Metrics Collection

The `MetricsCollector` class tracks performance metrics:

- **File Operation Latency**: Time taken for read/write/metadata operations.
- **IOPS**: I/O operations per second for different operation types.
- **Throughput**: Data transfer rates for read/write operations.
- **Success Rates**: Success/failure rates for various operations.

#### Metric Types

```python
# Example metric collection for file operations
def track_file_operation_latency(self, pvc_name, operation_type, latency):
    """Track latency for file operations
    
    Args:
        pvc_name: Name of PVC
        operation_type: Type of operation ('read', 'write', 'metadata')
        latency: Operation latency in seconds
    """
    # Record metrics in internal data structures for later reporting
```

### 4.5 Monitoring

Two monitoring options are available:

1. **Kubernetes Deployment**: Deploys Prometheus and Grafana in the cluster.
   - Automatically collects metrics from the exporter
   - Provides pre-configured dashboards for visualization

2. **Standalone Mode**: Runs a local Prometheus exporter.
   - Simpler setup for development environments
   - Exports metrics on a local endpoint (default: http://localhost:8000/metrics)

#### Monitoring Setup Process

```bash
# Start monitoring
./monitoring/setup_monitoring.sh

# Option 1: Kubernetes deployment
kubectl apply -f monitoring/kubernetes/namespace.yaml
kubectl apply -f monitoring/kubernetes/prometheus-configmap.yaml
kubectl apply -f monitoring/kubernetes/prometheus-deployment.yaml
kubectl apply -f monitoring/kubernetes/grafana-dashboards.yaml
kubectl apply -f monitoring/kubernetes/grafana-deployment.yaml
kubectl apply -f monitoring/kubernetes/metrics-exporter.yaml

# Option 2: Standalone exporter
pip install prometheus-client psutil pyyaml
python monitoring/prometheus_exporter.py --port 8000 --watch-dir reports/orchestrator/
```

## 5. Running the Tests

### 5.1 Basic Execution

```bash
python run_tests.py
```

### 5.2 Common Options

```bash
python run_tests.py --config config/orchestrator_config.yaml --duration 3600 --interval 5
```

Parameters:
- `--config`: Path to configuration file
- `--duration`: Test duration in seconds
- `--interval`: Seconds to wait between operations
- `--driver-pod-name`: Name of EFS CSI driver pod (for log collection)

### 5.3 Setting Up Monitoring

```bash
# Start monitoring
./monitoring/setup_monitoring.sh

# Option 1: Kubernetes deployment (requires kubectl access)
# Access Grafana at http://localhost:3000 after running:
kubectl port-forward -n monitoring svc/grafana 3000:3000

# Option 2: Standalone exporter
# Metrics available at http://localhost:8000/metrics
```

## 6. Configuration

The main configuration file (`config/orchestrator_config.yaml`) controls test behavior:

```yaml
# Test parameters
test:
  namespace: "default"
  duration:   # Test duration in seconds
  operation_interval: 3  # Seconds between operations

# Resource limits
resource_limits:
  max_pvcs: 
  max_pods_per_pvc: 
  total_max_pods: 

# Operation weights (probability distribution)
operation_weights:
  create_pvc: 
  attach_pod: 
  delete_pod: 
  delete_pvc: 
  verify_readwrite: 
  run_specific_scenario: 

# Pod configuration
pod_config:
  image: "alpine:latest"
  command: ["/bin/sh", "-c"]
  args: ["touch /data/pod-ready && while true; do sleep 30; done"]
  readiness_probe:
    initial_delay_seconds: 
    period_seconds: 
  node_selector:
    topology.kubernetes.io/zone: us-west-1b
  tolerations:
  - effect: NoSchedule
    key: instance
    operator: Equal
    value: core

# Scenario-specific configuration
scenarios:
  concurrent_pvc:
    enabled: true
    max_pvcs: 
    min_pvcs: 
  many_to_one:
    enabled: true
    max_pods: 
    min_pods: 
  one_to_one:
    enabled: true
    max_pairs: 
    min_pairs: 
```

### 6.1 Custom Configuration

You can create custom configurations for specific test cases. The framework will look for:

1. Command-line specified config: `--config custom_config.yaml`
2. Dedicated orchestrator config: `config/orchestrator_config.yaml`
3. Default config: The main config passed to run_tests.py

### 6.2 Configuration Usage in Code

The framework is designed to use configuration values wherever possible, reducing hardcoded values:

1. **Pod Configuration**: The `_build_pod_manifest` function uses configuration values for:
   - Container image
   - Commands and arguments
   - Readiness probe parameters
   - Resource limits
   - Node selector and tolerations

2. **PVC Configuration**: The `_create_pvc` function uses configuration for:
   - Storage class name
   - Access modes
   - Storage size
   - Annotations and labels

3. **Scenario Configuration**: Test scenarios use configuration for resource counts:
   - Many-to-one: Number of pods per PVC
   - One-to-one: Number of PVC-pod pairs
   - Concurrent PVC: Number of PVCs to create/delete

4. **Test Parameters**: Test behavior is configured through:
   - Test duration
   - Operation interval
   - Resource limits
   - Operation weights

## 7. Understanding Test Results

### 7.1 Report Structure

Test reports are generated in `reports/orchestrator/` with metrics including:

- **Operations**: Success/failure counts and rates for each operation type
- **Scenarios**: Results for each test scenario
- **File Performance**: Metrics for read/write operations
  - IOPS (operations per second)
  - Latency (operation duration)
  - Throughput (MB/s)

Example report structure:
```json
{
  "test_name": "efs_orchestrator_20250613_162210",
  "test_type": "orchestrator",
  "timestamp": "20250613_162210",
  "system_info": {
    "kubernetes_version": "1.24.7",
    "nodes": 3,
    "platform": "eks"
  },
  "results": {
    "operations": {
      "create_pvc": {"success": 45, "fail": 2},
      "attach_pod": {"success": 120, "fail": 5},
      "delete_pod": {"success": 110, "fail": 0},
      "delete_pvc": {"success": 40, "fail": 0},
      "verify_write": {"success": 80, "fail": 3},
      "verify_read": {"success": 77, "fail": 3}
    },
    "scenarios": {
      "shared_volume_rw": {"runs": 25, "success": 22, "fail": 3},
      "many_to_one": {"runs": 10, "success": 9, "fail": 1},
      "one_to_one": {"runs": 8, "success": 8, "fail": 0},
      "concurrent_pvc": {"runs": 5, "success": 4, "fail": 1}
    }
  },
  "metrics": {
    "file_performance": {
      "overall": {
        "read": {
          "latency_avg": 0.0325,
          "iops_avg": 30.76,
          "throughput_avg": 5.43
        },
        "write": {
          "latency_avg": 0.0412,
          "iops_avg": 24.27,
          "throughput_avg": 4.21
        }
      }
    }
  }
}
```

### 7.2 Logs

Logs are available in `logs/` directory:
- `orchestrator_YYYYMMDD_HHMMSS.log`: Regular execution logs
- `efs_orchestrator_failure_*`: Logs collected on test failures

#### Log Analysis Example

Regular execution logs contain detailed operation information:
```
2025-06-13 16:22:10,342 - tests.orchestrator - INFO - Selected operation: _create_pvc (selected 4 times)
2025-06-13 16:22:10,456 - tests.orchestrator - INFO - Created PVC: test-pvc-3a8f21c5
```

Failure logs contain comprehensive diagnostic information:
```
2025-06-13 16:22:15,789 - tests.orchestrator - ERROR - [MANY2ONE] FAILED: Pods cannot share data
2025-06-13 16:22:15,790 - tests.orchestrator - INFO - [MANY2ONE] Writer pod mount info: 'fs-12345abc.efs.us-west-2.amazonaws.com:/ on /data type nfs4'
```

## 8. Troubleshooting

### 8.1 Common Issues

1. **PVC Creation Failures**:
   - Check StorageClass configuration
   - Verify EFS filesystem exists and is accessible
   - Check IAM permissions for the EFS CSI driver

2. **Pod Readiness Issues**:
   - Check pod events with `kubectl describe pod <pod-name>`
   - Verify EFS mount permissions
   - Check if readiness probe is succeeding

3. **Read/Write Test Failures**:
   - Check for network connectivity issues
   - Verify mount points inside pods
   - Check EFS access point settings

### 8.2 Diagnostic Tools

The framework includes built-in diagnostic tools:

1. **Log Collection**: Automatically collects relevant logs on failures
2. **Mount Information**: Gathers mount details from pods for analysis
3. **Resource State**: Collects descriptive information about Kubernetes resources

## 9. Advanced Topics

### 9.1 Modifying Test Scenarios

To add or modify test scenarios:

1. Create a new method in `EFSCSIOrchestrator` following the pattern of existing scenarios:
```python
def _scenario_my_new_test(self):
    """
    Test description here
    """
    self.logger.info("Running scenario: My New Test")
    self.scenarios['my_new_test']['runs'] += 1
    
    try:
        # Implementation here
        if success:
            self.scenarios['my_new_test']['success'] += 1
        else:
            self.scenarios['my_new_test']['fail'] += 1
    except Exception as e:
        self.logger.error(f"Scenario failed: {e}")
        self.scenarios['my_new_test']['fail'] += 1
```

2. Add the scenario to the list in `_run_specific_scenario`:
```python
def _run_specific_scenario(self):
    """Run a specific test scenario"""
    scenarios = [
        self._scenario_many_to_one,
        self._scenario_one_to_one,
        self._scenario_concurrent_pvc,
        self._scenario_my_new_test  # Add your new scenario here
    ]
    scenario = random.choice(scenarios)
    self.logger.info(f"Running specific scenario: {scenario.__name__}")
    scenario()
```

3. Update scenario tracking in `__init__`:
```python
self.scenarios = {
    'shared_volume_rw': {'runs': 0, 'success': 0, 'fail': 0},
    'many_to_one': {'runs': 0, 'success': 0, 'fail': 0},
    'one_to_one': {'runs': 0, 'success': 0, 'fail': 0},
    'concurrent_pvc': {'runs': 0, 'success': 0, 'fail': 0},
    'my_new_test': {'runs': 0, 'success': 0, 'fail': 0}  # Add tracking for your new scenario
}
```

4. Update scenario configuration in the YAML file:
```yaml
scenarios:
  # ... existing scenarios
  my_new_test:
    enabled: true
    custom_param1: value1
    custom_param2: value2
```

5. Update weights in configuration if needed.

### 9.2 Custom Metrics

To add custom metrics:

1. Extend the `MetricsCollector` class in `utils/metrics_collector.py`:
```python
def track_my_custom_metric(self, pvc_name, value):
    """Track a custom metric
    
    Args:
        pvc_name: Name of PVC
        value: Metric value
    """
    if 'custom_metrics' not in self.metrics:
        self.metrics['custom_metrics'] = {}
    
    if pvc_name not in self.metrics['custom_metrics']:
        self.metrics['custom_metrics'][pvc_name] = []
    
    self.metrics['custom_metrics'][pvc_name].append(value)
```

2. Update the Prometheus exporter in `monitoring/prometheus_exporter.py` to expose new metrics:
```python
# Add a new Prometheus metric
my_custom_metric = Gauge(
    'efs_csi_custom_metric', 
    'Description of your custom metric',
    ['pvc_name']
)

# Update it in process_metrics function
def process_metrics(metrics_data):
    # Process existing metrics
    # ...
    
    # Process custom metrics
    if 'custom_metrics' in metrics_data:
        for pvc_name, values in metrics_data['custom_metrics'].items():
            if values:
                my_custom_metric.labels(pvc_name=pvc_name).set(sum(values) / len(values))
```

3. Update dashboards to visualize the new metrics:
   - Edit relevant JSON files in `monitoring/dashboards/`
   - Add new panels that query your custom metrics

### 9.3 Failure Analysis

For detailed failure analysis:

1. Examine logs in `logs/efs_orchestrator_failure_*` directories:
   - Review `failed_resources` folder for resource state at failure time
   - Check pod and PVC events
   - Look for error patterns in driver logs

2. Check collected metrics in `reports/orchestrator/*_metrics.json`:
   - Look for performance anomalies before failures
   - Check for patterns in failed operations

3. Review pod and PVC events collected during failures:
   - `kubectl describe` output saved during failures
   - Container logs and events

4. Create targeted tests for specific failure cases:
   - Modify configuration for specific scenarios
   - Reduce random factors to focus on problem areas

### 9.4 Code Modularization Principles

The framework follows these modularization principles:

1. **Single Responsibility**: Each function has a clear and focused purpose
2. **Function Size**: Functions are kept small (under 50 lines) for better maintainability
3. **Configuration Over Code**: Use configuration values instead of hardcoded constants
4. **Descriptive Naming**: Function and variable names clearly communicate their purpose
5. **Helper Methods**: Complex operations are broken down into helper methods
6. **Error Handling**: Errors are caught at appropriate levels and properly logged

Example of modularization:

```python
# Instead of a single large function:
def build_complex_widget(config, name):
    # 100+ lines of code doing many things

# Break it down into smaller functions:
def build_complex_widget(config, name):
    """Orchestrate the widget building process"""
    base = create_widget_base(name)
    add_widget_components(base, config)
    configure_widget_behavior(base, config)
    return base

def create_widget_base(name):
    """Create the foundation of the widget"""
    # 10-15 lines focused on just this task

def add_widget_components(widget, config):
    """Add all necessary components to the widget"""
    # 15-20 lines focused on component addition

def configure_widget_behavior(widget, config):
    """Set up the widget's behavior based on config"""
    # 15-20 lines focused on behavior configuration
```

## 10. Component Relationships

```
┌─────────────────┐     ┌───────────────────┐     ┌────────────────┐
│   run_tests.py  │────▶│ EFSCSIOrchestrator│────▶│Test Operations │
└─────────────────┘     └───────────────────┘     └────────────────┘
        │                        │                        │
        ▼                        ▼                        ▼
┌─────────────────┐     ┌───────────────────┐     ┌────────────────┐
│ReportGenerator  │◀────│ MetricsCollector  │◀────│Test Scenarios  │
└─────────────────┘     └───────────────────┘     └────────────────┘
        │                        ▲                        │
        │                        │                        │
        ▼                        │                        ▼
┌─────────────────────────┐      │      ┌────────────────────────┐
│      Reports &          │      │      │   K8s Resources        │
│      Dashboards         │      │      │  (PVCs, Pods, etc.)    │
└─────────────────────────┘      │      └────────────────────────┘
                                 │
                        ┌────────────────┐
                        │  Monitoring    │
                        │  Stack         │
                        └────────────────┘
```

### 10.1 Data Flow

1. `run_tests.py` initializes components and kicks off testing
2. `EFSCSIOrchestrator` executes operations and scenarios
3. Test operations and scenarios interact with Kubernetes resources
4. `MetricsCollector` gathers metrics from operations and scenarios
5. `ReportGenerator` creates reports from collected metrics
6. Reports are stored in files and exposed via the monitoring stack
7. Dashboards visualize the metrics for analysis

### 10.2 Control Flow

1. User invokes `run_tests.py` with configuration options
2. `run_tests.py` initializes the orchestrator
3. Orchestrator runs for the specified duration, executing operations
4. Orchestrator tracks resources and metrics during execution
5. After completion, orchestrator generates a report
6. Metrics are available during and after test execution via monitoring


## 12. Conclusion

The AWS EFS CSI Driver Stress & Scale Testing Framework provides a comprehensive, configurable platform for testing the EFS CSI driver under various conditions. Its modular architecture allows for easy extension and customization, while the built-in metrics collection and monitoring provide valuable insights into driver performance and behavior.

The recent modularization efforts have significantly improved code maintainability and configurability, making it easier to extend the framework and adapt it to different testing needs.

By following this documentation, developers should be able to understand the system architecture, run tests, analyze results, and extend the framework for their specific testing needs.
