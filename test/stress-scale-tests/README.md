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

## Requirements

- Python 3.8+
- Kubernetes cluster with EFS CSI Driver installed
- `kubectl` configured to access the cluster

## Quick Start

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

## Configuration Options

The main configuration file is located at `config/orchestrator_config.yaml`. Key parameters include:

- `test.duration`: Test duration in seconds (default: 3600)
- `test.namespace`: Kubernetes namespace for test resources (default: efs-stress-test) 
- `test.operation_interval`: Time between operations in seconds (default: 3)
- `resource_limits`: Controls maximum PVCs and pods to create
- `operation_weights`: Adjust relative frequency of different operations

## Running Tests

Basic test with default parameters:
```
python run_tests.py
```

Run with custom duration (e.g., 2 hours):
```
python run_tests.py --duration 7200
```

Run with custom operation rate (operations per second):
```
python run_tests.py --rate 10
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
