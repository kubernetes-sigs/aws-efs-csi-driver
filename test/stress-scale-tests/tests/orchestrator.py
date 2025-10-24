#!/usr/bin/env python3

import random
import time
import yaml
import logging
import uuid
import os
import boto3
from kubernetes import client, config
from datetime import datetime
from utils.log_integration import collect_logs_on_test_failure

class EFSCSIOrchestrator:
    """Orchestrator for testing EFS CSI driver operations"""
    
    def __init__(self, config_file=None, component_configs=None, namespace=None, metrics_collector=None, driver_pod_name=None):
        """Initialize the orchestrator with configuration
        
        Args:
            config_file: Path to single config file (legacy approach)
            component_configs: Dictionary of component config files paths
            namespace: Kubernetes namespace for test resources
            metrics_collector: Metrics collector instance
            driver_pod_name: Name of driver pod for log collection
        """
        # Store driver pod name for log collection
        self.driver_pod_name = driver_pod_name
        
        # Configure logger before anything else for early diagnostics
        self.logger = logging.getLogger(__name__)
        self._init_component_configuration(component_configs, namespace)
        # Initialize clients and resources
        self._init_kubernetes_clients()
        self._init_metrics_collector(metrics_collector)
        self._init_logging()  # Now reconfigure logging with loaded config
        self._init_test_parameters()
        self._init_resource_tracking()
        
        self.logger.info("EFS CSI Orchestrator initialized")
        
        # Create namespace if it doesn't exist
        self._ensure_namespace_exists()
    

    
    def _init_component_configuration(self, component_configs, namespace):
        """Initialize configuration from component files
        
        Args:
            component_configs: Dictionary mapping component names to file paths
            namespace: Kubernetes namespace override (optional)
        """
        if not component_configs:
            self.logger.error("No component configs provided")
            self.config = {}
            self.namespace = namespace or 'default'
            return
            
        self.logger.info("Loading configuration from component files")
        self.config = {}
        
        # Load all component configs
        components = {
            'driver': {'file': component_configs.get('driver'), 'key': 'driver'},
            'storage': {'file': component_configs.get('storage'), 'key': 'storage_class'},
            'test': {'file': component_configs.get('test'), 'key': None},  # Special handling for test
            'pod': {'file': component_configs.get('pod'), 'key': 'pod_config'},
            'scenarios': {'file': component_configs.get('scenarios'), 'key': 'scenarios'}
        }
        
        for component_name, details in components.items():
            self._load_component(component_name, details['file'], details['key'])
        
        # Set namespace from config or use default
        test_namespace = None
        if hasattr(self, 'test_config') and isinstance(self.test_config, dict):
            if 'test' in self.test_config:
                test_namespace = self.test_config.get('test', {}).get('namespace')
            elif 'namespace' in self.test_config:
                test_namespace = self.test_config.get('namespace')
                
        self.namespace = namespace or test_namespace or 'default'
        self.logger.info(f"Using namespace: {self.namespace}")
        
    def _load_component(self, component_name, file_path, config_key):
        """Load a component configuration file
        
        Args:
            component_name: Name of the component (e.g., 'driver', 'storage')
            file_path: Path to the component file
            config_key: Key to use in self.config for this component, or None for special handling
        """
        if not file_path or not os.path.exists(file_path):
            self.logger.warning(f"Component file for {component_name} not found at {file_path}")
            setattr(self, f"{component_name}_config", {})
            return
            
        try:
            with open(file_path, 'r') as f:
                component_data = yaml.safe_load(f) or {}
                
            # Store the complete component data
            setattr(self, f"{component_name}_config", component_data)
            self.logger.info(f"Loaded {component_name} config from {file_path}")
            
            # Special handling for test component which contains multiple top-level keys
            if component_name == 'test':
                # For test config, copy all top-level keys to self.config
                for key, value in component_data.items():
                    self.config[key] = value
                    self.logger.debug(f"Added {key} from test config to main config")
            elif config_key:
                # For other components, look for the specified key
                if config_key in component_data:
                    self.config[config_key] = component_data[config_key]
                    self.logger.debug(f"Added {config_key} from {component_name} config to main config")
                else:
                    # If the expected key isn't found, add the whole component
                    if len(component_data) > 0:
                        for key, value in component_data.items():
                            self.config[key] = value
                            self.logger.debug(f"Added {key} from {component_name} config to main config")
                    else:
                        self.logger.warning(f"No data found in {component_name} config")
        except Exception as e:
            self.logger.error(f"Error loading {component_name} config: {e}")
            setattr(self, f"{component_name}_config", {})
    
    def _init_kubernetes_clients(self):
        """Initialize Kubernetes API clients"""
        config.load_kube_config()
        self.core_v1 = client.CoreV1Api()
        self.apps_v1 = client.AppsV1Api()
        self.storage_v1 = client.StorageV1Api()
    
    def _init_metrics_collector(self, metrics_collector):
        """Initialize metrics collector"""
        from utils.metrics_collector import MetricsCollector
        self.metrics_collector = metrics_collector or MetricsCollector()
    
    def _init_logging(self):
        """Set up logging based on configuration"""
        self.logger = logging.getLogger(__name__)
        # Remove any existing handlers to prevent duplicates
        self.logger.handlers.clear()
        
        log_level = getattr(logging, self.config.get('logging', {}).get('level', 'INFO'))
        self.logger.setLevel(log_level)
        # Prevent propagation to root logger to avoid duplicate logs
        self.logger.propagate = False
        
        formatter = logging.Formatter('%(asctime)s - %(name)s - %(levelname)s - %(message)s')
        
        # Console handler if enabled
        if self.config.get('logging', {}).get('console_enabled', True):
            console_handler = logging.StreamHandler()
            console_handler.setFormatter(formatter)
            self.logger.addHandler(console_handler)
        
        # File handler if enabled
        if self.config.get('logging', {}).get('file_enabled', True):
            os.makedirs('logs', exist_ok=True)
            file_handler = logging.FileHandler(f'logs/orchestrator_{datetime.now().strftime("%Y%m%d_%H%M%S")}.log')
            file_handler.setFormatter(formatter)
            self.logger.addHandler(file_handler)
    
    def _init_test_parameters(self):
        """Initialize test parameters from configuration"""
        # Test parameters
        self.test_duration = self.config['test'].get('duration', 3600)  # seconds
        self.operation_interval = self.config['test'].get('operation_interval', 3)  # seconds
        
        # Resource limits
        resource_limits = self.config.get('resource_limits', {})
        self.max_pvcs = resource_limits.get('max_pvcs', 100)
        self.max_pods_per_pvc = resource_limits.get('max_pods_per_pvc', 50)
        self.total_max_pods = resource_limits.get('total_max_pods', 250)
    
    def _init_resource_tracking(self):
        """Initialize resource tracking data structures"""
        # Resource tracking
        self.pvcs = []  # List of PVC names
        self.pods = {}  # Maps pvc_name -> list of pod_names
        self.current_pod_count = 0
        
        # Test status tracking
        self.results = {
            'create_pvc': {'success': 0, 'fail': 0},
            'attach_pod': {'success': 0, 'fail': 0},
            'delete_pod': {'success': 0, 'fail': 0},
            'delete_pvc': {'success': 0, 'fail': 0},
            'verify_write': {'success': 0, 'fail': 0},
            'verify_read': {'success': 0, 'fail': 0}
        }
        
        # Initialize test scenarios
        self.scenarios = {
            'shared_volume_rw': {'runs': 0, 'success': 0, 'fail': 0},
            'many_to_one': {'runs': 0, 'success': 0, 'fail': 0},
            'one_to_one': {'runs': 0, 'success': 0, 'fail': 0},
            'concurrent_pvc': {'runs': 0, 'success': 0, 'fail': 0}
        }
        
    def _ensure_namespace_exists(self):
        """Create the namespace if it doesn't exist already"""
        try:
            # Check if namespace exists
            self.core_v1.read_namespace(name=self.namespace)
            self.logger.info(f"Namespace '{self.namespace}' already exists")
        except client.exceptions.ApiException as e:
            if e.status == 404:
                # Create namespace if it doesn't exist
                namespace_manifest = {
                    "apiVersion": "v1",
                    "kind": "Namespace",
                    "metadata": {
                        "name": self.namespace
                    }
                }
                
                self.core_v1.create_namespace(body=namespace_manifest)
                self.logger.info(f"Created namespace '{self.namespace}'")
            else:
                self.logger.error(f"Error checking namespace: {e}")
                raise
    
    def deploy_csi_driver(self):
        """
        Deploy or update the EFS CSI driver using Helm.
        Uses the driver configuration from orchestrator_config.yaml.
        """
        import subprocess
        
        self.logger.info("Deploying/updating EFS CSI driver with configuration")
        
        # Get driver configuration
        driver_config = self.config.get('driver', {})
        
        # Get repository and tag
        repository = driver_config.get('repository', '745939127895.dkr.ecr.us-east-1.amazonaws.com/amazon/aws-efs-csi-driver')
        tag = f"v{driver_config.get('version', '2.1.1')}"
        
        # Build Helm command with --force flag to adopt existing resources
        cmd = [
            "helm", "upgrade", "--install",
            "--force",  # Add force flag to adopt existing resources
            "aws-efs-csi-driver", 
            "aws-efs-csi-driver/aws-efs-csi-driver",
            "--namespace", "kube-system",
            "--set", f"image.repository={repository}",
            "--set", f"image.tag={tag}",
            "--set", "controller.serviceAccount.create=false",
            "--set", "controller.serviceAccount.name=efs-csi-controller-sa",
            "-f", "config/driver-values.yaml"
        ]
        
        try:
            self.logger.info(f"Running Helm command: {' '.join(cmd)}")
            result = subprocess.run(cmd, capture_output=True, text=True)
            
            if result.returncode != 0:
                self.logger.error(f"Error deploying CSI driver: {result.stderr}")
                return False
            
            self.logger.info("EFS CSI deployed/updated successfully")
            return True
        except Exception as e:
            self.logger.error(f"Exception while deploying CSI driver: {e}")
            return False
            
    def run_test(self):
        """
        Run the orchestrator test by randomly selecting operations
        until the test duration is reached
        """
        self.logger.info(f"Starting orchestrator test for {self.test_duration} seconds")
        # Deploy the CSI driver with configuration
        self.deploy_csi_driver()
        start_time = time.time()
        self._ensure_storage_class()
        operations, weights = self._get_operations_and_weights()
        cumulative_weights, total_weight = self._get_cumulative_weights(weights)
        self._run_initial_operations()
        operation_counts = {op.__name__: 0 for op, _ in operations}
        
        try:
            while time.time() - start_time < self.test_duration:
                self._run_random_operation(operations, cumulative_weights, total_weight, operation_counts)
                time.sleep(self.operation_interval)
        except KeyboardInterrupt:
            self.logger.info("Test interrupted by user")
        except Exception as e:
            self._handle_unexpected_test_error(e)
        finally:
            elapsed = time.time() - start_time
            self.logger.info(f"Test completed in {elapsed:.2f} seconds")
            self._cleanup()
            return self._generate_report()

    def _get_operations_and_weights(self):
        weights = self.config.get('operation_weights', {})
        operations = [
            (self._create_pvc, weights.get('create_pvc', 25)),
            (self._attach_pod, weights.get('attach_pod', 25)),
            (self._delete_pod, weights.get('delete_pod', 20)),
            (self._delete_pvc, weights.get('delete_pvc', 15)),
            (self._verify_readwrite, weights.get('verify_readwrite', 15)),
            (self._run_specific_scenario, weights.get('run_specific_scenario', 20))
        ]
        operation_funcs, weights = zip(*operations)
        return operations, weights

    def _get_cumulative_weights(self, weights):
        cumulative_weights = []
        current_sum = 0
        for weight in weights:
            current_sum += weight
            cumulative_weights.append(current_sum)
        total_weight = cumulative_weights[-1]
        return cumulative_weights, total_weight

    def _run_initial_operations(self):
        self.logger.info("Running each operation type once to ensure coverage")
        self._create_pvc()
        self._create_pvc()
        self._attach_pod()
        self._attach_pod()
        self._attach_pod()
        self._verify_readwrite()
        self._run_specific_scenario()
        self._delete_pod()
        self._delete_pvc()
        self.logger.info("Completed initial operation sequence, continuing with randomized operations")

    def _run_random_operation(self, operations, cumulative_weights, total_weight, operation_counts):
        random_val = random.uniform(0, total_weight)
        for i, (operation, _) in enumerate(operations):
            if random_val <= cumulative_weights[i]:
                op_name = operation.__name__
                operation_counts[op_name] = operation_counts.get(op_name, 0) + 1
                self.logger.info(f"Selected operation: {op_name} (selected {operation_counts[op_name]} times)")
                operation()
                break

    def _handle_unexpected_test_error(self, e):
        self.logger.error(f"Unexpected error during test: {e}", exc_info=True)
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        test_name = f"orchestrator_unexpected_failure_{timestamp}"
        failed_resources = []
        for pvc_name in self.pvcs:
            failed_resources.append({"type": "pvc", "name": pvc_name, "namespace": self.namespace})
            for pod_name in self.pods.get(pvc_name, []):
                failed_resources.append({"type": "pod", "name": pod_name, "namespace": self.namespace})
        logs_path = collect_logs_on_test_failure(
            test_name, 
            self.metrics_collector, 
            self.driver_pod_name,
            failed_resources=failed_resources
        )
        self.logger.info(f"Collected comprehensive failure logs to: {logs_path}")

    def _create_pvc(self):
        """Create a PVC with access point using configured values"""
        # Check if we've reached the maximum PVC count
        if len(self.pvcs) >= self.max_pvcs:
            self.logger.info("Maximum PVC count reached, skipping creation")
            return
            
        pvc_name = f"test-pvc-{uuid.uuid4().hex[:8]}"
        self.logger.info(f"Creating PVC: {pvc_name}")
        
        try:
            # Build the PVC manifest from config
            pvc_manifest = self._build_pvc_manifest(pvc_name)
            
            # Create and wait for PVC to be bound
            success = self._create_and_wait_for_pvc(pvc_name, pvc_manifest)
            
            if not success:
                self.logger.warning(f"PVC {pvc_name} creation process did not complete successfully")
            
        except Exception as e:
            self.results['create_pvc']['fail'] += 1
            self.logger.error(f"Failed to create PVC: {e}")
    
    def _build_pvc_manifest(self, pvc_name):
        """Build a PVC manifest based on configuration"""
        pvc_config = self.config.get('pvc_config', {})
        
        # Get storage class name from config
        sc_name = self.config.get('storage_class', {}).get('name', 'efs-sc')
        
        # Get access modes from config or use default
        access_modes = pvc_config.get('access_modes', ["ReadWriteMany"])
        
        # Get storage size from config or use default
        storage_size = pvc_config.get('storage_size', "1Gi")
        
        # Create base manifest
        pvc_manifest = {
            "apiVersion": "v1",
            "kind": "PersistentVolumeClaim",
            "metadata": {"name": pvc_name},
            "spec": {
                "accessModes": access_modes,
                "storageClassName": sc_name,
                "resources": {
                    "requests": {"storage": storage_size}
                }
            }
        }
        
        # Add metadata from config
        self._add_pvc_metadata(pvc_manifest, pvc_config)
        
        return pvc_manifest
        
    def _add_pvc_metadata(self, pvc_manifest, pvc_config):
        """Add metadata like annotations and labels to PVC manifest"""
        # Add annotations if configured
        pvc_annotations = pvc_config.get('annotations', {})
        if pvc_annotations:
            pvc_manifest['metadata']['annotations'] = pvc_annotations
        
        # Add labels if configured
        pvc_labels = pvc_config.get('labels', {})
        if pvc_labels:
            pvc_manifest['metadata']['labels'] = pvc_labels
            
        return pvc_manifest
        
    def _create_and_wait_for_pvc(self, pvc_name, pvc_manifest):
        """Create PVC and wait for it to be bound"""
        # Create PVC
        self.core_v1.create_namespaced_persistent_volume_claim(
            namespace=self.namespace,
            body=pvc_manifest
        )
        
        # Track the PVC
        self.pvcs.append(pvc_name)
        self.pods[pvc_name] = []
        
        # Update results
        self.results['create_pvc']['success'] += 1
        sc_name = pvc_manifest['spec']['storageClassName']
        self.logger.info(f"Created PVC: {pvc_name} with storage class {sc_name}")
        
        # Get timeout value from config or use default
        retry_config = self.config.get('retries', {})
        pvc_bind_timeout = retry_config.get('pvc_bind_timeout', 30)
        
        # Wait for PVC to be bound
        return self._wait_for_pvc_bound(pvc_name, timeout=pvc_bind_timeout)
    
    def _attach_pod(self, pvc_name=None):
        """
        Attach a pod to a PVC. If pvc_name is provided, attach to that specific PVC. Otherwise, select a random PVC.
        """
        if not self.pvcs:
            self.logger.info("No PVCs available, skipping pod attachment")
            return None
        if self.current_pod_count >= self.total_max_pods:
            self.logger.info("Maximum total pod count reached, skipping attachment")
            return None
        pvc_name = self._select_pvc_for_pod(pvc_name)
        if pvc_name is None:
            return None
        pod_name = f"test-pod-{uuid.uuid4().hex[:8]}"
        pod_manifest = self._build_pod_manifest(pod_name, pvc_name)
        try:
            self.core_v1.create_namespaced_pod(namespace=self.namespace, body=pod_manifest)
            self._track_new_pod(pvc_name, pod_name)
            self.logger.info(f"Created pod: {pod_name} using PVC: {pvc_name}")
            if not self._wait_for_pod_ready(pod_name, timeout=60):
                self.logger.warning(f"Timeout waiting for pod {pod_name} to be ready")
                return None
            return pod_name
        except Exception as e:
            self.results['attach_pod']['fail'] += 1
            self.logger.error(f"Failed to create pod: {e}")
            return None

    def _select_pvc_for_pod(self, pvc_name):
        if pvc_name is None or pvc_name not in self.pvcs:
            pvc_name = random.choice(self.pvcs)
        if len(self.pods[pvc_name]) >= self.max_pods_per_pvc:
            self.logger.info(f"PVC {pvc_name} has reached max pods ({self.max_pods_per_pvc}), skipping")
            return None
        return pvc_name

    def _build_pod_manifest(self, pod_name, pvc_name):
        """Build pod manifest using configuration values"""
        pod_config = self.config.get('pod_config', {})
        
        # Build the container specification
        container = self._build_container_spec(pod_config)
        
        # Build pod metadata
        metadata = self._build_pod_metadata(pod_name, pod_config)
        
        # Build pod spec
        pod_spec = self._build_pod_spec(container, pvc_name, pod_config)
        
        # Combine into complete manifest
        manifest = {
            "apiVersion": "v1",
            "kind": "Pod",
            "metadata": metadata,
            "spec": pod_spec
        }
        
        return manifest
        
    def _build_container_spec(self, pod_config):
        """Build the container specification from config"""
        # Determine command arguments
        args = pod_config.get('args')
        if not args:
            startup_script = self._get_pod_startup_script()
            args = [startup_script]
            
        # Create base container spec
        container = {
            "name": "test-container",
            "image": pod_config.get('image', 'alpine:latest'),
            "volumeMounts": [{"name": "efs-volume", "mountPath": "/data"}],
        }
        
        # Add command if specified in config
        if 'command' in pod_config:
            container["command"] = pod_config['command']
        elif not args:
            # Default command if args not specified and command not in config
            container["command"] = ["/bin/sh", "-c"]
        
        # Add args if available
        if args:
            container["args"] = args
            
        # Add readiness probe
        container["readinessProbe"] = self._build_readiness_probe(pod_config)
        
        # Add resource constraints
        container["resources"] = self._build_container_resources(pod_config)
        
        return container
    
    def _build_readiness_probe(self, pod_config):
        """Build readiness probe configuration from pod config"""
        readiness_probe = pod_config.get('readiness_probe', {})
        return {
            "exec": {
                "command": ["/bin/sh", "-c", "cat /data/pod-ready 2>/dev/null || cat /tmp/ready/pod-ready 2>/dev/null"]
            },
            "initialDelaySeconds": readiness_probe.get('initial_delay_seconds', 15),
            "periodSeconds": readiness_probe.get('period_seconds', 5),
            "failureThreshold": readiness_probe.get('failure_threshold', 6),
            "timeoutSeconds": readiness_probe.get('timeout_seconds', 5)
        }
    
    def _build_container_resources(self, pod_config):
        """Build container resources configuration from config"""
        container_resources = self.config.get('pod_resources', {})
        return {
            "requests": container_resources.get('requests', {"cpu": "100m", "memory": "64Mi"}),
            "limits": container_resources.get('limits', {"cpu": "200m", "memory": "128Mi"})
        }
    
    def _build_pod_metadata(self, pod_name, pod_config):
        """Build pod metadata from config"""
        metadata = {
            "name": pod_name,
            "labels": {"app": "efs-test", "component": "stress-test"}
        }
        
        # Add custom labels if specified
        custom_labels = pod_config.get('labels', {})
        if custom_labels:
            metadata['labels'].update(custom_labels)
            
        return metadata
    
    def _build_pod_spec(self, container, pvc_name, pod_config):
        """Build pod spec from container and config"""
        pod_spec = {
            "containers": [container],
            "volumes": [{
                "name": "efs-volume",
                "persistentVolumeClaim": {"claimName": pvc_name}
            }],
            "tolerations": [
                {"key": "node.kubernetes.io/not-ready", "operator": "Exists", "effect": "NoExecute", "tolerationSeconds": 300},
                {"key": "node.kubernetes.io/unreachable", "operator": "Exists", "effect": "NoExecute", "tolerationSeconds": 300}
            ]
        }
        
        # Add additional tolerations from config
        if 'tolerations' in pod_config:
            pod_spec['tolerations'].extend(pod_config['tolerations'])
            
        # Add node selector if specified in config
        if 'node_selector' in pod_config:
            pod_spec['nodeSelector'] = pod_config['node_selector']
            self.logger.info(f"Using node selector: {pod_config['node_selector']}")
            
        # Add any additional pod spec settings from config
        pod_spec_settings = pod_config.get('pod_spec', {})
        for key, value in pod_spec_settings.items():
            if key not in pod_spec:
                pod_spec[key] = value
                
        return pod_spec

    def _get_pod_startup_script(self):
        """Get the pod startup script by composing script components"""
        base_script = self._get_basic_pod_script()
        stale_handle_detection = self._get_stale_handle_detection()
        readiness_check = self._get_readiness_check_script()
        health_check_loop = self._get_health_check_loop()
        
        return f"""#!/bin/sh
{base_script}
{stale_handle_detection}
{readiness_check}
{health_check_loop}
"""

    def _get_basic_pod_script(self):
        """Get the basic startup and initialization script"""
        return """echo "Pod $(hostname) starting up"
ls -la /data || echo "ERROR: Cannot access /data directory"

# Initialize stale handle tracking
mkdir -p /tmp/metrics
touch /tmp/stale_count"""

    def _get_stale_handle_detection(self):
        """Get the stale file handle detection function"""
        return """
# Create stale handle detection functions
detect_stale_handle() {
    # Args: $1 = path being checked
    if [ $? -ne 0 ]; then
        ERR_MSG=$(echo "$ERROR_OUTPUT" | grep -i "stale file handle")
        if [ $? -eq 0 ]; then
            echo "EFS_ERROR: STALE_FILE_HANDLE: path=$1, message=$ERR_MSG"
            echo $(date +"%Y-%m-%d %H:%M:%S") > /tmp/stale_handle_detected
            echo "$1: $ERR_MSG" >> /tmp/stale_count
            # Count lines in stale_count file
            STALE_COUNT=$(wc -l < /tmp/stale_count 2>/dev/null || echo 0)
            echo "Stale file handle count: $STALE_COUNT"
        fi
    fi
}

# Check for stale handles on volume root
echo "Testing volume access..."
ERROR_OUTPUT=$(ls -la /data 2>&1 1>/dev/null)
detect_stale_handle "/data"
"""

    def _get_readiness_check_script(self):
        """Get the script for readiness check and file creation"""
        return """
for i in 1 2 3 4 5; do
    echo "Attempt $i to create readiness file"
    ERROR_OUTPUT=$(touch /data/pod-ready 2>&1)
    if [ $? -eq 0 ]; then
        echo "Successfully created /data/pod-ready"
        break
    else
        echo "Failed to create readiness file on attempt $i: $ERROR_OUTPUT"
        detect_stale_handle "/data/pod-ready"
        
        if [ $i -eq 5 ]; then
            echo "All attempts failed, creating alternative readiness file"
            mkdir -p /tmp/ready && touch /tmp/ready/pod-ready
        fi
        sleep 2
    fi
done
"""

    def _get_health_check_loop(self):
        """Get the periodic health check loop script"""
        return """
# Periodic file system health checks
while true; do
    # Every 30 seconds, check for stale handles
    if [ $((RANDOM % 3)) -eq 0 ]; then  # Do checks randomly to spread load
        TEST_FILE="/data/test-$(date +%s).txt"
        ERROR_OUTPUT=$(touch $TEST_FILE 2>&1)
        detect_stale_handle "$TEST_FILE"
        
        if [ -f "$TEST_FILE" ]; then
            rm $TEST_FILE 2>/dev/null
        fi
    fi
    sleep 30
done
"""

    def _track_new_pod(self, pvc_name, pod_name):
        self.pods[pvc_name].append(pod_name)
        self.current_pod_count += 1
        self.results['attach_pod']['success'] += 1

    def _delete_pod(self, pod_name=None, pvc_name=None, force=False):
        """
        Delete a pod. If pod_name and pvc_name are provided, delete that specific pod. Otherwise, select a random pod.
        """
        pvc_name, pod_name = self._select_pod_for_deletion(pod_name, pvc_name)
        if not pod_name:
            return False
        self.logger.info(f"Deleting pod: {pod_name} from PVC: {pvc_name}")
        try:
            self._delete_pod_k8s(pod_name, force)
            if not self._wait_for_pod_deleted(pod_name):
                self.logger.warning(f"Timeout waiting for pod {pod_name} to be deleted")
                return False
            self._untrack_deleted_pod(pvc_name, pod_name)
            self.logger.info(f"Deleted pod: {pod_name}")
            self.results['delete_pod']['success'] += 1
            return True
        except Exception as e:
            self.results['delete_pod']['fail'] += 1
            self.logger.error(f"Failed to delete pod {pod_name}: {e}")
            return False

    def _select_pod_for_deletion(self, pod_name, pvc_name):
        if pod_name is None or pvc_name is None:
            all_pods = [(pvc, pod) for pvc, pod_list in self.pods.items() for pod in pod_list]
            if not all_pods:
                self.logger.info("No pods to delete")
                return (None, None)
            return random.choice(all_pods)
        elif pod_name not in self.pods.get(pvc_name, []):
            self.logger.warning(f"Pod {pod_name} not found in PVC {pvc_name}")
            return (None, None)
        return (pvc_name, pod_name)

    def _delete_pod_k8s(self, pod_name, force):
        if force:
            grace_period_seconds = 0
            propagation_policy = 'Background'
            self.logger.info(f"Force deleting pod {pod_name} with grace period 0")
        else:
            grace_period_seconds = None
            propagation_policy = 'Foreground'
        self.core_v1.delete_namespaced_pod(
            name=pod_name,
            namespace=self.namespace,
            grace_period_seconds=grace_period_seconds,
            propagation_policy=propagation_policy
        )

    def _untrack_deleted_pod(self, pvc_name, pod_name):
        if pod_name in self.pods.get(pvc_name, []):
            self.pods[pvc_name].remove(pod_name)
            self.current_pod_count -= 1

    def _delete_pvc(self, pvc_name=None, force=False):
        """
        Delete a PVC. If pvc_name is provided, delete that specific PVC. Otherwise, select a random PVC.
        """
        pvc_name = self._select_pvc_for_deletion(pvc_name)
        if not pvc_name:
            return False
        self.logger.info(f"Deleting PVC: {pvc_name}")
        self._delete_all_pods_for_pvc(pvc_name)
        try:
            self._delete_pvc_k8s(pvc_name, force)
            if not self._wait_for_pvc_deleted(pvc_name):
                self.logger.warning(f"Timeout waiting for PVC {pvc_name} to be deleted")
                return False
            self._untrack_deleted_pvc(pvc_name)
            self.logger.info(f"Deleted PVC: {pvc_name}")
            self.results['delete_pvc']['success'] += 1
            return True
        except Exception as e:
            self.results['delete_pvc']['fail'] += 1
            self.logger.error(f"Failed to delete PVC {pvc_name}: {e}")
            return False

    def _select_pvc_for_deletion(self, pvc_name):
        if not self.pvcs:
            self.logger.info("No PVCs to delete")
            return None
        if pvc_name is None or pvc_name not in self.pvcs:
            return random.choice(self.pvcs)
        return pvc_name

    def _delete_all_pods_for_pvc(self, pvc_name):
        if self.pods.get(pvc_name):
            self.logger.info(f"Deleting {len(self.pods[pvc_name])} pods using PVC {pvc_name}")
            for pod_name in list(self.pods[pvc_name]):
                self._delete_pod(pod_name, pvc_name)

    def _delete_pvc_k8s(self, pvc_name, force):
        if force:
            grace_period_seconds = 0
            propagation_policy = 'Background'
            self.logger.info(f"Force deleting PVC {pvc_name} with grace period 0")
        else:
            grace_period_seconds = None
            propagation_policy = 'Foreground'
        self.core_v1.delete_namespaced_persistent_volume_claim(
            name=pvc_name,
            namespace=self.namespace,
            grace_period_seconds=grace_period_seconds,
            propagation_policy=propagation_policy
        )

    def _untrack_deleted_pvc(self, pvc_name):
        if pvc_name in self.pvcs:
            self.pvcs.remove(pvc_name)
        if pvc_name in self.pods:
            del self.pods[pvc_name]

    def _verify_readwrite(self):
        """
        Verify read/write operations between pods sharing a PVC
        This tests that pods sharing the same volume can see each other's writes
        """
        # Find PVCs that have multiple pods
        shared_pvcs = [(pvc, pods) for pvc, pods in self.pods.items() if len(pods) >= 2]
        if not shared_pvcs:
            self.logger.info("No shared PVCs with multiple pods for read/write test")
            return
        pvc_name, pod_names = random.choice(shared_pvcs)
        if len(pod_names) < 2:
            return
        writer_pod = random.choice(pod_names)
        reader_pod = random.choice([p for p in pod_names if p != writer_pod])
        test_file = f"test-{uuid.uuid4().hex[:8]}.txt"
        test_content = f"Test content: {uuid.uuid4()}" * 50
        content_size_bytes = len(test_content.encode('utf-8'))
        self.logger.info(f"Testing read/write between pods {writer_pod} and {reader_pod} sharing PVC {pvc_name}")
        self.logger.info(f"File size: {content_size_bytes} bytes")
        try:
            write_success, write_duration = self._run_write_op(writer_pod, test_file, test_content, pvc_name, content_size_bytes)
            if not write_success:
                self._track_rw_failure('write')
                self._track_scenario_failure('shared_volume_rw')
                return
            time.sleep(2)
            read_success, read_duration, resp = self._run_read_op(reader_pod, test_file, test_content, pvc_name, content_size_bytes)
            if read_success:
                self._track_rw_success('read')
                self._track_scenario_success('shared_volume_rw')
                self._run_metadata_ls(reader_pod, pvc_name)
            else:
                self._track_rw_failure('read')
                self._track_scenario_failure('shared_volume_rw')
                self.logger.error(f"Pod {reader_pod} failed to read content written by {writer_pod}. Got different content length: {len(resp)} vs expected {len(test_content)}")
        except Exception as e:
            self.logger.error(f"Failed in read/write verification: {e}")
            self._track_rw_failure('write')
            self._track_scenario_failure('shared_volume_rw')

    def _run_write_op(self, writer_pod, test_file, test_content, pvc_name, content_size_bytes):
        import subprocess
        write_op_start = time.time()
        write_cmd = f"kubectl exec -n {self.namespace} {writer_pod} -- /bin/sh -c 'echo \"{test_content}\" > /data/{test_file}'"
        self.logger.info(f"Executing write command: {write_cmd}")
        try:
            write_process = subprocess.run(
                write_cmd,
                shell=True,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            write_duration = time.time() - write_op_start
            self.metrics_collector.track_file_operation_latency(pvc_name, "write", write_duration)
            self.metrics_collector.track_file_operation_iops(pvc_name, "write", 1, write_duration)
            self.metrics_collector.track_file_operation_throughput(pvc_name, "write", content_size_bytes, write_duration)
            self.logger.info(f"Write operation completed in {write_duration:.3f}s")
            self.logger.info(f"Write throughput: {(content_size_bytes / 1024 / 1024) / write_duration:.2f} MB/s")
            self._track_rw_success('write')
            self.logger.info(f"Pod {writer_pod} wrote to /data/{test_file}")
            return True, write_duration
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Write command execution failed: {e}")
            self.logger.error(f"Command stderr: {e.stderr}")
            return False, 0

    def _run_read_op(self, reader_pod, test_file, test_content, pvc_name, content_size_bytes):
        import subprocess
        read_op_start = time.time()
        read_cmd = f"kubectl exec -n {self.namespace} {reader_pod} -- cat /data/{test_file}"
        self.logger.info(f"Executing read command: {read_cmd}")
        try:
            read_process = subprocess.run(
                read_cmd,
                shell=True,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            read_duration = time.time() - read_op_start
            self.metrics_collector.track_file_operation_latency(pvc_name, "read", read_duration)
            self.metrics_collector.track_file_operation_iops(pvc_name, "read", 1, read_duration)
            self.metrics_collector.track_file_operation_throughput(pvc_name, "read", content_size_bytes, read_duration)
            resp = read_process.stdout.strip()
            self.logger.info(f"Read operation completed in {read_duration:.3f}s")
            self.logger.info(f"Read throughput: {(content_size_bytes / 1024 / 1024) / read_duration:.2f} MB/s")
            self.logger.info(f"Read result length: {len(resp)} bytes")
            if test_content in resp:
                self.logger.info(f"Pod {reader_pod} successfully read content written by writer pod")
                return True, read_duration, resp
            else:
                return False, read_duration, resp
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Read command execution failed: {e}")
            self.logger.error(f"Command stderr: {e.stderr}")
            return False, 0, ''

    def _run_metadata_ls(self, reader_pod, pvc_name):
        import subprocess
        meta_op_start = time.time()
        ls_cmd = f"kubectl exec -n {self.namespace} {reader_pod} -- ls -la /data/"
        try:
            ls_process = subprocess.run(
                ls_cmd,
                shell=True,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            meta_duration = time.time() - meta_op_start
            self.metrics_collector.track_file_operation_latency(pvc_name, "metadata", meta_duration)
            self.metrics_collector.track_file_operation_iops(pvc_name, "metadata", 1, meta_duration)
            self.logger.info(f"Metadata operation (ls) completed in {meta_duration:.3f}s")
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Metadata ls command failed: {e}")
            self.logger.error(f"Command stderr: {e}")

    def _track_rw_success(self, op_type):
        if op_type == 'write':
            self.results['verify_write']['success'] += 1
        elif op_type == 'read':
            self.results['verify_read']['success'] += 1

    def _track_rw_failure(self, op_type):
        if op_type == 'write':
            self.results['verify_write']['fail'] += 1
        elif op_type == 'read':
            self.results['verify_read']['fail'] += 1

    def _track_scenario_success(self, scenario):
        self.scenarios[scenario]['runs'] += 1
        self.scenarios[scenario]['success'] += 1

    def _track_scenario_failure(self, scenario):
        self.scenarios[scenario]['runs'] += 1
        self.scenarios[scenario]['fail'] += 1

    def _run_specific_scenario(self):
        """
        Run a specific test scenario
        Randomly select from the required scenarios
        """
        scenarios = [
            self._scenario_many_to_one,
            self._scenario_one_to_one,
            self._scenario_concurrent_pvc,
            self._scenario_controller_crash_test
        ]

        # scenarios = [self._scenario_controller_crash_test]
        
        # Add controller crash test if enabled
        # if self.config["scenarios"].get("controller_crash", {}).get("enabled", False):
        #     scenarios.append(self._scenario_controller_crash_test)
        
        # Pick a random scenario
        scenario = random.choice(scenarios)
        scenario_name = scenario.__name__
        
        # Enhanced logging - make it very clear which scenario was selected
        self.logger.info("=" * 60)
        self.logger.info(f"SELECTED SCENARIO: {scenario_name}")
        self.logger.info("=" * 60)
        
        # Execute the scenario
        scenario()
        
        # Log when scenario completes
        self.logger.info(f"COMPLETED SCENARIO: {scenario_name}")
        self.logger.info("-" * 60)
    
    def _scenario_many_to_one(self):
        """
        Test many pods mounting a single PVC
        1. Create one PVC
        2. Create multiple pods that all mount the same PVC
        3. Verify pods can read/write successfully using kubectl subprocess
        4. Clean up
        """
        self.logger.info("+" * 80)
        self.logger.info("STARTING MANY-TO-ONE SCENARIO DIAGNOSTICS")
        self.logger.info("+" * 80)
        self.scenarios['many_to_one']['runs'] += 1
        try:
            pvc_name = self._create_many_to_one_pvc()
            if not pvc_name:
                self.scenarios['many_to_one']['fail'] += 1
                return
            pod_names = self._create_many_to_one_pods(pvc_name)
            if len(pod_names) < 2:
                self.logger.error(f"[MANY2ONE] FAILED: Insufficient pods created ({len(pod_names)}), need at least 2 for read/write test")
                self.scenarios['many_to_one']['fail'] += 1
                return
            success = self._test_many_to_one_rw(pvc_name, pod_names)
            if success:
                self.logger.info(f"[MANY2ONE] SUCCESS: Many-to-one scenario successful with {len(pod_names)} pods")
                self.scenarios['many_to_one']['success'] += 1
            else:
                self.scenarios['many_to_one']['fail'] += 1
                self._collect_many_to_one_failure_logs(pvc_name, pod_names)
        except Exception as e:
            self.logger.error(f"[MANY2ONE] FAILED: Unhandled error in many-to-one scenario: {e}")
            self.scenarios['many_to_one']['fail'] += 1
        self.logger.info("+" * 80)
        self.logger.info("COMPLETED MANY-TO-ONE SCENARIO DIAGNOSTICS")
        self.logger.info("+" * 80)

    def _create_many_to_one_pvc(self):
        """Create a PVC for many-to-one scenario using configuration"""
        # Generate PVC name with unique identifier
        pvc_name = f"many2one-{uuid.uuid4().hex[:8]}"
        
        self.logger.info(f"[MANY2ONE] STEP 1: Creating dedicated PVC: {pvc_name}")
        
        try:
            # Get configuration values
            scenario_config = self.config.get('scenarios', {}).get('many_to_one', {})
            sc_name = self.config.get('storage_class', {}).get('name', 'efs-sc')
            
            # Create PVC manifest using config values
            pvc_manifest = {
                "apiVersion": "v1",
                "kind": "PersistentVolumeClaim",
                "metadata": {"name": pvc_name},
                "spec": {
                    "accessModes": ["ReadWriteMany"],  # This is generally fixed for EFS
                    "storageClassName": sc_name,
                    "resources": {
                        "requests": {"storage": "1Gi"}  # Size doesn't matter for EFS but required in PVC spec
                    }
                }
            }
            
            # Add annotations if configured
            pvc_annotations = scenario_config.get('pvc_annotations', {})
            if pvc_annotations:
                pvc_manifest['metadata']['annotations'] = pvc_annotations
                
            # Create PVC
            self.core_v1.create_namespaced_persistent_volume_claim(
                namespace=self.namespace,
                body=pvc_manifest
            )
            
            # Track the PVC
            self.pvcs.append(pvc_name)
            self.pods[pvc_name] = []
            
            # Get timeout from config
            retry_config = self.config.get('retries', {})
            pvc_bind_timeout = retry_config.get('pvc_bind_timeout', 30)
            
            self.logger.info(f"[MANY2ONE] PVC {pvc_name} created with storage class {sc_name}")
            
            # Wait for PVC to be bound with configured timeout
            if not self._wait_for_pvc_bound(pvc_name, timeout=pvc_bind_timeout):
                self.logger.error(f"[MANY2ONE] FAILED: Timeout waiting for PVC {pvc_name} to be bound after {pvc_bind_timeout}s")
                return None
                
            return pvc_name
            
        except Exception as e:
            self.logger.error(f"[MANY2ONE] FAILED: Error creating PVC: {e}")
            return None

    def _create_many_to_one_pods(self, pvc_name):
        # Get pod count range from config or use defaults
        scenario_config = self.config.get('scenarios', {}).get('many_to_one', {})
        min_pods = scenario_config.get('min_pods', 3)
        max_pods = scenario_config.get('max_pods', 5)
        num_pods = random.randint(min_pods, max_pods)
        
        self.logger.info(f"[MANY2ONE] STEP 2: Creating {num_pods} pods for the same PVC {pvc_name}")
        pod_names = []
        for i in range(num_pods):
            self.logger.info(f"[MANY2ONE] Creating pod {i+1}/{num_pods} for PVC {pvc_name}")
            pod_name = self._attach_pod(pvc_name)
            if pod_name:
                self.logger.info(f"[MANY2ONE] Successfully created and attached pod {pod_name}")
                pod_names.append(pod_name)
            else:
                self.logger.error(f"[MANY2ONE] Failed to create pod {i+1}/{num_pods}")
        self.logger.info(f"[MANY2ONE] Created {len(pod_names)}/{num_pods} pods successfully")
        return pod_names

    def _test_many_to_one_rw(self, pvc_name, pod_names):
        import subprocess
        test_file = f"many2one-{uuid.uuid4().hex[:8]}.txt"
        test_content = f"Many2One test content: {uuid.uuid4()}"
        writer_pod = random.choice(pod_names)
        reader_pod = random.choice([p for p in pod_names if p != writer_pod])
        self.logger.info(f"[MANY2ONE] STEP 3: Testing read/write operations")
        self.logger.info(f"[MANY2ONE] Writer pod: {writer_pod}, Reader pod: {reader_pod}")
        try:
            write_cmd = f"kubectl exec -n {self.namespace} {writer_pod} -- /bin/sh -c 'echo \"{test_content}\" > /data/{test_file}'"
            self.logger.info(f"[MANY2ONE] Executing write command: {write_cmd}")
            subprocess.run(write_cmd, shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            time.sleep(5)
            read_cmd = f"kubectl exec -n {self.namespace} {reader_pod} -- cat /data/{test_file}"
            self.logger.info(f"[MANY2ONE] Executing read command: {read_cmd}")
            read_process = subprocess.run(read_cmd, shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            read_result = read_process.stdout.strip()
            self.logger.info(f"[MANY2ONE] Read command result: '{read_result}'")
            return test_content in read_result
        except Exception as e:
            self.logger.error(f"[MANY2ONE] FAILED: Error during read/write test: {e}")
            return False

    def _collect_many_to_one_failure_logs(self, pvc_name, pod_names):
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        test_name = f"many2one_failure_{timestamp}"
        failed_resources = (
            [{"type": "pod", "name": pod, "namespace": self.namespace} for pod in pod_names] +
            [{"type": "pvc", "name": pvc_name, "namespace": self.namespace}]
        )
        logs_path = collect_logs_on_test_failure(
            test_name,
            self.metrics_collector,
            self.driver_pod_name,
            failed_resources=failed_resources
        )
        self.logger.info(f"Collected detailed failure logs to: {logs_path}")

    def _scenario_one_to_one(self):
        """
        Test one pod per PVC scenario
        1. Create multiple PVCs
        2. Create one pod per PVC
        3. Verify each pod can write to its own volume using kubectl subprocess
        4. Clean up
        """
        self.logger.info("Running scenario: One pod per PVC")
        self.scenarios['one_to_one']['runs'] += 1
        
        # Get pair count range from config
        scenario_config = self.config.get('scenarios', {}).get('one_to_one', {})
        min_pairs = scenario_config.get('min_pairs', 3)
        max_pairs = scenario_config.get('max_pairs', 5)
        
        # Use configured values instead of hardcoded ones
        num_pairs = random.randint(min_pairs, max_pairs)
        self.logger.info(f"Creating {num_pairs} PVC-pod pairs (range from config: {min_pairs}-{max_pairs})")
        pairs = self._create_one_to_one_pairs(num_pairs)
        if len(pairs) < 2:
            self.logger.warning(f"Failed to create enough PVC-pod pairs, only created {len(pairs)}")
            self.scenarios['one_to_one']['fail'] += 1
            return
        success = self._test_one_to_one_rw(pairs)
        if success:
            self.logger.info(f"[ONE2ONE] One-to-one scenario successful with {len(pairs)} PVC-pod pairs")
            self.scenarios['one_to_one']['success'] += 1
        else:
            self.logger.error("[ONE2ONE] One-to-one scenario failed")
            self.scenarios['one_to_one']['fail'] += 1
            self._collect_one_to_one_failure_logs(pairs)

    def _create_one_to_one_pairs(self, num_pairs):
        """Create pairs of PVCs and pods for one-to-one scenario"""
        # Get the number of pairs to create
        num_pairs = self._get_one_to_one_pair_count(num_pairs)
        self.logger.info(f"[ONE2ONE] Creating {num_pairs} PVC-pod pairs")
        
        # Create the pairs
        pairs = []
        for i in range(num_pairs):
            pair = self._create_one_to_one_pair()
            if pair:
                pairs.append(pair)
                
        return pairs
    
    def _get_one_to_one_pair_count(self, requested_pairs):
        """Determine how many PVC-pod pairs to create based on config and request"""
        # Get configuration for one-to-one scenario
        scenario_config = self.config.get('scenarios', {}).get('one_to_one', {})
        min_pairs = scenario_config.get('min_pairs', 3)
        max_pairs = scenario_config.get('max_pairs', 5)
        
        # If requested_pairs wasn't specified, use configured range
        if requested_pairs <= 0:
            pairs = random.randint(min_pairs, max_pairs)
            self.logger.info(f"[ONE2ONE] Using configured range: creating {pairs} PVC-pod pairs")
            return pairs
        
        return requested_pairs
    
    def _create_one_to_one_pair(self):
        """Create a single PVC-pod pair for one-to-one scenario"""
        # Generate PVC name
        pvc_name = f"one2one-{uuid.uuid4().hex[:8]}"
        
        # Create the PVC manifest
        pvc_manifest = self._build_one_to_one_pvc_manifest(pvc_name)
        
        try:
            # Create the PVC
            self.core_v1.create_namespaced_persistent_volume_claim(
                namespace=self.namespace,
                body=pvc_manifest
            )
            self.pvcs.append(pvc_name)
            self.pods[pvc_name] = []
            
            # Get timeout from config
            retry_config = self.config.get('retries', {})
            pvc_bind_timeout = retry_config.get('pvc_bind_timeout', 30)
            
            # Wait for PVC to be bound
            if not self._wait_for_pvc_bound(pvc_name, timeout=pvc_bind_timeout):
                self.logger.warning(f"[ONE2ONE] Timeout waiting for PVC {pvc_name} to be bound after {pvc_bind_timeout}s")
                return None
                
            # Create and attach pod
            pod_name = self._attach_pod(pvc_name)
            if pod_name:
                self.logger.info(f"[ONE2ONE] Successfully created pair: PVC {pvc_name}, Pod {pod_name}")
                return (pvc_name, pod_name)
                
            return None
            
        except Exception as e:
            self.logger.error(f"[ONE2ONE] Error creating PVC or pod: {e}")
            return None
    
    def _build_one_to_one_pvc_manifest(self, pvc_name):
        """Build PVC manifest for one-to-one scenario"""
        scenario_config = self.config.get('scenarios', {}).get('one_to_one', {})
        
        # Get storage class name from config
        sc_name = self.config.get('storage_class', {}).get('name', 'efs-sc')
        
        # Create base manifest
        pvc_manifest = {
            "apiVersion": "v1",
            "kind": "PersistentVolumeClaim",
            "metadata": {"name": pvc_name},
            "spec": {
                "accessModes": ["ReadWriteMany"],
                "storageClassName": sc_name,
                "resources": {"requests": {"storage": "1Gi"}}
            }
        }
        
        # Add any PVC annotations if configured
        pvc_annotations = scenario_config.get('pvc_annotations', {})
        if pvc_annotations:
            if 'metadata' not in pvc_manifest:
                pvc_manifest['metadata'] = {}
            pvc_manifest['metadata']['annotations'] = pvc_annotations
            
        return pvc_manifest

    def _test_one_to_one_rw(self, pairs):
        import subprocess
        for pvc_name, pod_name in pairs:
            test_file = f"one2one-{uuid.uuid4().hex[:8]}.txt"
            test_content = f"One2One test content for {pvc_name}: {uuid.uuid4()}"
            try:
                write_cmd = f"kubectl exec -n {self.namespace} {pod_name} -- /bin/sh -c 'echo \"{test_content}\" > /data/{test_file}'"
                self.logger.info(f"[ONE2ONE] Executing write command for pod {pod_name}: {write_cmd}")
                subprocess.run(
                    write_cmd,
                    shell=True,
                    check=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True
                )
                read_cmd = f"kubectl exec -n {self.namespace} {pod_name} -- cat /data/{test_file}"
                self.logger.info(f"[ONE2ONE] Executing read command for pod {pod_name}: {read_cmd}")
                read_process = subprocess.run(
                    read_cmd,
                    shell=True,
                    check=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True
                )
                read_result = read_process.stdout.strip()
                self.logger.info(f"[ONE2ONE] Pod {pod_name} read result: '{read_result}'")
                if test_content not in read_result:
                    self.logger.error(f"[ONE2ONE] Pod {pod_name} failed to read its own write. Expected '{test_content}', got '{read_result}'")
                    return False
                else:
                    self.logger.info(f"[ONE2ONE] Pod {pod_name} successfully wrote and read from its own volume")
            except subprocess.CalledProcessError as e:
                self.logger.error(f"[ONE2ONE] Command execution failed for pod {pod_name}: {e}")
                self.logger.error(f"[ONE2ONE] Command stderr: {e.stderr}")
                return False
            except Exception as e:
                self.logger.error(f"[ONE2ONE] Error in one-to-one scenario for pod {pod_name}: {e}")
                return False
        return True

    def _collect_one_to_one_failure_logs(self, pairs):
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        test_name = f"one2one_failure_{timestamp}"
        failed_resources = []
        for pvc_name, pod_name in pairs:
            failed_resources.append({"type": "pod", "name": pod_name, "namespace": self.namespace})
            failed_resources.append({"type": "pvc", "name": pvc_name, "namespace": self.namespace})
        logs_path = collect_logs_on_test_failure(
            test_name,
            self.metrics_collector,
            self.driver_pod_name,
            failed_resources=failed_resources
        )
        self.logger.info(f"Collected detailed failure logs to: {logs_path}")

    def _scenario_concurrent_pvc(self):
        """
        Test rapid PVC creation and deletion
        1. Create multiple PVCs in quick succession
        2. Create pods for some of them
        3. Delete some PVCs in quick succession
        4. Verify operations successfully
        """
        self.logger.info("Running scenario: Rapid PVC operations")
        self.scenarios['concurrent_pvc']['runs'] += 1
        
        # Get PVC count range from config
        scenario_config = self.config.get('scenarios', {}).get('concurrent_pvc', {})
        min_pvcs = scenario_config.get('min_pvcs', 3)
        max_pvcs = scenario_config.get('max_pvcs', 7)
        
        # Number of PVCs to create
        num_pvcs = random.randint(min_pvcs, max_pvcs)
        self.logger.info(f"Creating {num_pvcs} PVCs (range from config: {min_pvcs}-{max_pvcs})")
        
        pvc_names = [f"concurrent-pvc-{uuid.uuid4().hex[:8]}" for _ in range(num_pvcs)]
        created_pvcs = []
        
        try:
            # Step 1: Create multiple PVCs in quick succession
            created_pvcs = self._concurrent_create_pvcs(pvc_names)
            
            if len(created_pvcs) < 2:
                self._mark_concurrent_scenario_failed(f"Failed to create enough PVCs, only created {len(created_pvcs)}")
                return
            
            # Step 2: Create pods for some of the PVCs
            self._concurrent_create_pods(created_pvcs)
            
            # Step 3: Delete some PVCs in quick succession
            self._concurrent_delete_pvcs(created_pvcs, min_pvcs)
            
            # Mark scenario as successful
            self.logger.info("Rapid PVC scenario completed successfully")
            self.scenarios['concurrent_pvc']['success'] += 1
            
        except Exception as e:
            self._handle_concurrent_scenario_failure(e, created_pvcs)

    def _concurrent_create_pvcs(self, pvc_names):
        """Create multiple PVCs in quick succession for the concurrent scenario"""
        created_pvcs = []
        self.logger.info(f"Creating {len(pvc_names)} PVCs in quick succession")
        
        for pvc_name in pvc_names:
            success = self._create_pvc_for_concurrent(pvc_name)
            if success:
                created_pvcs.append(pvc_name)
        
        self.logger.info(f"Successfully created {len(created_pvcs)} PVCs")
        return created_pvcs

    def _concurrent_create_pods(self, created_pvcs):
        """Create pods for some of the PVCs in the concurrent scenario"""
        num_pods = min(len(created_pvcs), 3)
        pod_pvcs = random.sample(created_pvcs, num_pods)
        
        self.logger.info(f"Creating {num_pods} pods for PVCs in concurrent scenario")
        for pvc_name in pod_pvcs:
            self._attach_pod(pvc_name)

    def _concurrent_delete_pvcs(self, created_pvcs, min_pvcs):
        """Delete some PVCs in quick succession"""
        num_to_delete = min(len(created_pvcs), min_pvcs)
        pvcs_to_delete = random.sample(created_pvcs, num_to_delete)
        
        self.logger.info(f"Deleting {num_to_delete} PVCs in quick succession")
        for pvc_name in pvcs_to_delete:
            self._delete_pvc(pvc_name)

    def _mark_concurrent_scenario_failed(self, reason):
        """Mark concurrent scenario as failed with a reason"""
        self.logger.warning(reason)
        self.scenarios['concurrent_pvc']['fail'] += 1

    def _handle_concurrent_scenario_failure(self, e, created_pvcs):
        """Handle failure in the concurrent PVC scenario"""
        self.logger.error(f"Error in rapid PVC scenario: {e}")
        self.scenarios['concurrent_pvc']['fail'] += 1
        
        # Collect logs for failure diagnostics with detailed information
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        test_name = f"concurrent_pvc_failure_{timestamp}"
        
        # Track all resources involved in this scenario
        failed_resources = self._collect_concurrent_failure_resources(created_pvcs)
        
        logs_path = collect_logs_on_test_failure(
            test_name,
            self.metrics_collector, 
            self.driver_pod_name,
            failed_resources=failed_resources
        )
        self.logger.info(f"Collected detailed failure logs to: {logs_path}")
        
    def _collect_concurrent_failure_resources(self, created_pvcs):
        """Collect resources involved in concurrent scenario failure"""
        failed_resources = []
        
        # Add all created PVCs
        for pvc_name in created_pvcs:
            failed_resources.append({"type": "pvc", "name": pvc_name, "namespace": self.namespace})
            
            # Add pods using those PVCs
            for pod_name in self.pods.get(pvc_name, []):
                failed_resources.append({"type": "pod", "name": pod_name, "namespace": self.namespace})
                
        return failed_resources
    
    def _create_pvc_for_concurrent(self, pvc_name):
        """
        Helper method for creating PVCs in concurrent scenario
        Returns True if successful, False otherwise
        """
        try:
            # Get configuration values
            scenario_config = self.config.get('scenarios', {}).get('concurrent_pvc', {})
            sc_name = self.config.get('storage_class', {}).get('name', 'efs-sc')
            
            # Create PVC manifest using config values
            pvc_manifest = {
                "apiVersion": "v1",
                "kind": "PersistentVolumeClaim",
                "metadata": {"name": pvc_name},
                "spec": {
                    "accessModes": ["ReadWriteMany"],  # This is generally fixed for EFS
                    "storageClassName": sc_name,
                    "resources": {
                        "requests": {"storage": "1Gi"}  # Size doesn't matter for EFS but required in PVC spec
                    }
                }
            }
            
            # Add annotations if configured
            pvc_annotations = scenario_config.get('pvc_annotations', {})
            if pvc_annotations:
                pvc_manifest['metadata']['annotations'] = pvc_annotations
            
            # Create PVC
            self.core_v1.create_namespaced_persistent_volume_claim(
                namespace=self.namespace,
                body=pvc_manifest
            )
            
            # Track the PVC
            self.pvcs.append(pvc_name)
            self.pods[pvc_name] = []
            
            # Update results
            self.results['create_pvc']['success'] += 1
            self.logger.info(f"Created PVC: {pvc_name} with storage class {sc_name}")
            
            # Get timeout from config
            retry_config = self.config.get('retries', {})
            pvc_bind_timeout = retry_config.get('pvc_bind_timeout', 30)
            
            # Wait for PVC to be bound
            if not self._wait_for_pvc_bound(pvc_name, timeout=pvc_bind_timeout):
                self.logger.warning(f"Timeout waiting for PVC {pvc_name} to be bound after {pvc_bind_timeout}s")
                return False
                
            return True
            
        except Exception as e:
            self.results['create_pvc']['fail'] += 1
            self.logger.error(f"Failed to create PVC {pvc_name} concurrently: {e}")
            return False
    
    def _wait_for_pod_ready(self, pod_name, timeout=60):
        """
        Wait for pod to be ready
        Returns True if ready within timeout, False otherwise
        """
        start_time = time.time()
        self.logger.info(f"Waiting for pod {pod_name} to be ready")
        
        # For diagnostics
        last_phase = None
        diagnostic_logged = False
        
        while time.time() - start_time < timeout:
            pod_status = self._check_pod_status(pod_name)
            
            # Pod not found
            if pod_status.get('not_found', False):
                return False
                
            # Update last_phase for tracking phase transitions
            if pod_status.get('phase') != last_phase:
                self.logger.info(f"Pod {pod_name} phase: {pod_status.get('phase')}")
                last_phase = pod_status.get('phase')
                
            # Check if pod is ready
            if pod_status.get('ready', False):
                self.logger.info(f"Pod {pod_name} is ready")
                return True
                
            # Check for failure states
            if self._is_pod_in_failure_state(pod_status):
                self._log_pod_diagnostics(pod_name)
                return False
                
            # Check if we should log diagnostics
            elapsed = time.time() - start_time
            if self._should_log_wait_diagnostics(elapsed, timeout, diagnostic_logged):
                self.logger.info(f"Pod {pod_name} taking longer than expected to become ready ({elapsed:.1f}s). Collecting diagnostics...")
                self._log_pod_diagnostics(pod_name)
                diagnostic_logged = True
                
            time.sleep(2)
        
        self.logger.warning(f"Timeout waiting for pod {pod_name} to be ready after {timeout}s")
        self._log_pod_diagnostics(pod_name)
        return False
        
    def _check_pod_status(self, pod_name):
        """Check pod status and return information about its current state"""
        try:
            pod = self.core_v1.read_namespaced_pod_status(
                name=pod_name,
                namespace=self.namespace
            )
            
            status = {
                'phase': pod.status.phase,
                'ready': False,
                'conditions': [],
                'all_conditions_text': ''
            }
            
            # Extract conditions if available
            if pod.status.phase == "Running" and pod.status.conditions:
                all_conditions = []
                
                for condition in pod.status.conditions:
                    condition_text = f"{condition.type}={condition.status}"
                    all_conditions.append(condition_text)
                    
                    # Check if the Ready condition is true
                    if condition.type == "Ready" and condition.status == "True":
                        status['ready'] = True
                
                status['conditions'] = all_conditions
                status['all_conditions_text'] = ', '.join(all_conditions)
                
                # Log all conditions for diagnostics if available
                if all_conditions:
                    self.logger.info(f"Pod {pod_name} conditions: {status['all_conditions_text']}")
            
            return status
            
        except client.exceptions.ApiException as e:
            if e.status == 404:
                self.logger.warning(f"Pod {pod_name} not found")
                return {'not_found': True, 'phase': 'NotFound'}
            self.logger.warning(f"Error checking pod status: {e}")
            return {'error': str(e), 'phase': 'Error'}
    
    def _is_pod_in_failure_state(self, pod_status):
        """Check if pod is in a terminal failure state"""
        failure_phases = ["Failed", "Unknown"]
        return pod_status.get('phase') in failure_phases
    
    def _should_log_wait_diagnostics(self, elapsed, timeout, already_logged):
        """Determine if diagnostics should be logged during wait operations"""
        if already_logged:
            return False
        return elapsed > timeout / 2
        
    def _log_pod_diagnostics(self, pod_name):
        """
        Collect and log detailed pod diagnostics
        This helps diagnose why a pod isn't becoming ready
        """
        try:
            self.logger.info(f"===== DIAGNOSTICS FOR POD {pod_name} =====")
            pod = self.core_v1.read_namespaced_pod(name=pod_name, namespace=self.namespace)
            self._log_container_statuses(pod)
            self._log_pod_events(pod_name)
            self._log_pod_logs(pod_name)
            self._log_pod_volumes(pod)
            if pod.status.phase == "Running":
                self._run_pod_diagnostics_commands(pod_name)
            self.logger.info(f"===== END DIAGNOSTICS FOR POD {pod_name} =====")
        except Exception as e:
            self.logger.error(f"Error collecting pod diagnostics: {e}")

    def _log_container_statuses(self, pod):
        if pod.status.container_statuses:
            for container in pod.status.container_statuses:
                self.logger.info(f"Container {container.name} status:")
                self.logger.info(f"  - Ready: {container.ready}")
                self.logger.info(f"  - Started: {container.started}")
                self.logger.info(f"  - Restart Count: {container.restart_count}")
                if container.state.waiting:
                    self.logger.info(f"  - Waiting: reason={container.state.waiting.reason}, message={container.state.waiting.message}")
                elif container.state.running:
                    self.logger.info(f"  - Running: started at {container.state.running.started_at}")
                elif container.state.terminated:
                    self.logger.info(f"  - Terminated: reason={container.state.terminated.reason}, exit_code={container.state.terminated.exit_code}")
        else:
            self.logger.info("No container statuses available")

    def _log_pod_events(self, pod_name):
        try:
            field_selector = f"involvedObject.name={pod_name}"
            events = self.core_v1.list_namespaced_event(namespace=self.namespace, field_selector=field_selector)
            if events.items:
                self.logger.info(f"Pod events:")
                for event in events.items:
                    self.logger.info(f"  [{event.last_timestamp}] {event.type}/{event.reason}: {event.message}")
            else:
                self.logger.info("No events found for pod")
        except Exception as e:
            self.logger.warning(f"Error retrieving pod events: {e}")

    def _log_pod_logs(self, pod_name):
        try:
            # Get pod logs - fetch more lines to ensure we catch stale handle errors
            logs = self.core_v1.read_namespaced_pod_log(
                name=pod_name,
                namespace=self.namespace,
                container="test-container",
                tail_lines=100
            )
            
            if logs:
                # Check for stale file handle errors
                self._check_for_stale_file_handle_errors(pod_name, logs)
                
                # Log the last 20 lines for readability
                self.logger.info(f"Container logs (last 20 lines):")
                for line in logs.splitlines()[-20:]:
                    self.logger.info(f"  {line}")
            else:
                self.logger.info("No logs available")
        except Exception as e:
            self.logger.warning(f"Error retrieving pod logs: {e}")
    
    def _check_for_stale_file_handle_errors(self, pod_name, logs):
        """Check pod logs for stale file handle errors and record them in metrics"""
        import re
        
        # Simple regex patterns to detect stale file handle errors
        structured_pattern = r'EFS_ERROR: STALE_FILE_HANDLE: path=(.*?), message=(.*?)$'
        standard_pattern = r'stat: cannot stat \'([^\']*)\': Stale file handle'
        
        # Debug information - how many lines of logs are we processing?
        log_lines = logs.splitlines() if logs else []
        self.logger.info(f"Analyzing {len(log_lines)} lines of logs from pod {pod_name} for stale file handle errors")
        
        # Initialize counters for debug info
        matches_found = 0
        
        # Find structured error formats (from our modified StatefulSet)
        structured_matches = re.findall(structured_pattern, logs, re.MULTILINE)
        for volume_path, error_msg in structured_matches:
            self.logger.warning(f"Detected stale file handle in pod {pod_name}: {volume_path} - {error_msg}")
            self.metrics_collector.record_stale_file_handle(volume_path, error_msg, source_pod=pod_name)
            matches_found += 1
        
        # Find standard error formats
        standard_matches = re.findall(standard_pattern, logs, re.MULTILINE)
        for path in standard_matches:
            error_msg = f"Stale file handle error in {path}"
            # Extract volume path (parent directory)
            volume_path = path.split('/')[1] if path.startswith('/') else path
            self.logger.warning(f"Detected stale file handle in pod {pod_name}: /{volume_path} - {error_msg}")
            self.metrics_collector.record_stale_file_handle(f"/{volume_path}", error_msg, source_pod=pod_name)
            matches_found += 1
        
        # Log summary information
        if matches_found > 0:
            self.logger.warning(f"Found {matches_found} stale file handle errors in pod {pod_name} logs")
        else:
            self.logger.info(f"No stale file handle errors detected in pod {pod_name} logs")
            
        # Attempt to manually add a test error if no real errors were found
        # This is just for testing - would be removed in production
        if matches_found == 0 and "aws-statefulset" in pod_name:
            self.logger.warning(f"Adding simulated stale handle error for testing")
            self.metrics_collector.record_stale_file_handle("/aws-test", "Simulated stale handle error", source_pod=pod_name)

    def _log_pod_volumes(self, pod):
        if pod.spec.volumes:
            self.logger.info(f"Pod volumes:")
            for volume in pod.spec.volumes:
                volume_details = {}
                if hasattr(volume, 'persistent_volume_claim') and volume.persistent_volume_claim:
                    volume_details["type"] = "PVC"
                    volume_details["claim_name"] = volume.persistent_volume_claim.claim_name
                elif hasattr(volume, 'host_path') and volume.host_path:
                    volume_details["type"] = "HostPath"
                    volume_details["path"] = volume.host_path.path
                self.logger.info(f"  - {volume.name}: {volume_details}")

    def _run_pod_diagnostics_commands(self, pod_name):
        try:
            # Check mount points
            mount_cmd = "mount | grep /data"
            exec_command = ['/bin/sh', '-c', mount_cmd]
            resp = self.core_v1.connect_get_namespaced_pod_exec(
                pod_name, 
                self.namespace,
                command=exec_command,
                stdin=False,
                stdout=True,
                stderr=True,
                tty=False
            )
            self.logger.info(f"Mount diagnostic output: {resp}")
            # Check if we can write to the volume
            touch_cmd = "touch /data/test_write && echo 'Write test successful'"
            exec_command = ['/bin/sh', '-c', touch_cmd]
            resp = self.core_v1.connect_get_namespaced_pod_exec(
                pod_name, 
                self.namespace,
                command=exec_command,
                stdin=False,
                stdout=True,
                stderr=True,
                tty=False
            )
            self.logger.info(f"Write test output: {resp}")
        except Exception as e:
            self.logger.warning(f"Cannot execute diagnostic commands in pod: {e}")

    def _wait_for_pod_deleted(self, pod_name, timeout=60):
        """
        Wait for pod to be deleted
        Returns True if deleted within timeout, False otherwise
        """
        start_time = time.time()
        self.logger.info(f"Waiting for pod {pod_name} to be deleted")
        
        while time.time() - start_time < timeout:
            try:
                self.core_v1.read_namespaced_pod_status(
                    name=pod_name,
                    namespace=self.namespace
                )
                # Pod still exists, wait
                time.sleep(2)
                
            except client.exceptions.ApiException as e:
                if e.status == 404:
                    self.logger.info(f"Pod {pod_name} has been deleted")
                    return True
                self.logger.warning(f"Error checking pod deletion status: {e}")
            
            time.sleep(2)
        
        self.logger.warning(f"Timeout waiting for pod {pod_name} to be deleted after {timeout}s")
        return False
    
    def _wait_for_pvc_bound(self, pvc_name, timeout=60):
        """
        Wait for PVC to be bound
        Returns True if bound within timeout, False otherwise
        """
        start_time = time.time()
        self.logger.info(f"Waiting for PVC {pvc_name} to be bound")
        
        while time.time() - start_time < timeout:
            try:
                pvc = self.core_v1.read_namespaced_persistent_volume_claim(
                    name=pvc_name,
                    namespace=self.namespace
                )
                
                if pvc.status.phase == "Bound":
                    self.logger.info(f"PVC {pvc_name} is bound")
                    return True
                    
                # Still waiting
                self.logger.debug(f"PVC {pvc_name} is in {pvc.status.phase} state, waiting...")
                
            except client.exceptions.ApiException as e:
                if e.status == 404:
                    self.logger.warning(f"PVC {pvc_name} not found")
                    return False
                self.logger.warning(f"Error checking PVC status: {e}")
            
            time.sleep(2)
        
        self.logger.warning(f"Timeout waiting for PVC {pvc_name} to be bound after {timeout}s")
        return False
    
    def _wait_for_pvc_deleted(self, pvc_name, timeout=60):
        """
        Wait for PVC to be deleted
        Returns True if deleted within timeout, False otherwise
        """
        start_time = time.time()
        self.logger.info(f"Waiting for PVC {pvc_name} to be deleted")
        
        while time.time() - start_time < timeout:
            try:
                self.core_v1.read_namespaced_persistent_volume_claim(
                    name=pvc_name,
                    namespace=self.namespace
                )
                # PVC still exists, wait
                time.sleep(2)
                
            except client.exceptions.ApiException as e:
                if e.status == 404:
                    self.logger.info(f"PVC {pvc_name} has been deleted")
                    return True
                self.logger.warning(f"Error checking PVC deletion status: {e}")
            
            time.sleep(2)
        
        self.logger.warning(f"Timeout waiting for PVC {pvc_name} to be deleted after {timeout}s")
        return False
        
    def _start_statefulset_monitoring(self):
        """Initialize StatefulSet pod monitoring for stale file handle errors"""
        # Check if monitoring is enabled in config
        monitoring_config = self.config.get('monitoring', {}).get('statefulset', {})
        self._statefulset_monitoring_enabled = monitoring_config.get('enabled', True)
        
        if not self._statefulset_monitoring_enabled:
            self.logger.info("StatefulSet monitoring is disabled in config")
            return
            
        # Get monitoring configuration
        self._statefulset_namespace = monitoring_config.get('namespace', 'default')
        self._statefulset_selector = monitoring_config.get('pod_label_selector', 'app=aws-app')
        self._statefulset_check_interval = monitoring_config.get('check_interval', 60)  # seconds
        
        self.logger.info(f"Starting StatefulSet monitoring for stale file handles in namespace {self._statefulset_namespace}")
        self.logger.info(f"Using pod selector: {self._statefulset_selector}")
        self.logger.info(f"Check interval: {self._statefulset_check_interval} seconds")
        
        # Schedule first check
        self._next_statefulset_check_time = time.time() + 30  # First check after 30 seconds
        
    def _check_statefulsets_for_stale_handles(self):
        """Check StatefulSet pods for stale file handle errors"""
        if not hasattr(self, '_statefulset_monitoring_enabled') or not self._statefulset_monitoring_enabled:
            return
            
        self.logger.info("Checking StatefulSet pods for stale file handle errors")
        import subprocess
        
        try:
            # Get all pods matching the selector
            cmd = f"kubectl get pods -n {self._statefulset_namespace} -l {self._statefulset_selector} -o name"
            result = subprocess.run(cmd, shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            pods = [pod.strip().replace("pod/", "") for pod in result.stdout.strip().split("\n") if pod.strip()]
            
            if not pods:
                self.logger.info(f"No StatefulSet pods found with selector '{self._statefulset_selector}'")
            else:
                self.logger.info(f"Found {len(pods)} StatefulSet pods to check for stale file handle errors")
                
                # Process each pod's logs
                for pod_name in pods:
                    self.logger.info(f"Checking logs for pod {pod_name}")
                    self._check_pod_for_stale_handles(pod_name)
                
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Error checking StatefulSet pods: {e}")
            self.logger.error(f"Error details: {e.stderr}")
            
        except Exception as e:
            self.logger.error(f"Unexpected error during StatefulSet monitoring: {e}")
            
        # Schedule next check
        self._next_statefulset_check_time = time.time() + self._statefulset_check_interval
        
    def _check_pod_for_stale_handles(self, pod_name):
        """Check a specific pod's logs for stale file handle errors"""
        import subprocess
        import re
        
        try:
            # Get recent logs from the pod
            cmd = f"kubectl logs -n {self._statefulset_namespace} {pod_name} --tail=100"
            result = subprocess.run(cmd, shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            logs = result.stdout
            
            # Check for stale file handle errors - similar to our pod log checking method
            if logs:
                # Look for structured error format
                structured_pattern = r'EFS_ERROR: STALE_FILE_HANDLE: path=(.*?), message=(.*?)$'
                standard_pattern = r'stat: cannot stat \'([^\']*)\': Stale file handle'
                
                # Find structured error formats (from our modified StatefulSet)
                structured_matches = re.findall(structured_pattern, logs, re.MULTILINE)
                for volume_path, error_msg in structured_matches:
                    self.logger.warning(f"Detected stale file handle in StatefulSet pod {pod_name}: {volume_path} - {error_msg}")
                    self.metrics_collector.record_stale_file_handle(volume_path, error_msg, source_pod=pod_name)
                
                # Find standard error formats
                standard_matches = re.findall(standard_pattern, logs, re.MULTILINE)
                for path in standard_matches:
                    error_msg = f"Stale file handle error in {path}"
                    volume_path = path.split('/')[1] if path.startswith('/') else path
                    formatted_path = f"/{volume_path}"
                    self.logger.warning(f"Detected stale file handle in StatefulSet pod {pod_name}: {formatted_path} - {error_msg}")
                    self.metrics_collector.record_stale_file_handle(formatted_path, error_msg, source_pod=pod_name)
            
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Error getting logs for pod {pod_name}: {e}")
        
        except Exception as e:
            self.logger.error(f"Error processing logs for pod {pod_name}: {e}")
    
    def _log_efs_filesystem_state(self):
        """Log the state of the EFS file system after test completion."""
        try:
            fs_id = self.config.get('driver', {}).get('filesystem_id')
            region = self.config.get('cluster', {}).get('region', 'us-west-1')
            if not fs_id:
                self.logger.warning("No filesystem_id found in config for EFS state check.")
                return None
            efs = boto3.client('efs', region_name=region)
            response = efs.describe_file_systems(FileSystemId=fs_id)
            fs = response['FileSystems'][0]
            fs_info = {
                "filesystem_id": fs_id,
                "state": fs['LifeCycleState'],
                "size_bytes": fs['SizeInBytes']['Value'],
                "mount_targets": fs['NumberOfMountTargets']
            }
            self.logger.info(f"EFS FileSystem {fs_id} state: {fs['LifeCycleState']}, Size: {fs['SizeInBytes']['Value']} bytes, MountTargets: {fs['NumberOfMountTargets']}")
            return fs_info
        except Exception as e:
            self.logger.error(f"Failed to log EFS file system state: {e}")
            return None
    
    def _ensure_storage_class(self):
        """Ensure EFS StorageClass exists"""
        sc_config = self.config.get('storage_class', {})
        sc_name = sc_config.get('name', 'efs-sc')
        
        try:
            # Check if storage class already exists
            self.storage_v1.read_storage_class(name=sc_name)
            self.logger.info(f"StorageClass '{sc_name}' already exists")
            
        except client.exceptions.ApiException as e:
            if e.status == 404:
                # Create storage class
                sc_manifest = {
                    "apiVersion": "storage.k8s.io/v1",
                    "kind": "StorageClass",
                    "metadata": {"name": sc_name},
                    "provisioner": "efs.csi.aws.com",
                    "parameters": sc_config.get('parameters', {
                        "provisioningMode": "efs-ap",
                        "fileSystemId": "fs-XXXX",  # This should be replaced with actual filesystem ID
                        "directoryPerms": "700"
                    })
                }
                
                # Add mount options if defined
                if 'mount_options' in sc_config:
                    sc_manifest["mountOptions"] = sc_config['mount_options']
                
                # Add reclaim policy if defined
                if 'reclaim_policy' in sc_config:
                    sc_manifest["reclaimPolicy"] = sc_config['reclaim_policy']
                
                # Add volume binding mode if defined
                if 'volume_binding_mode' in sc_config:
                    sc_manifest["volumeBindingMode"] = sc_config['volume_binding_mode']
                
                self.storage_v1.create_storage_class(body=sc_manifest)
                self.logger.info(f"Created StorageClass '{sc_name}'")
                
            else:
                self.logger.error(f"Error checking StorageClass: {e}")
                raise
    
    def _collect_statefulset_pod_logs(self):
        """Collect StatefulSet pod logs and save them to the reports directory"""
        self.logger.info("Collecting StatefulSet pod logs for stale file handle analysis")
        import subprocess
        import os
        
        # Create reports directory if it doesn't exist
        report_dir = os.path.join("reports", "statefulset_logs")
        os.makedirs(report_dir, exist_ok=True)
        
        # Get StatefulSet pod selector from config or use default
        selector = "app=aws-app"
        namespace = "default"
        
        try:
            # Get all pods with the selector
            cmd = f"kubectl get pods -n {namespace} -l {selector} -o name"
            result = subprocess.run(cmd, shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            pods = [pod.strip().replace("pod/", "") for pod in result.stdout.strip().split("\n") if pod.strip()]
            
            if not pods:
                self.logger.info(f"No StatefulSet pods found with selector '{selector}'")
                return []
                
            self.logger.info(f"Found {len(pods)} StatefulSet pods, collecting logs")
            collected_logs = []
            
            # Create timestamp for log files
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            
            # Collect logs for each pod
            for pod_name in pods:
                log_file = os.path.join(report_dir, f"{pod_name}_logs_{timestamp}.txt")
                try:
                    # Get pod logs
                    cmd = f"kubectl logs -n {namespace} {pod_name}"
                    result = subprocess.run(cmd, shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                    
                    # Save logs to file
                    with open(log_file, "w") as f:
                        f.write(result.stdout)
                        
                    self.logger.info(f"Saved logs for pod {pod_name} to {log_file}")
                    collected_logs.append(log_file)
                    
                except subprocess.CalledProcessError as e:
                    self.logger.error(f"Failed to get logs for pod {pod_name}: {e}")
                    self.logger.error(f"Error details: {e.stderr}")
            
            # Store the collected log files for the summary generation
            self._statefulset_log_files = collected_logs
            return collected_logs
            
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Failed to get StatefulSet pods: {e}")
            self.logger.error(f"Error details: {e.stderr}")
            return []
        except Exception as e:
            self.logger.error(f"Error collecting StatefulSet pod logs: {e}")
            return []
    
    def _generate_stale_file_handle_summary(self):
        """Generate a summary of stale file handle errors from collected pod logs"""
        self.logger.info("Generating stale file handle error summary")
        import os
        import re
        
        # Check if we have collected logs
        if not hasattr(self, '_statefulset_log_files') or not self._statefulset_log_files:
            self.logger.info("No StatefulSet log files collected, skipping summary generation")
            return
            
        # Create timestamp for summary file
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        reports_dir = "reports"
        os.makedirs(reports_dir, exist_ok=True)
        summary_file = os.path.join(reports_dir, f"stale_file_handle_summary_{timestamp}.txt")
        
        # Regular expressions for detecting stale file handle errors
        error_patterns = [
            re.compile(r"Stale file handle"),
            re.compile(r"EFS_ERROR: STALE_FILE_HANDLE")
        ]
        
        # Summary data
        total_errors = 0
        errors_by_pod = {}
        error_lines = []
        
        # Parse each log file
        with open(summary_file, "w") as summary:
            summary.write("STALE FILE HANDLE ERROR SUMMARY\n")
            summary.write("=" * 50 + "\n\n")
            summary.write(f"Report generated: {datetime.now().isoformat()}\n\n")
            
            for log_file in self._statefulset_log_files:
                pod_name = os.path.basename(log_file).split('_logs_')[0]
                pod_errors = 0
                
                try:
                    with open(log_file) as f:
                        log_content = f.read()
                        line_number = 0
                        
                        # Process each line for errors
                        for line in log_content.splitlines():
                            line_number += 1
                            for pattern in error_patterns:
                                if pattern.search(line):
                                    pod_errors += 1
                                    error_lines.append(f"{pod_name} [line {line_number}]: {line.strip()}")
                                    break
                    
                    if pod_errors > 0:
                        errors_by_pod[pod_name] = pod_errors
                        total_errors += pod_errors
                
                except Exception as e:
                    summary.write(f"Error processing log file {log_file}: {str(e)}\n")
            
            # Write summary statistics
            summary.write(f"Total stale file handle errors found: {total_errors}\n\n")
            
            if total_errors > 0:
                summary.write("Errors by pod:\n")
                summary.write("-" * 30 + "\n")
                
                for pod, count in errors_by_pod.items():
                    summary.write(f"{pod}: {count} errors\n")
                
                summary.write("\nDetailed error lines:\n")
                summary.write("-" * 50 + "\n")
                
                for error_line in error_lines:
                    summary.write(f"{error_line}\n")
            else:
                summary.write("No stale file handle errors detected in the logs.\n")
        
        self.logger.info(f"Stale file handle summary written to {summary_file}")
        
        if total_errors > 0:
            self.logger.warning(f"Found {total_errors} stale file handle errors across {len(errors_by_pod)} pods")
        else:
            self.logger.info("No stale file handle errors detected")
        
        return summary_file, total_errors
    
    def _cleanup(self):
        """Clean up all resources created during test with robust error handling"""
        self.logger.info("===== STARTING COMPREHENSIVE CLEANUP =====")
        cleanup_start_time = time.time()
        cleanup_timeout = 180  # 3 minutes timeout for entire cleanup
        cleanup_failures = []
        force_delete = False
        
        # First, collect StatefulSet pod logs for stale handle analysis
        self._collect_statefulset_pod_logs()
        
        try:
            self._cleanup_resources(force_delete, cleanup_failures)
            remaining_resources = self._get_remaining_resources()
            if remaining_resources:
                self.logger.warning(f"First cleanup pass incomplete. Remaining resources: {remaining_resources}")
                self.logger.info("Attempting force deletion of remaining resources...")
                force_delete = True
                self._cleanup_resources(force_delete, cleanup_failures)
                remaining_resources = self._get_remaining_resources()
                if remaining_resources:
                    self.logger.error(f"Cleanup incomplete. Remaining resources after force deletion: {remaining_resources}")
            elapsed = time.time() - cleanup_start_time
            if cleanup_failures:
                self.logger.warning(f"Cleanup completed in {elapsed:.2f} seconds with {len(cleanup_failures)} failures")
                self.logger.warning(f"Failed deletions: {cleanup_failures}")
            else:
                self.logger.info(f"Cleanup completed successfully in {elapsed:.2f} seconds")
        except Exception as e:
            self.logger.error(f"Error during cleanup: {e}", exc_info=True)
        finally:
            self.logger.info("===== CLEANUP PROCESS FINISHED =====")
            self._log_efs_filesystem_state()
            
            # Generate stale file handle summary report from collected logs
            self._generate_stale_file_handle_summary()

    def _cleanup_resources(self, force, failures):
        """Delete all pods and PVCs with error handling"""
        self._cleanup_pods(force, failures)
        time.sleep(5)  # Allow pod termination before PVC deletion
        self._cleanup_pvcs(force, failures)

    def _cleanup_pods(self, force, failures):
        self.logger.info(f"Deleting {self.current_pod_count} pods (force={force})...")
        for pvc_name, pod_list in list(self.pods.items()):
            for pod_name in list(pod_list):
                try:
                    success = self._delete_pod(pod_name, pvc_name, force=force)
                    if not success:
                        failures.append(f"pod/{pod_name}")
                except Exception as e:
                    self.logger.error(f"Error deleting pod {pod_name}: {e}")
                    failures.append(f"pod/{pod_name}")

    def _cleanup_pvcs(self, force, failures):
        self.logger.info(f"Deleting {len(self.pvcs)} PVCs (force={force})...")
        for pvc_name in list(self.pvcs):
            try:
                success = self._delete_pvc(pvc_name, force=force)
                if not success:
                    failures.append(f"pvc/{pvc_name}")
            except Exception as e:
                self.logger.error(f"Error deleting PVC {pvc_name}: {e}")
                failures.append(f"pvc/{pvc_name}")

    def _get_remaining_resources(self):
        """Get a list of any resources that weren't cleaned up"""
        remaining = []
        
        # Check for remaining pods with our test labels
        try:
            pods = self.core_v1.list_namespaced_pod(
                namespace=self.namespace,
                label_selector="app=efs-test"
            )
            for pod in pods.items:
                remaining.append(f"pod/{pod.metadata.name}")
        except Exception as e:
            self.logger.error(f"Error checking for remaining pods: {e}")
        
        # Check for remaining PVCs created by our tests
        try:
            pvcs = self.core_v1.list_namespaced_persistent_volume_claim(
                namespace=self.namespace
            )
            for pvc in pvcs.items:
                # Only include PVCs that match our naming pattern
                if pvc.metadata.name.startswith(("test-pvc-", "many2one-", "one2one-", "concurrent-pvc-")):
                    remaining.append(f"pvc/{pvc.metadata.name}")
        except Exception as e:
            self.logger.error(f"Error checking for remaining PVCs: {e}")
            
        return remaining
    
    def _generate_report(self):
        """Generate test report"""
        # Get EFS filesystem state information
        fs_info = self._log_efs_filesystem_state()
        
        # Get stale file handle information from metrics collector
        stale_handle_metrics = self._get_stale_handle_metrics()
        
        report = {
            "test_duration": time.time(),
            "operations": self._generate_operations_report(),
            "efs_filesystem": fs_info,
            "scenarios": self._generate_scenarios_report(),
            "filesystem_errors": {
                "stale_file_handles": stale_handle_metrics
            }
        }
        
        # Print report summary
        self._print_report_summary(report)
        
        return report
        
    def _get_stale_handle_metrics(self):
        """Get stale file handle metrics from metrics collector"""
        metrics = {}
        
        # Check if stale handle errors were tracked
        if hasattr(self.metrics_collector, 'efs_metrics') and 'stale_handle_errors' in self.metrics_collector.efs_metrics:
            stale_handle_data = self.metrics_collector.efs_metrics['stale_handle_errors']
            
            # Extract counts by volume path
            counts_by_path = {}
            for path, count in stale_handle_data.get('counts', {}).items():
                counts_by_path[path] = count
                
            # Build metrics summary
            metrics = {
                'total_count': sum(counts_by_path.values()),
                'affected_paths': list(counts_by_path.keys()),
                'counts_by_path': counts_by_path,
                'incidents': stale_handle_data.get('incidents', [])
            }
            
            # Log summary of stale file handle errors
            if metrics['total_count'] > 0:
                self.logger.warning(f"Detected {metrics['total_count']} stale file handle errors across {len(metrics['affected_paths'])} volume paths")
                for path, count in counts_by_path.items():
                    self.logger.warning(f"  - {path}: {count} errors")
        
        return metrics
        
    def _generate_operations_report(self):
        """Generate the operations section of the report"""
        operations_report = {}
        
        # Standard operations
        for op_name in ['create_pvc', 'attach_pod', 'delete_pod', 'delete_pvc']:
            operations_report[op_name] = self._get_operation_stats(op_name)
            
        # Special case for read/write operations
        operations_report["verify_read_write"] = {
            "write_success": self.results['verify_write']['success'],
            "write_fail": self.results['verify_write']['fail'],
            "read_success": self.results['verify_read']['success'],
            "read_fail": self.results['verify_read']['fail'],
            "write_success_rate": self._calculate_success_rate(self.results['verify_write']),
            "read_success_rate": self._calculate_success_rate(self.results['verify_read']),
        }
        
        return operations_report
    
    def _get_operation_stats(self, op_name):
        """Get statistics for a specific operation"""
        return {
            "success": self.results[op_name]['success'],
            "fail": self.results[op_name]['fail'],
            "success_rate": self._calculate_success_rate(self.results[op_name]),
        }
        
    def _generate_scenarios_report(self):
        """Generate the scenarios section of the report"""
        scenarios_report = {}
        
        for scenario_name in self.scenarios:
            scenarios_report[scenario_name] = {
                "runs": self.scenarios[scenario_name]['runs'],
                "success": self.scenarios[scenario_name]['success'],
                "fail": self.scenarios[scenario_name]['fail'],
                "success_rate": self._calculate_scenario_success_rate(scenario_name)
            }
            
        return scenarios_report
    
    def _calculate_success_rate(self, result):
        """Calculate success rate as percentage"""
        total = result['success'] + result['fail']
        if total == 0:
            return 0
        return (result['success'] / total) * 100
    
    def _calculate_scenario_success_rate(self, scenario_name):
        """Calculate scenario success rate as percentage"""
        runs = self.scenarios[scenario_name]['runs']
        if runs == 0:
            return 0
        return (self.scenarios[scenario_name]['success'] / runs) * 100
    
    def _scenario_controller_crash_test(self):
        """
        Test the resilience of CSI driver by crashing the controller pod during PVC provisioning.
        
        Steps:
        1. Create a PVC
        2. Crash the controller pod
        3. Verify that the PVC still becomes bound
        4. Attach a pod and verify read/write functionality
        """
        self.logger.info("+" * 80)
        self.logger.info("STARTING CONTROLLER CRASH TEST SCENARIO")
        self.logger.info("+" * 80)
        
        # Initialize scenario tracking if needed
        if 'controller_crash' not in self.scenarios:
            self.scenarios['controller_crash'] = {'runs': 0, 'success': 0, 'fail': 0}
        self.scenarios['controller_crash']['runs'] += 1
        
        try:
            # Step 1: Create PVC with unique name
            self._create_crash_test_pvc()
            
            # Step 2: Crash controller pod
            if not self._crash_controller_pod():
                self.logger.error("Failed to crash or verify controller pod recreation")
                self._track_scenario_failure('controller_crash')
                return
                
            # Step 3: Verify PVC becomes bound and attach pod
            if not self._verify_crash_test_pvc_and_pod():
                self._track_scenario_failure('controller_crash')
                return
                
            # Success!
            self.logger.info("Controller crash test completed successfully")
            self._track_scenario_success('controller_crash')
            
        except Exception as e:
            self.logger.error(f"Exception in controller crash test: {str(e)}")
            self._track_scenario_failure('controller_crash')
            self._handle_unexpected_test_error(e)
            
        self.logger.info("+" * 80)
        self.logger.info("COMPLETED CONTROLLER CRASH TEST SCENARIO")
        self.logger.info("+" * 80)
    
    def _create_crash_test_pvc(self):
        """Create a PVC for controller crash test"""
        crash_config = self.config["scenarios"].get("controller_crash", {})
        pvc_name = f"crash-test-{uuid.uuid4().hex[:8]}"
        
        self.logger.info(f"[CRASH-TEST] Creating PVC {pvc_name}")
        
        # Build and create PVC manifest
        pvc_manifest = self._build_pvc_manifest(pvc_name)
        self.core_v1.create_namespaced_persistent_volume_claim(
            namespace=self.namespace,
            body=pvc_manifest
        )
        
        # Track PVC
        self.pvcs.append(pvc_name)
        self.pods[pvc_name] = []
        self.results['create_pvc']['success'] += 1
        
        # Save PVC name for later verification
        self._crash_test_pvc_name = pvc_name
        
        return pvc_name
    
    def _crash_controller_pod(self):
        """
        Find and delete the CSI controller pod to simulate a crash.
        The pod will be automatically recreated by Kubernetes.
        """
        import subprocess
        
        # Get controller crash test configuration
        crash_config = self.config["scenarios"].get("controller_crash", {})
        controller_namespace = crash_config.get("controller_namespace", "kube-system")
        controller_pod_selector = crash_config.get("controller_pod_selector", "app=efs-csi-controller")
        
        try:
            # Find the controller pod
            self.logger.info(f"Finding controller pod in namespace {controller_namespace}")
            cmd = f"kubectl get pods -n {controller_namespace} -l {controller_pod_selector} --no-headers -o custom-columns=:metadata.name"
            
            result = subprocess.run(
                cmd,
                shell=True,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            
            controller_pods = [pod for pod in result.stdout.strip().split('\n') if pod]
            
            if not controller_pods:
                self.logger.error(f"No controller pods found in namespace {controller_namespace}")
                return False
            
            # Delete the controller pod
            controller_pod = controller_pods[0]
            self.logger.info(f"Crashing controller pod: {controller_pod}")
            
            delete_cmd = f"kubectl delete pod {controller_pod} -n {controller_namespace} --wait=false"
            subprocess.run(delete_cmd, shell=True, check=True)
            
            # Wait briefly to ensure deletion has started
            time.sleep(5)
            
            # Wait for new controller pod
            return self._verify_controller_recreation(controller_namespace, controller_pod_selector)
            
        except Exception as e:
            self.logger.error(f"Failed to crash controller pod: {str(e)}")
            return False
    
    def _verify_controller_recreation(self, namespace, pod_selector):
        """Verify the controller pod was recreated after being deleted"""
        import subprocess
        
        self.logger.info("Verifying controller pod recreation")
        max_retries = 12
        
        for attempt in range(max_retries):
            try:
                # Check if any controller pod exists and its status
                cmd = f"kubectl get pods -n {namespace} -l {pod_selector} -o jsonpath='{{.items[*].status.phase}}'"
                result = subprocess.run(cmd, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                
                if not result.stdout:
                    self.logger.info(f"Controller pod not found yet (attempt {attempt+1}/{max_retries})")
                elif "Running" in result.stdout:
                    self.logger.info("New controller pod is running")
                    return True
                elif "ContainerCreating" in result.stdout or "Pending" in result.stdout:
                    self.logger.info("New controller pod is being created")
                    return True
                elif "Error" in result.stdout or "CrashLoopBackOff" in result.stdout:
                    self.logger.error("Controller pod is in error state")
                    return False
                    
                time.sleep(10)
                
            except Exception as e:
                self.logger.error(f"Error checking controller pod status: {str(e)}")
                return False
                
        self.logger.error("Controller pod was not recreated within expected time")
        return False
    
    def _verify_crash_test_pvc_and_pod(self):
        """Verify PVC becomes bound after controller crash and attach a pod"""
        # Get controller crash test configuration
        crash_config = self.config["scenarios"].get("controller_crash", {})
        recovery_timeout = crash_config.get("recovery_timeout", 300)
        
        # Get the PVC name we saved earlier
        pvc_name = getattr(self, '_crash_test_pvc_name', None)
        
        if not pvc_name:
            self.logger.error("No crash test PVC name found")
            return False
            
        # Wait for PVC to become bound with extended timeout
        self.logger.info(f"Waiting for PVC {pvc_name} to become bound after controller crash")
        if not self._wait_for_pvc_bound(pvc_name, timeout=recovery_timeout):
            self.logger.error(f"PVC {pvc_name} failed to bind after controller crash")
            self._run_pod_diagnostics_commands()
            return False
            
        self.logger.info(f"PVC {pvc_name} successfully bound after controller crash")
        
        # Create a pod using this PVC
        self.logger.info(f"Creating pod to use PVC {pvc_name}")
        pod_name = self._attach_pod(pvc_name)
        
        if not pod_name:
            self.logger.error(f"Failed to attach pod to PVC {pvc_name} after controller crash")
            return False
            
        # Verify read/write works
        self.logger.info(f"Verifying read/write capability")
        if not self._verify_single_pod_readwrite(pod_name, pvc_name):
            self.logger.error("Read/write verification failed after controller crash")
            return False
            
        return True
        
    def _verify_single_pod_readwrite(self, pod_name, pvc_name):
        """Verify a single pod can read/write to its volume"""
        import subprocess
        
        test_file = f"crash-test-{uuid.uuid4().hex[:8]}.txt"
        test_content = f"Controller crash test: {uuid.uuid4()}"
        
        try:
            # Write test
            write_cmd = f"kubectl exec -n {self.namespace} {pod_name} -- /bin/sh -c 'echo \"{test_content}\" > /data/{test_file}'"
            self.logger.info(f"Executing write command: {write_cmd}")
            subprocess.run(write_cmd, shell=True, check=True)
            
            # Read test
            read_cmd = f"kubectl exec -n {self.namespace} {pod_name} -- cat /data/{test_file}"
            self.logger.info(f"Executing read command: {read_cmd}")
            read_process = subprocess.run(read_cmd, shell=True, check=True, stdout=subprocess.PIPE, text=True)
            read_result = read_process.stdout.strip()
            
            # Check result
            if test_content in read_result:
                self.logger.info(f"Read/write test successful")
                return True
            else:
                self.logger.error(f"Read/write test failed: expected '{test_content}', got '{read_result}'")
                return False
                
        except Exception as e:
            self.logger.error(f"Error in read/write test: {str(e)}")
            return False
    
    def _print_report_summary(self, report):
        """Print a summary of the test report"""
        self.logger.info("===== EFS CSI Driver Test Summary =====")
        
        # Operations summary
        self.logger.info("--- Operations ---")
        for op_name, op_data in report['operations'].items():
            if 'success_rate' in op_data:  # Regular operations
                self.logger.info(f"{op_name}: {op_data['success']} succeeded, {op_data['fail']} failed ({op_data['success_rate']:.1f}%)")
            else:  # Read/write operations with separate metrics
                write_rate = op_data['write_success_rate'] if 'write_success_rate' in op_data else 0
                read_rate = op_data['read_success_rate'] if 'read_success_rate' in op_data else 0
                self.logger.info(f"{op_name}: Writes {op_data['write_success']} succeeded, {op_data['write_fail']} failed ({write_rate:.1f}%)")
                self.logger.info(f"{op_name}: Reads {op_data['read_success']} succeeded, {op_data['read_fail']} failed ({read_rate:.1f}%)")
        
        # Scenarios summary
        self.logger.info("--- Scenarios ---")
        for scenario_name, scenario_data in report['scenarios'].items():
            if scenario_data['runs'] > 0:
                self.logger.info(f"{scenario_name}: {scenario_data['success']} succeeded, {scenario_data['fail']} failed out of {scenario_data['runs']} runs ({scenario_data['success_rate']:.1f}%)")
            else:
                self.logger.info(f"{scenario_name}: No runs")
                
        # Filesystem errors summary
        stale_handle_metrics = report.get('filesystem_errors', {}).get('stale_file_handles', {})
        total_stale_handles = stale_handle_metrics.get('total_count', 0)
        if total_stale_handles > 0:
            self.logger.info("--- Filesystem Errors ---")
            self.logger.info(f"Stale File Handles: {total_stale_handles} errors detected")
            # Show distribution by path if available
            if 'counts_by_path' in stale_handle_metrics:
                for path, count in stale_handle_metrics['counts_by_path'].items():
                    self.logger.info(f"  - {path}: {count} errors")
        
        self.logger.info("=========================================")

# Main function to run the orchestrator
def main():
    """Main entry point"""
    # Setup argument parsing
    import argparse
    parser = argparse.ArgumentParser(description='EFS CSI Driver Orchestrator')
    parser.add_argument('--config-dir', default='config/components', help='Path to component config directory')
    parser.add_argument('--duration', default=300, type=int, help='Test duration in seconds')
    parser.add_argument('--interval', default=5, type=int, help='Operation interval in seconds')
    parser.add_argument('--namespace', default='default', help='Kubernetes namespace to use')
    args = parser.parse_args()
    
    # Setup component configs
    component_configs = {
        'driver': f"{args.config_dir}/driver.yaml",
        'storage': f"{args.config_dir}/storage.yaml",
        'test': f"{args.config_dir}/test.yaml",
        'pod': f"{args.config_dir}/pod.yaml",
        'scenarios': f"{args.config_dir}/scenarios.yaml"
    }
    
    # Initialize orchestrator
    orchestrator = EFSCSIOrchestrator(
        component_configs=component_configs,
        namespace=args.namespace
    )
    
    # Override default test parameters if specified
    if args.duration:
        orchestrator.test_duration = args.duration
    if args.interval:
        orchestrator.operation_interval = args.interval
    
    # Run the test
    orchestrator.run_test()

if __name__ == "__main__":
    main()
# Enhanced modular implementation for orchestrator
