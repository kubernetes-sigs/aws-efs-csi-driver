# AWS EFS CSI Driver Stress and Scalability Tests

This framework provides comprehensive stress and scalability testing for the AWS EFS CSI Driver in Kubernetes environments.

## Overview

The test framework automatically generates load on the EFS CSI Driver by creating and managing PVCs and Pods according to configurable parameters. It tests various access patterns and scenarios to ensure reliability under stress.

## Features

- **Orchestrated Testing**: Random sequence of volume and pod operations with configurable weights
- **Scenario Testing**: Specialized test scenarios including:
  - Shared Volume Access (multiple pods using a single PVC to test ReadWriteMany capability)
  - Dedicated Volume Access (individual pods with dedicated PVCs to test isolation)
  - Concurrent Volume Operations (rapid creation and deletion of multiple PVCs to test API handling)
- **Shared Volume Testing**: Verifies read/write operations between pods sharing volumes
- **Comprehensive Reporting**: Detailed logs and metrics in JSON and summary formats
- **Configurable Parameters**: Adjust test duration, operation rates, resource limits, and more

## Prerequisites

- AWS Account with appropriate permissions for:
  - EFS filesystem creation and management
  - EKS cluster management (if creating a new cluster)
- Kubernetes cluster with:
  - EFS CSI Driver installed (unless using the orchestrator to install it)
  - Node(s) in the same VPC as your EFS filesystem
- `kubectl` configured to access the cluster
- Required Python packages (install via requirements.txt):
  - kubernetes
  - pytest
  - pyyaml
  - prometheus_client
  - pandas
  - psutil
  - boto3

## Quick Start

### Important Configuration Notes

Before running tests, you'll need to configure key settings in `config/orchestrator_config.yaml`. The most important sections are:

1. **Driver Configuration**:
   ```yaml
   driver:
     create_filesystem: true/false  # Set to true to automatically create a new EFS filesystem
     filesystem_id: fs-xxx         # Required if create_filesystem is false (use existing filesystem)
     # Note: If create_filesystem is true, boto3 will be used to create the filesystem
   ```

2. **Storage Class Configuration**:
   ```yaml
   storage_class:
     parameters:
       fileSystemId: fs-xxx       # Must match your filesystem_id
       region: us-west-1          # Your AWS region
       availabilityZoneName: us-west-1b  # AZ where your nodes are running
   ```

3. **Pod Configuration**:
   ```yaml
   pod_config:
     node_selector:
       topology.kubernetes.io/zone: us-west-1b  # Must match your node's AZ
   ```

### Getting Started

1. Set up a Python virtual environment (recommended):
   ```
   # Create a virtual environment
   python -m venv venv
   
   # Activate the virtual environment
   # On Linux/macOS:
   source venv/bin/activate
   # On Windows:
   # venv\Scripts\activate
   ```

2. Install dependencies:
   ```
   pip install -r requirements.txt
   ```

3. Configure the test parameters in `config/orchestrator_config.yaml`

4. Run the tests:
   ```
   python run_tests.py
   ```

## Configuration Structure

The configuration is modularized into separate components for better organization and clarity:

1. `config/orchestrator_config.yaml`: Main configuration file that imports component configurations
2. Component configurations in `config/components/`:
   - `driver.yaml`: Driver installation and resource settings
   - `storage.yaml`: Storage class configuration
   - `test.yaml`: Test parameters, metrics, and reporting settings
   - `pod.yaml`: Pod configuration settings
   - `scenarios.yaml`: Test scenario definitions

Each component file is well-documented with comments explaining available options. The modular structure allows you to:
- Focus on specific configuration aspects independently
- Easily understand which settings are related
- Comment out unused sections without affecting other components
- Override specific settings in the main config file if needed

### Key Configuration Parameters

Most commonly adjusted settings:

1. In `driver.yaml`:
   - `driver.create_filesystem`: Whether to create a new EFS filesystem
   - `driver.filesystem_id`: Your EFS filesystem ID

2. In `storage.yaml`:
   - `storage_class.parameters.fileSystemId`: Must match your filesystem_id
   - `storage_class.parameters.region`: Your AWS region
   - `storage_class.parameters.availabilityZoneName`: Your AZ

3. In `test.yaml`:
   - `test.duration`: Test duration in seconds
   - `test.namespace`: Kubernetes namespace for test resources
   - `test.operation_interval`: Time between operations

4. In `pod.yaml`:
   - `pod_config.node_selector`: Must match your node's availability zone

5. In `scenarios.yaml`:
   - Enable/disable specific test scenarios as needed
   - Adjust scenario parameters like pod counts and PVC limits

## Running Tests

Basic test with default parameters:
```
python run_tests.py
```

Run with custom duration (e.g., 2 hours):
```
python run_tests.py --duration 7200
```

Run with custom interval (seconds between operations):
```
python run_tests.py --interval 10
```

## Cleanup

To clean up resources created by tests:
```
python cleanup_test_resources.py
```

## Reports

Test reports are stored in:
- `reports/orchestrator/`: Orchestrator test reports (JSON)
- `reports/general/`: General test summary reports

## Architecture

- `tests/orchestrator.py`: Main test orchestration engine
- `utils/metrics_collector.py`: Collects performance metrics
- `utils/report_generator.py`: Generates test reports
- `run_tests.py`: CLI for running tests
- `cleanup_test_resources.py`: Utility for cleaning up test resources

## Notes

- The tests use `kubectl` subprocess calls for pod exec operations to avoid WebSocket protocol issues
- All tests run in the namespace specified in the config (default: `efs-stress-test`)
# AWS EFS CSI Driver Testing Framework
