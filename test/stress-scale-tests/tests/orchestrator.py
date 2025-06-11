#!/usr/bin/env python3

import random
import time
import yaml
import logging
import uuid
import os
from kubernetes import client, config
from datetime import datetime
from utils.log_integration import collect_logs_on_test_failure

class EFSCSIOrchestrator:
    """Orchestrator for testing EFS CSI driver operations"""
    
    def __init__(self, config_file='config/orchestrator_config.yaml', namespace=None, metrics_collector=None, driver_pod_name=None):
        """Initialize the orchestrator with configuration"""
        # Store driver pod name for log collection
        self.driver_pod_name = driver_pod_name
        
        # Load configuration
        with open(config_file, 'r') as f:
            self.config = yaml.safe_load(f)
        
        # Initialize Kubernetes client
        config.load_kube_config()
        self.core_v1 = client.CoreV1Api()
        self.apps_v1 = client.AppsV1Api()
        self.storage_v1 = client.StorageV1Api()
        
        # Initialize metrics collector if provided, or create a new one
        from utils.metrics_collector import MetricsCollector
        self.metrics_collector = metrics_collector or MetricsCollector()
        
        # Set namespace from config or use default
        self.namespace = namespace or self.config['test'].get('namespace', 'default')
        
        # Set up logging based on configuration
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
        
        # Test parameters
        self.test_duration = self.config['test'].get('duration', 3600)  # seconds
        self.operation_interval = self.config['test'].get('operation_interval', 3)  # seconds
        
        # Resource limits
        resource_limits = self.config.get('resource_limits', {})
        self.max_pvcs = resource_limits.get('max_pvcs', 100)
        self.max_pods_per_pvc = resource_limits.get('max_pods_per_pvc', 50)
        self.total_max_pods = resource_limits.get('total_max_pods', 30)
        
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
        
        self.logger.info("EFS CSI Orchestrator initialized")
        
        # Create namespace if it doesn't exist
        self._ensure_namespace_exists()
        
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
    
    def run_test(self):
        """
        Run the orchestrator test by randomly selecting operations
        until the test duration is reached
        """
        self.logger.info(f"Starting orchestrator test for {self.test_duration} seconds")
        start_time = time.time()
        
        # Ensure storage class exists
        self._ensure_storage_class()
        
        # Get operation weights from config or use defaults
        weights = self.config.get('operation_weights', {})
        operations = [
            (self._create_pvc, weights.get('create_pvc', 25)),
            (self._attach_pod, weights.get('attach_pod', 25)),
            (self._delete_pod, weights.get('delete_pod', 20)),
            (self._delete_pvc, weights.get('delete_pvc', 15)),
            (self._verify_readwrite, weights.get('verify_readwrite', 15)),
            (self._run_specific_scenario, weights.get('run_specific_scenario', 20))
        ]
        
        # Extract operations and weights into separate lists for cleaner selection
        operation_funcs, weights = zip(*operations)
        
        # Pre-calculate cumulative weights
        cumulative_weights = []
        current_sum = 0
        for weight in weights:
            current_sum += weight
            cumulative_weights.append(current_sum)
        total_weight = cumulative_weights[-1]
        
        # First, ensure we run all operations at least once in a controlled sequence
        self.logger.info("Running each operation type once to ensure coverage")
        
        # First create some PVCs
        self._create_pvc()
        self._create_pvc()
        
        # Then attach pods to those PVCs to create pairs
        self._attach_pod()
        self._attach_pod()
        self._attach_pod()  # One more to ensure we have multiple pods per PVC
        
        # Now we can run verify_readwrite which requires at least 2 pods on same PVC
        self._verify_readwrite()
        
        # Run a specific scenario
        self._run_specific_scenario()
        
        # Delete a pod
        self._delete_pod()
        
        # Delete a PVC
        self._delete_pvc()
        
        self.logger.info("Completed initial operation sequence, continuing with randomized operations")
        
        # Track operation selections to verify distribution
        operation_counts = {op.__name__: 0 for op, _ in operations}
        
        # Run operations randomly until test_duration is reached
        try:
            while time.time() - start_time < self.test_duration:
                # Select a random operation based on pre-calculated cumulative weights
                random_val = random.uniform(0, total_weight)
                
                # Find the operation corresponding to the random value
                for i, (operation, _) in enumerate(operations):
                    if random_val <= cumulative_weights[i]:
                        op_name = operation.__name__
                        operation_counts[op_name] = operation_counts.get(op_name, 0) + 1
                        self.logger.info(f"Selected operation: {op_name} (selected {operation_counts[op_name]} times)")
                        operation()
                        break
                
                # Wait between operations to avoid overwhelming the system
                time.sleep(self.operation_interval)
        
        except KeyboardInterrupt:
            self.logger.info("Test interrupted by user")
        except Exception as e:
            self.logger.error(f"Unexpected error during test: {e}", exc_info=True)
            
            # Collect logs for unexpected failure with comprehensive resource tracking
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            test_name = f"orchestrator_unexpected_failure_{timestamp}"
            
            # Gather information about all active resources for diagnostic logging
            failed_resources = []
            
            # Add all PVCs currently being tracked
            for pvc_name in self.pvcs:
                failed_resources.append({"type": "pvc", "name": pvc_name, "namespace": self.namespace})
                
                # Add all pods associated with each PVC
                for pod_name in self.pods.get(pvc_name, []):
                    failed_resources.append({"type": "pod", "name": pod_name, "namespace": self.namespace})
            
            logs_path = collect_logs_on_test_failure(
                test_name, 
                self.metrics_collector, 
                self.driver_pod_name,
                failed_resources=failed_resources
            )
            self.logger.info(f"Collected comprehensive failure logs to: {logs_path}")
        finally:
            # Get test duration
            elapsed = time.time() - start_time
            self.logger.info(f"Test completed in {elapsed:.2f} seconds")
            
            # Clean up resources
            self._cleanup()
            
            # Return test results
            return self._generate_report()
    
    def _create_pvc(self):
        """Create a PVC with access point"""
        # Check if we've reached the maximum PVC count
        if len(self.pvcs) >= self.max_pvcs:
            self.logger.info("Maximum PVC count reached, skipping creation")
            return
            
        pvc_name = f"test-pvc-{uuid.uuid4().hex[:8]}"
        self.logger.info(f"Creating PVC: {pvc_name}")
        
        try:
            # Create PVC manifest
            pvc_manifest = {
                "apiVersion": "v1",
                "kind": "PersistentVolumeClaim",
                "metadata": {"name": pvc_name},
                "spec": {
                    "accessModes": ["ReadWriteMany"],
                    "storageClassName": "efs-sc",
                    "resources": {
                        "requests": {"storage": "1Gi"}
                    }
                }
            }
            
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
            self.logger.info(f"Created PVC: {pvc_name}")
            
            # Wait for PVC to be bound
            if not self._wait_for_pvc_bound(pvc_name, timeout=30):
                self.logger.warning(f"Timeout waiting for PVC {pvc_name} to be bound")
            
        except Exception as e:
            self.results['create_pvc']['fail'] += 1
            self.logger.error(f"Failed to create PVC: {e}")
    
    def _attach_pod(self, pvc_name=None):
        """
        Attach a pod to a PVC
        If pvc_name is provided, attach to that specific PVC
        Otherwise, select a random PVC
        """
        # If no PVCs exist, skip
        if not self.pvcs:
            self.logger.info("No PVCs available, skipping pod attachment")
            return None
        
        # If we've reached max total pod count, skip
        if self.current_pod_count >= self.total_max_pods:
            self.logger.info("Maximum total pod count reached, skipping attachment")
            return None
        
        # Select PVC (either specified or random)
        if pvc_name is None or pvc_name not in self.pvcs:
            pvc_name = random.choice(self.pvcs)
        
        # Check if this PVC has reached max pods
        if len(self.pods[pvc_name]) >= self.max_pods_per_pvc:
            self.logger.info(f"PVC {pvc_name} has reached max pods ({self.max_pods_per_pvc}), skipping")
            return None
            
        pod_name = f"test-pod-{uuid.uuid4().hex[:8]}"
        self.logger.info(f"Creating pod {pod_name} on PVC {pvc_name}")
        
        try:
            # Get pod configuration from config
            pod_config = self.config.get('pod_config', {})
            
            # Create better startup script with diagnostic capabilities
            startup_script = """
#!/bin/sh
# Diagnostic information collection
echo "Pod $(hostname) starting up"
echo "Attempting to access /data directory"
ls -la /data || echo "ERROR: Cannot access /data directory"

# Try to create readiness file with retries
for i in 1 2 3 4 5; do
    echo "Attempt $i to create readiness file"
    if touch /data/pod-ready; then
        echo "Successfully created /data/pod-ready"
        break
    else
        echo "Failed to create readiness file on attempt $i"
        if [ $i -eq 5 ]; then
            echo "All attempts failed, creating alternative readiness file"
            # Create alternative readiness file that doesn't depend on volume mount
            mkdir -p /tmp/ready && touch /tmp/ready/pod-ready
        fi
        sleep 2
    fi
done

# Stay alive
while true; do
    sleep 30
done
"""
            # Create pod manifest with enhanced readiness check and debugging
            pod_manifest = {
                "apiVersion": "v1",
                "kind": "Pod",
                "metadata": {
                    "name": pod_name,
                    "labels": {
                        "app": "efs-test",
                        "component": "stress-test"
                    }
                },
                "spec": {
                    "containers": [{
                        "name": "test-container",
                        "image": pod_config.get('image', 'alpine:latest'),
                        "command": ["/bin/sh", "-c"],
                        "args": [startup_script],
                        "volumeMounts": [{
                            "name": "efs-volume",
                            "mountPath": "/data"
                        }],
                        "readinessProbe": {
                            "exec": {
                                "command": ["/bin/sh", "-c", "cat /data/pod-ready 2>/dev/null || cat /tmp/ready/pod-ready 2>/dev/null"]
                            },
                            "initialDelaySeconds": 15,
                            "periodSeconds": 5,
                            "failureThreshold": 6,
                            "timeoutSeconds": 5
                        },
                        "resources": {
                            "requests": {
                                "cpu": "100m",
                                "memory": "64Mi"
                            },
                            "limits": {
                                "cpu": "200m",
                                "memory": "128Mi"
                            }
                        }
                    }],
                    "volumes": [{
                        "name": "efs-volume",
                        "persistentVolumeClaim": {
                            "claimName": pvc_name
                        }
                    }],
                    "tolerations": [{
                        "key": "node.kubernetes.io/not-ready",
                        "operator": "Exists",
                        "effect": "NoExecute",
                        "tolerationSeconds": 300
                    }, {
                        "key": "node.kubernetes.io/unreachable",
                        "operator": "Exists",
                        "effect": "NoExecute",
                        "tolerationSeconds": 300
                    }]
                }
            }
            
            # Add any additional tolerations from config
            if 'tolerations' in pod_config:
                pod_manifest['spec']['tolerations'].extend(pod_config['tolerations'])
            
            # Add node selector only if explicitly defined in config
            # Otherwise, let Kubernetes schedule freely
            if 'node_selector' in pod_config:
                pod_manifest['spec']['nodeSelector'] = pod_config['node_selector']
            
            # Create pod in Kubernetes
            self.core_v1.create_namespaced_pod(
                namespace=self.namespace,
                body=pod_manifest
            )
            
            # Track the pod
            self.pods[pvc_name].append(pod_name)
            self.current_pod_count += 1
            
            # Update results
            self.results['attach_pod']['success'] += 1
            self.logger.info(f"Created pod: {pod_name} using PVC: {pvc_name}")
            
            # Wait for pod to be ready
            if not self._wait_for_pod_ready(pod_name, timeout=60):
                self.logger.warning(f"Timeout waiting for pod {pod_name} to be ready")
                return None
            
            return pod_name
            
        except Exception as e:
            self.results['attach_pod']['fail'] += 1
            self.logger.error(f"Failed to create pod: {e}")
            return None
    
    def _delete_pod(self, pod_name=None, pvc_name=None, force=False):
        """
        Delete a pod
        If pod_name and pvc_name are provided, delete that specific pod
        Otherwise, select a random pod
        
        Args:
            pod_name: Name of pod to delete
            pvc_name: Name of PVC associated with pod
            force: If True, use force deletion with grace period 0
        """
        # Find all pods if not specified
        if pod_name is None or pvc_name is None:
            all_pods = []
            for pvc, pod_list in self.pods.items():
                all_pods.extend([(pvc, pod) for pod in pod_list])
                
            if not all_pods:
                self.logger.info("No pods to delete")
                return False
                
            # Pick a random pod
            pvc_name, pod_name = random.choice(all_pods)
        elif pod_name not in self.pods.get(pvc_name, []):
            self.logger.warning(f"Pod {pod_name} not found in PVC {pvc_name}")
            return False
        
        self.logger.info(f"Deleting pod: {pod_name} from PVC: {pvc_name}")
        
        try:
            # Set up delete options
            if force:
                # Force delete with grace period 0 seconds
                grace_period_seconds = 0
                propagation_policy = 'Background'
                self.logger.info(f"Force deleting pod {pod_name} with grace period 0")
            else:
                # Normal delete with default grace period (usually 30s)
                grace_period_seconds = None
                propagation_policy = 'Foreground'
            
            # Delete the pod
            self.core_v1.delete_namespaced_pod(
                name=pod_name,
                namespace=self.namespace,
                grace_period_seconds=grace_period_seconds,
                propagation_policy=propagation_policy
            )
            
            # Wait for the pod to be deleted
            if not self._wait_for_pod_deleted(pod_name):
                self.logger.warning(f"Timeout waiting for pod {pod_name} to be deleted")
                return False
            
            # Remove from tracking
            if pod_name in self.pods[pvc_name]:
                self.pods[pvc_name].remove(pod_name)
                self.current_pod_count -= 1
            
            # Update results
            self.results['delete_pod']['success'] += 1
            self.logger.info(f"Deleted pod: {pod_name}")
            return True
            
        except Exception as e:
            self.results['delete_pod']['fail'] += 1
            self.logger.error(f"Failed to delete pod {pod_name}: {e}")
            return False
    
    def _delete_pvc(self, pvc_name=None, force=False):
        """
        Delete a PVC
        If pvc_name is provided, delete that specific PVC
        Otherwise, select a random PVC
        
        Args:
            pvc_name: Name of PVC to delete
            force: If True, use force deletion with grace period 0
        """
        if not self.pvcs:
            self.logger.info("No PVCs to delete")
            return False
            
        # Pick a PVC if not specified
        if pvc_name is None or pvc_name not in self.pvcs:
            pvc_name = random.choice(self.pvcs)
        
        self.logger.info(f"Deleting PVC: {pvc_name}")
        
        # Ensure no pods are using it
        if self.pods[pvc_name]:
            # Delete all pods using this PVC
            self.logger.info(f"Deleting {len(self.pods[pvc_name])} pods using PVC {pvc_name}")
            for pod_name in list(self.pods[pvc_name]):  # Create a copy of the list to avoid modification during iteration
                self._delete_pod(pod_name, pvc_name)
        
        try:
            # Set up delete options
            if force:
                # Force delete with grace period 0 seconds
                grace_period_seconds = 0
                propagation_policy = 'Background'
                self.logger.info(f"Force deleting PVC {pvc_name} with grace period 0")
            else:
                # Normal delete with default grace period
                grace_period_seconds = None
                propagation_policy = 'Foreground'
                
            # Delete the PVC
            self.core_v1.delete_namespaced_persistent_volume_claim(
                name=pvc_name,
                namespace=self.namespace,
                grace_period_seconds=grace_period_seconds,
                propagation_policy=propagation_policy
            )
            
            # Wait for the PVC to be deleted
            if not self._wait_for_pvc_deleted(pvc_name):
                self.logger.warning(f"Timeout waiting for PVC {pvc_name} to be deleted")
                return False
            
            # Remove from tracking
            if pvc_name in self.pvcs:
                self.pvcs.remove(pvc_name)
            if pvc_name in self.pods:
                del self.pods[pvc_name]
            
            # Update results
            self.results['delete_pvc']['success'] += 1
            self.logger.info(f"Deleted PVC: {pvc_name}")
            return True
            
        except Exception as e:
            self.results['delete_pvc']['fail'] += 1
            self.logger.error(f"Failed to delete PVC {pvc_name}: {e}")
            return False
    
    def _verify_readwrite(self):
        """
        Verify read/write operations between pods sharing a PVC
        This tests that pods sharing the same volume can see each other's writes
        
        Note: Using kubectl subprocesses instead of Kubernetes API to avoid
        WebSocket upgrade errors
        
        Also collects performance metrics for file operations.
        """
        # Find PVCs that have multiple pods
        shared_pvcs = [(pvc, pods) for pvc, pods in self.pods.items() if len(pods) >= 2]
        
        if not shared_pvcs:
            self.logger.info("No shared PVCs with multiple pods for read/write test")
            return
            
        # Pick a random shared PVC
        pvc_name, pod_names = random.choice(shared_pvcs)
        if len(pod_names) < 2:
            return
            
        # Pick two distinct pods
        writer_pod = random.choice(pod_names)
        reader_pod = random.choice([p for p in pod_names if p != writer_pod])
        
        test_file = f"test-{uuid.uuid4().hex[:8]}.txt"
        test_content = f"Test content: {uuid.uuid4()}" * 50  # Make content larger for better measurements
        content_size_bytes = len(test_content.encode('utf-8'))
        
        self.logger.info(f"Testing read/write between pods {writer_pod} and {reader_pod} sharing PVC {pvc_name}")
        self.logger.info(f"File size: {content_size_bytes} bytes")
        
        try:
            import subprocess
            
            # Track write operation with metrics
            write_op_start = time.time()
            
            # Write with first pod using kubectl
            write_cmd = f"kubectl exec -n {self.namespace} {writer_pod} -- /bin/sh -c 'echo \"{test_content}\" > /data/{test_file}'"
            self.logger.info(f"Executing write command: {write_cmd}")
            
            write_process = subprocess.run(
                write_cmd,
                shell=True,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            
            # Measure write operation duration
            write_duration = time.time() - write_op_start
            write_ops_count = 1  # One write operation
            
            # Track metrics for write operation
            self.metrics_collector.track_file_operation_latency(pvc_name, "write", write_duration)
            self.metrics_collector.track_file_operation_iops(pvc_name, "write", write_ops_count, write_duration)
            self.metrics_collector.track_file_operation_throughput(pvc_name, "write", content_size_bytes, write_duration)
            
            self.logger.info(f"Write operation completed in {write_duration:.3f}s")
            self.logger.info(f"Write throughput: {(content_size_bytes / 1024 / 1024) / write_duration:.2f} MB/s")
            
            # Update results
            self.results['verify_write']['success'] += 1
            self.logger.info(f"Pod {writer_pod} wrote to /data/{test_file}")
            
            # Sleep briefly to ensure the write completes and is visible
            time.sleep(2)
            
            # Track read operation with metrics
            read_op_start = time.time()
            
            # Read with second pod using kubectl
            read_cmd = f"kubectl exec -n {self.namespace} {reader_pod} -- cat /data/{test_file}"
            self.logger.info(f"Executing read command: {read_cmd}")
            
            read_process = subprocess.run(
                read_cmd,
                shell=True,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            
            # Measure read operation duration
            read_duration = time.time() - read_op_start
            read_ops_count = 1  # One read operation
            
            # Track metrics for read operation
            self.metrics_collector.track_file_operation_latency(pvc_name, "read", read_duration)
            self.metrics_collector.track_file_operation_iops(pvc_name, "read", read_ops_count, read_duration)
            self.metrics_collector.track_file_operation_throughput(pvc_name, "read", content_size_bytes, read_duration)
            
            resp = read_process.stdout.strip()
            
            self.logger.info(f"Read operation completed in {read_duration:.3f}s")
            self.logger.info(f"Read throughput: {(content_size_bytes / 1024 / 1024) / read_duration:.2f} MB/s")
            self.logger.info(f"Read result length: {len(resp)} bytes")
            
            # Verify content
            if test_content in resp:
                self.results['verify_read']['success'] += 1
                self.logger.info(f"Pod {reader_pod} successfully read content written by {writer_pod}")
                
                # Update scenario tracking
                self.scenarios['shared_volume_rw']['runs'] += 1
                self.scenarios['shared_volume_rw']['success'] += 1
                
                # Add metadata operation metrics - perform ls to measure metadata operations
                meta_op_start = time.time()
                ls_cmd = f"kubectl exec -n {self.namespace} {reader_pod} -- ls -la /data/"
                
                ls_process = subprocess.run(
                    ls_cmd,
                    shell=True,
                    check=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True
                )
                
                meta_duration = time.time() - meta_op_start
                meta_ops_count = 1  # One metadata operation
                
                # Track metrics for metadata operation
                self.metrics_collector.track_file_operation_latency(pvc_name, "metadata", meta_duration)
                self.metrics_collector.track_file_operation_iops(pvc_name, "metadata", meta_ops_count, meta_duration)
                
                self.logger.info(f"Metadata operation (ls) completed in {meta_duration:.3f}s")
                
            else:
                self.results['verify_read']['fail'] += 1
                self.scenarios['shared_volume_rw']['runs'] += 1
                self.scenarios['shared_volume_rw']['fail'] += 1
                self.logger.error(f"Pod {reader_pod} failed to read content written by {writer_pod}. Got different content length: {len(resp)} vs expected {len(test_content)}")
            
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Command execution failed: {e}")
            self.logger.error(f"Command stderr: {e.stderr}")
            if 'write_process' not in locals():
                self.results['verify_write']['fail'] += 1
            else:
                self.results['verify_read']['fail'] += 1
            
            # Update scenario tracking
            self.scenarios['shared_volume_rw']['runs'] += 1
            self.scenarios['shared_volume_rw']['fail'] += 1
        except Exception as e:
            self.logger.error(f"Failed in read/write verification: {e}")
            if 'write_process' not in locals():
                self.results['verify_write']['fail'] += 1
            else:
                self.results['verify_read']['fail'] += 1
            
            # Update scenario tracking
            self.scenarios['shared_volume_rw']['runs'] += 1
            self.scenarios['shared_volume_rw']['fail'] += 1
    
    def _run_specific_scenario(self):
        """
        Run a specific test scenario
        Randomly select from the required scenarios
        """
        scenarios = [
            self._scenario_many_to_one,
            self._scenario_one_to_one, 
            self._scenario_concurrent_pvc
        ]
        
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
        
        # Create PVC
        pvc_name = f"many2one-{uuid.uuid4().hex[:8]}"
        self.logger.info(f"[MANY2ONE] STEP 1: Creating dedicated PVC: {pvc_name}")
        
        try:
            # Create PVC manifest
            pvc_manifest = {
                "apiVersion": "v1",
                "kind": "PersistentVolumeClaim",
                "metadata": {"name": pvc_name},
                "spec": {
                    "accessModes": ["ReadWriteMany"],
                    "storageClassName": "efs-sc",
                    "resources": {
                        "requests": {"storage": "1Gi"}
                    }
                }
            }
            
            # Create PVC
            self.logger.info(f"[MANY2ONE] Creating PVC manifest with storageClassName: {pvc_manifest['spec']['storageClassName']}")
            self.core_v1.create_namespaced_persistent_volume_claim(
                namespace=self.namespace,
                body=pvc_manifest
            )
            
            # Track the PVC
            self.pvcs.append(pvc_name)
            self.pods[pvc_name] = []
            self.logger.info(f"[MANY2ONE] PVC {pvc_name} created successfully")
            
            # Wait for PVC to be bound
            self.logger.info(f"[MANY2ONE] Waiting for PVC {pvc_name} to be bound...")
            if not self._wait_for_pvc_bound(pvc_name, timeout=30):
                self.logger.error(f"[MANY2ONE] FAILED: Timeout waiting for PVC {pvc_name} to be bound")
                self.scenarios['many_to_one']['fail'] += 1
                return
            
            # Get PVC status to verify it's bound correctly
            try:
                pvc_status = self.core_v1.read_namespaced_persistent_volume_claim_status(
                    name=pvc_name,
                    namespace=self.namespace
                )
                self.logger.info(f"[MANY2ONE] PVC status: phase={pvc_status.status.phase}, capacity={pvc_status.status.capacity}")
            except Exception as e:
                self.logger.warning(f"[MANY2ONE] Could not get PVC status: {e}")
            
            # Create multiple pods (3-5)
            num_pods = random.randint(3, 5)
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
            
            if len(pod_names) < 2:
                self.logger.error(f"[MANY2ONE] FAILED: Insufficient pods created ({len(pod_names)}), need at least 2 for read/write test")
                self.scenarios['many_to_one']['fail'] += 1
                return
            
            # Test read/write between two pods
            test_file = f"many2one-{uuid.uuid4().hex[:8]}.txt"
            test_content = f"Many2One test content: {uuid.uuid4()}"
            
            # Select two random pods
            writer_pod = random.choice(pod_names)
            reader_pod = random.choice([p for p in pod_names if p != writer_pod])
            
            self.logger.info(f"[MANY2ONE] STEP 3: Testing read/write operations")
            self.logger.info(f"[MANY2ONE] Writer pod: {writer_pod}, Reader pod: {reader_pod}")
            self.logger.info(f"[MANY2ONE] Test file: /data/{test_file}")
            self.logger.info(f"[MANY2ONE] Test content: {test_content}")
            
            import subprocess
            
            try:
                # Write with first pod using kubectl subprocess
                write_cmd = f"kubectl exec -n {self.namespace} {writer_pod} -- /bin/sh -c 'echo \"{test_content}\" > /data/{test_file}'"
                self.logger.info(f"[MANY2ONE] Executing write command: {write_cmd}")
                
                write_process = subprocess.run(
                    write_cmd, 
                    shell=True, 
                    check=True,
                    stdout=subprocess.PIPE, 
                    stderr=subprocess.PIPE,
                    text=True
                )
                
                # Verify the file exists in writer pod
                ls_cmd = f"kubectl exec -n {self.namespace} {writer_pod} -- ls -la /data/{test_file}"
                ls_process = subprocess.run(
                    ls_cmd,
                    shell=True,
                    check=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True
                )
                self.logger.info(f"[MANY2ONE] Writer pod ls check: '{ls_process.stdout.strip()}'")
                
                # Sleep to ensure filesystem sync
                self.logger.info(f"[MANY2ONE] Sleeping for 5 seconds to ensure filesystem sync")
                time.sleep(5)
                
                # Read with second pod using kubectl subprocess
                read_cmd = f"kubectl exec -n {self.namespace} {reader_pod} -- cat /data/{test_file}"
                self.logger.info(f"[MANY2ONE] Executing read command: {read_cmd}")
                
                read_process = subprocess.run(
                    read_cmd,
                    shell=True,
                    check=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True
                )
                
                read_result = read_process.stdout.strip()
                self.logger.info(f"[MANY2ONE] Read command result: '{read_result}'")
                
                # Verify content
                if test_content in read_result:
                    self.logger.info(f"[MANY2ONE] SUCCESS: Many-to-one scenario successful with {len(pod_names)} pods")
                    self.scenarios['many_to_one']['success'] += 1
                else:
                    self.logger.error(f"[MANY2ONE] FAILED: Pods cannot share data - Expected '{test_content}', got '{read_result}'")
                    
                    # Check mount info using kubectl subprocess
                    mount_cmd = f"kubectl exec -n {self.namespace} {writer_pod} -- mount | grep /data"
                    mount_process = subprocess.run(
                        mount_cmd,
                        shell=True,
                        check=True,
                        stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE,
                        text=True
                    )
                    self.logger.info(f"[MANY2ONE] Writer pod mount info: '{mount_process.stdout.strip()}'")
                    
                    reader_mount_cmd = f"kubectl exec -n {self.namespace} {reader_pod} -- mount | grep /data"
                    reader_mount_process = subprocess.run(
                        reader_mount_cmd,
                        shell=True,
                        check=True,
                        stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE,
                        text=True
                    )
                    self.logger.info(f"[MANY2ONE] Reader pod mount info: '{reader_mount_process.stdout.strip()}'")
                    
                    self.scenarios['many_to_one']['fail'] += 1
                    # Collect logs for failure diagnostics with detailed information about failed resources
                    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
                    test_name = f"many2one_failure_{timestamp}"
                    
                    # Track failed resources for detailed logging
                    failed_resources = [
                        # Add any pods involved in the scenario
                        {"type": "pod", "name": writer_pod, "namespace": self.namespace},
                        {"type": "pod", "name": reader_pod, "namespace": self.namespace},
                        # Add the PVC
                        {"type": "pvc", "name": pvc_name, "namespace": self.namespace}
                    ]
                    
                    logs_path = collect_logs_on_test_failure(
                        test_name, 
                        self.metrics_collector, 
                        self.driver_pod_name,
                        failed_resources=failed_resources
                    )
                    self.logger.info(f"Collected detailed failure logs to: {logs_path}")
            
            except subprocess.CalledProcessError as e:
                self.logger.error(f"[MANY2ONE] Command execution failed: {e}")
                self.logger.error(f"[MANY2ONE] Command stderr: {e.stderr}")
                self.scenarios['many_to_one']['fail'] += 1
            except Exception as e:
                self.logger.error(f"[MANY2ONE] FAILED: Error during read/write test: {e}")
                self.scenarios['many_to_one']['fail'] += 1
        
        except Exception as e:
            self.logger.error(f"[MANY2ONE] FAILED: Unhandled error in many-to-one scenario: {e}")
            self.scenarios['many_to_one']['fail'] += 1
        
        self.logger.info("+" * 80)
        self.logger.info("COMPLETED MANY-TO-ONE SCENARIO DIAGNOSTICS")
        self.logger.info("+" * 80)
        
        # Clean up (will be managed by the orchestrator's cleanup)
    
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
        
        num_pairs = random.randint(3, 5)
        self.logger.info(f"Creating {num_pairs} PVC-pod pairs")
        
        pairs = []
        
        try:
            # Create multiple PVC-pod pairs
            for i in range(num_pairs):
                # Create PVC
                pvc_name = f"one2one-{uuid.uuid4().hex[:8]}"
                
                # Create PVC manifest
                pvc_manifest = {
                    "apiVersion": "v1",
                    "kind": "PersistentVolumeClaim",
                    "metadata": {"name": pvc_name},
                    "spec": {
                        "accessModes": ["ReadWriteMany"],
                        "storageClassName": "efs-sc",
                        "resources": {
                            "requests": {"storage": "1Gi"}
                        }
                    }
                }
                
                # Create PVC
                self.core_v1.create_namespaced_persistent_volume_claim(
                    namespace=self.namespace,
                    body=pvc_manifest
                )
                
                # Track the PVC
                self.pvcs.append(pvc_name)
                self.pods[pvc_name] = []
                
                # Wait for PVC to be bound
                if not self._wait_for_pvc_bound(pvc_name, timeout=30):
                    self.logger.warning(f"Timeout waiting for PVC {pvc_name} to be bound")
                    continue
                
                # Create a pod for this PVC
                pod_name = self._attach_pod(pvc_name)
                if pod_name:
                    pairs.append((pvc_name, pod_name))
            
            if len(pairs) < 2:
                self.logger.warning(f"Failed to create enough PVC-pod pairs, only created {len(pairs)}")
                self.scenarios['one_to_one']['fail'] += 1
                return
            
            # Test that each pod can write to its own volume using kubectl subprocess
            import subprocess
            success = True
            
            for pvc_name, pod_name in pairs:
                test_file = f"one2one-{uuid.uuid4().hex[:8]}.txt"
                test_content = f"One2One test content for {pvc_name}: {uuid.uuid4()}"
                
                try:
                    # Write to file using kubectl
                    write_cmd = f"kubectl exec -n {self.namespace} {pod_name} -- /bin/sh -c 'echo \"{test_content}\" > /data/{test_file}'"
                    self.logger.info(f"[ONE2ONE] Executing write command for pod {pod_name}: {write_cmd}")
                    
                    write_process = subprocess.run(
                        write_cmd,
                        shell=True,
                        check=True,
                        stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE,
                        text=True
                    )
                    
                    # Read the file back to verify using kubectl
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
                        success = False
                        break
                    else:
                        self.logger.info(f"[ONE2ONE] Pod {pod_name} successfully wrote and read from its own volume")
                        
                except subprocess.CalledProcessError as e:
                    self.logger.error(f"[ONE2ONE] Command execution failed for pod {pod_name}: {e}")
                    self.logger.error(f"[ONE2ONE] Command stderr: {e.stderr}")
                    success = False
                    break
                except Exception as e:
                    self.logger.error(f"[ONE2ONE] Error in one-to-one scenario for pod {pod_name}: {e}")
                    success = False
                    break
            
            if success:
                self.logger.info(f"[ONE2ONE] One-to-one scenario successful with {len(pairs)} PVC-pod pairs")
                self.scenarios['one_to_one']['success'] += 1
            else:
                self.logger.error("[ONE2ONE] One-to-one scenario failed")
                self.scenarios['one_to_one']['fail'] += 1
                
                # Collect logs for failure diagnostics with detailed information
                timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
                test_name = f"one2one_failure_{timestamp}"
                
                # Track all failed resources for detailed logging
                failed_resources = []
                
                # Add all PVC-pod pairs
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
                
        except Exception as e:
            self.logger.error(f"[ONE2ONE] Unhandled error in one-to-one scenario: {e}")
            self.scenarios['one_to_one']['fail'] += 1
    
    def _scenario_concurrent_pvc(self):
        """
        Test rapid PVC creation and deletion
        1. Create multiple PVCs in quick succession
        2. Create pods for some of them
        3. Delete some PVCs in quick succession
        4. Verify operations complete successfully
        """
        self.logger.info("Running scenario: Rapid PVC operations")
        self.scenarios['concurrent_pvc']['runs'] += 1
        
        # Number of PVCs to create
        num_pvcs = random.randint(3, 7)
        self.logger.info(f"Creating {num_pvcs} PVCs in quick succession")
        
        pvc_names = [f"concurrent-pvc-{uuid.uuid4().hex[:8]}" for _ in range(num_pvcs)]
        created_pvcs = []
        
        try:
            # Create multiple PVCs in quick succession
            for pvc_name in pvc_names:
                success = self._create_pvc_for_concurrent(pvc_name)
                if success:
                    created_pvcs.append(pvc_name)
            
            if len(created_pvcs) < 2:
                self.logger.warning(f"Failed to create enough PVCs, only created {len(created_pvcs)}")
                self.scenarios['concurrent_pvc']['fail'] += 1
                return
            
            self.logger.info(f"Successfully created {len(created_pvcs)} PVCs")
            
            # Create pods for some of the PVCs
            num_pods = min(len(created_pvcs), 3)
            pod_pvcs = random.sample(created_pvcs, num_pods)
            
            for pvc_name in pod_pvcs:
                self._attach_pod(pvc_name)
            
            # Delete some PVCs in quick succession
            num_to_delete = min(len(created_pvcs), 3)
            pvcs_to_delete = random.sample(created_pvcs, num_to_delete)
            
            for pvc_name in pvcs_to_delete:
                self._delete_pvc(pvc_name)
            
            self.logger.info(f"Rapid PVC scenario completed")
            self.scenarios['concurrent_pvc']['success'] += 1
            
        except Exception as e:
            self.logger.error(f"Error in rapid PVC scenario: {e}")
            self.scenarios['concurrent_pvc']['fail'] += 1
            
            # Collect logs for failure diagnostics with detailed information
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            test_name = f"concurrent_pvc_failure_{timestamp}"
            
            # Track all resources involved in this scenario
            failed_resources = []
            
            # Add all created PVCs
            for pvc_name in created_pvcs:
                failed_resources.append({"type": "pvc", "name": pvc_name, "namespace": self.namespace})
                
                # Add pods using those PVCs
                for pod_name in self.pods.get(pvc_name, []):
                    failed_resources.append({"type": "pod", "name": pod_name, "namespace": self.namespace})
            
            logs_path = collect_logs_on_test_failure(
                test_name,
                self.metrics_collector, 
                self.driver_pod_name,
                failed_resources=failed_resources
            )
            self.logger.info(f"Collected detailed failure logs to: {logs_path}")
    
    def _create_pvc_for_concurrent(self, pvc_name):
        """
        Helper method for creating PVCs in concurrent scenario
        Returns True if successful, False otherwise
        """
        try:
            # Create PVC manifest
            pvc_manifest = {
                "apiVersion": "v1",
                "kind": "PersistentVolumeClaim",
                "metadata": {"name": pvc_name},
                "spec": {
                    "accessModes": ["ReadWriteMany"],
                    "storageClassName": "efs-sc",
                    "resources": {
                        "requests": {"storage": "1Gi"}
                    }
                }
            }
            
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
            self.logger.info(f"Created PVC: {pvc_name}")
            
            # Wait for PVC to be bound
            if not self._wait_for_pvc_bound(pvc_name, timeout=30):
                self.logger.warning(f"Timeout waiting for PVC {pvc_name} to be bound")
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
            try:
                pod = self.core_v1.read_namespaced_pod_status(
                    name=pod_name,
                    namespace=self.namespace
                )
                
                # Log phase transitions for better diagnostics
                if pod.status.phase != last_phase:
                    self.logger.info(f"Pod {pod_name} phase: {pod.status.phase}")
                    last_phase = pod.status.phase
                
                # Check if pod is ready
                if pod.status.phase == "Running":
                    if pod.status.conditions:
                        ready_condition = None
                        all_conditions = []
                        
                        for condition in pod.status.conditions:
                            all_conditions.append(f"{condition.type}={condition.status}")
                            if condition.type == "Ready":
                                ready_condition = condition
                        
                        # Log all conditions for diagnostics
                        self.logger.info(f"Pod {pod_name} conditions: {', '.join(all_conditions)}")
                        
                        if ready_condition and ready_condition.status == "True":
                            self.logger.info(f"Pod {pod_name} is ready")
                            return True
                
                # Check for failure states
                if pod.status.phase in ["Failed", "Unknown"]:
                    self.logger.error(f"Pod {pod_name} entered {pod.status.phase} state")
                    self._log_pod_diagnostics(pod_name)
                    return False
                    
                # Generate diagnostics only once during waiting if we're halfway through the timeout
                elapsed = time.time() - start_time
                if elapsed > timeout / 2 and not diagnostic_logged:
                    self.logger.info(f"Pod {pod_name} taking longer than expected to become ready ({elapsed:.1f}s). Collecting diagnostics...")
                    self._log_pod_diagnostics(pod_name)
                    diagnostic_logged = True
                    
                # Still waiting
                self.logger.debug(f"Pod {pod_name} is in {pod.status.phase} state, waiting...")
                
            except client.exceptions.ApiException as e:
                if e.status == 404:
                    self.logger.warning(f"Pod {pod_name} not found")
                    return False
                self.logger.warning(f"Error checking pod status: {e}")
            
            time.sleep(2)
        
        self.logger.warning(f"Timeout waiting for pod {pod_name} to be ready after {timeout}s")
        # Collect detailed diagnostics on timeout
        self._log_pod_diagnostics(pod_name)
        return False
        
    def _log_pod_diagnostics(self, pod_name):
        """
        Collect and log detailed pod diagnostics
        This helps diagnose why a pod isn't becoming ready
        """
        try:
            # Get detailed pod information
            self.logger.info(f"===== DIAGNOSTICS FOR POD {pod_name} =====")
            
            # 1. Get full pod details
            pod = self.core_v1.read_namespaced_pod(
                name=pod_name,
                namespace=self.namespace
            )
            
            # 2. Check container statuses
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
            
            # 3. Check pod events
            try:
                field_selector = f"involvedObject.name={pod_name}"
                events = self.core_v1.list_namespaced_event(
                    namespace=self.namespace,
                    field_selector=field_selector
                )
                
                if events.items:
                    self.logger.info(f"Pod events:")
                    for event in events.items:
                        self.logger.info(f"  [{event.last_timestamp}] {event.type}/{event.reason}: {event.message}")
                else:
                    self.logger.info("No events found for pod")
            except Exception as e:
                self.logger.warning(f"Error retrieving pod events: {e}")
            
            # 4. Get pod logs if container is running
            try:
                logs = self.core_v1.read_namespaced_pod_log(
                    name=pod_name,
                    namespace=self.namespace,
                    container="test-container",
                    tail_lines=20
                )
                if logs:
                    self.logger.info(f"Container logs (last 20 lines):")
                    for line in logs.splitlines()[-20:]:
                        self.logger.info(f"  {line}")
                else:
                    self.logger.info("No logs available")
            except Exception as e:
                self.logger.warning(f"Error retrieving pod logs: {e}")
            
            # 5. Check volume issues
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
            
            # 6. Try to run diagnostic command in pod if it's running
            if pod.status.phase == "Running":
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
            
            self.logger.info(f"===== END DIAGNOSTICS FOR POD {pod_name} =====")
            
        except Exception as e:
            self.logger.error(f"Error collecting pod diagnostics: {e}")
    
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
    
    def _cleanup(self):
        """Clean up all resources created during test with robust error handling"""
        self.logger.info("===== STARTING COMPREHENSIVE CLEANUP =====")
        cleanup_start_time = time.time()
        cleanup_timeout = 180  # 3 minutes timeout for entire cleanup
        
        # Track cleanup failures for reporting
        cleanup_failures = []
        
        # Force delete flag - initially false, but will be set to true for retry attempts
        force_delete = False
        
        try:
            # First attempt - normal deletion
            self._perform_cleanup(force_delete, cleanup_failures)
            
            # Check if there are any remaining resources that weren't cleaned up
            remaining_resources = self._get_remaining_resources()
            
            # If resources remain, try force delete
            if remaining_resources:
                self.logger.warning(f"First cleanup pass incomplete. Remaining resources: {remaining_resources}")
                self.logger.info("Attempting force deletion of remaining resources...")
                force_delete = True
                self._perform_cleanup(force_delete, cleanup_failures)
                
                # Check again for any remaining resources
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
    
    def _perform_cleanup(self, force=False, failures=None):
        """Perform the actual cleanup with optional force deletion"""
        if failures is None:
            failures = []
            
        # Delete all pods first
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
        
        # Wait briefly to allow pod termination before PVC deletion
        time.sleep(5)
        
        # Then delete all PVCs
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
        report = {
            "test_duration": time.time(),
            "operations": {
                "create_pvc": {
                    "success": self.results['create_pvc']['success'],
                    "fail": self.results['create_pvc']['fail'],
                    "success_rate": self._calculate_success_rate(self.results['create_pvc']),
                },
                "attach_pod": {
                    "success": self.results['attach_pod']['success'],
                    "fail": self.results['attach_pod']['fail'],
                    "success_rate": self._calculate_success_rate(self.results['attach_pod']),
                },
                "delete_pod": {
                    "success": self.results['delete_pod']['success'],
                    "fail": self.results['delete_pod']['fail'],
                    "success_rate": self._calculate_success_rate(self.results['delete_pod']),
                },
                "delete_pvc": {
                    "success": self.results['delete_pvc']['success'],
                    "fail": self.results['delete_pvc']['fail'],
                    "success_rate": self._calculate_success_rate(self.results['delete_pvc']),
                },
                "verify_read_write": {
                    "write_success": self.results['verify_write']['success'],
                    "write_fail": self.results['verify_write']['fail'],
                    "read_success": self.results['verify_read']['success'],
                    "read_fail": self.results['verify_read']['fail'],
                    "write_success_rate": self._calculate_success_rate(self.results['verify_write']),
                    "read_success_rate": self._calculate_success_rate(self.results['verify_read']),
                }
            },
            "scenarios": {
                "shared_volume_rw": {
                    "runs": self.scenarios['shared_volume_rw']['runs'],
                    "success": self.scenarios['shared_volume_rw']['success'],
                    "fail": self.scenarios['shared_volume_rw']['fail'],
                    "success_rate": self._calculate_scenario_success_rate('shared_volume_rw')
                },
                "many_to_one": {
                    "runs": self.scenarios['many_to_one']['runs'],
                    "success": self.scenarios['many_to_one']['success'],
                    "fail": self.scenarios['many_to_one']['fail'],
                    "success_rate": self._calculate_scenario_success_rate('many_to_one')
                },
                "one_to_one": {
                    "runs": self.scenarios['one_to_one']['runs'],
                    "success": self.scenarios['one_to_one']['success'],
                    "fail": self.scenarios['one_to_one']['fail'],
                    "success_rate": self._calculate_scenario_success_rate('one_to_one')
                },
                "concurrent_pvc": {
                    "runs": self.scenarios['concurrent_pvc']['runs'],
                    "success": self.scenarios['concurrent_pvc']['success'],
                    "fail": self.scenarios['concurrent_pvc']['fail'],
                    "success_rate": self._calculate_scenario_success_rate('concurrent_pvc')
                }
            }
        }
        
        # Print report summary
        self._print_report_summary(report)
        
        return report
    
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
        
        self.logger.info("=========================================")

# Main function to run the orchestrator
def main():
    """Main entry point"""
    # Setup argument parsing
    import argparse
    parser = argparse.ArgumentParser(description='EFS CSI Driver Orchestrator')
    parser.add_argument('--config', default='config/test_config.yaml', help='Path to config file')
    parser.add_argument('--duration', type=int, help='Test duration in seconds')
    parser.add_argument('--interval', type=int, help='Operation interval in seconds')
    parser.add_argument('--namespace', default='default', help='Kubernetes namespace to use')
    args = parser.parse_args()
    
    # Initialize and run orchestrator
    orchestrator = EFSCSIOrchestrator(config_file=args.config, namespace=args.namespace)
    
    # Override default test parameters if specified
    if args.duration:
        orchestrator.test_duration = args.duration
    if args.interval:
        orchestrator.operation_interval = args.interval
    
    # Run the test
    orchestrator.run_test()

if __name__ == "__main__":
    main()
