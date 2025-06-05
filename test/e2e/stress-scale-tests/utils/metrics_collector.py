import time
import psutil
import logging
import requests
import re
from datetime import datetime, timezone
from kubernetes import client, watch
from collections import defaultdict

class MetricsCollector:
    """Collect and store metrics during test execution"""
    
    def __init__(self):
        """Initialize metrics collector"""
        # Basic metrics structure
        self.operations = {}
        self.system_metrics = {}
        self.k8s_metrics = {}
        self.csi_metrics = {}
        
        # Controller metrics
        self.controller_metrics = {
            "request_latency": {},
            "operation_counts": defaultdict(int),
            "success_rates": defaultdict(lambda: {"success": 0, "failure": 0}),
            "volume_attach_timing": {}
        }
        
        # Node-level metrics
        self.node_metrics = {
            "mount_timing": {},
            "mount_errors": defaultdict(int),
            "resource_utilization": {}
        }
        
        # EFS-specific metrics
        self.efs_metrics = {
            "access_point_timing": {},
            "mount_completion_timing": {},
            "api_throttling_incidents": []
        }
        
        # Kubernetes events
        self.k8s_events = {
            "volume_events": [],
            "binding_times": {},
            "pod_startup_delays": {}
        }
        
        self.logger = logging.getLogger(__name__)
    
    def start_operation(self, name=None):
        """Start timing an operation
        
        Args:
            name: Name of the operation (optional)
        """
        op_id = name or f"op_{len(self.operations) + 1}"
        self.operations[op_id] = {
            "start_time": time.time(),
            "samples": []
        }
        self._collect_system_metrics(op_id)
        return op_id
    
    def end_operation(self, op_id):
        """End timing an operation and return duration
        
        Args:
            op_id: Operation ID or name
            
        Returns:
            Duration in seconds
        """
        if op_id not in self.operations:
            self.logger.warning(f"Operation {op_id} not found")
            return 0
            
        self.operations[op_id]["end_time"] = time.time()
        self.operations[op_id]["duration"] = (
            self.operations[op_id]["end_time"] - 
            self.operations[op_id]["start_time"]
        )
        self._collect_system_metrics(op_id, end=True)
        
        return self.operations[op_id]["duration"]
    
    def add_sample(self, op_id, metrics):
        """Add a sample to an operation
        
        Args:
            op_id: Operation ID or name
            metrics: Dictionary of metrics to add
        """
        if op_id not in self.operations:
            self.logger.warning(f"Operation {op_id} not found")
            return
            
        sample = {
            "timestamp": time.time(),
            "metrics": metrics
        }
        self.operations[op_id]["samples"].append(sample)
    
    def _collect_system_metrics(self, op_id, end=False):
        """Collect system metrics
        
        Args:
            op_id: Operation ID or name
            end: Whether this is the end of an operation
        """
        prefix = "end_" if end else "start_"
        
        # Collect CPU, memory, disk I/O metrics
        cpu_percent = psutil.cpu_percent(interval=0.1)
        memory = psutil.virtual_memory()
        disk_io = psutil.disk_io_counters()
        
        metrics = {
            f"{prefix}cpu_percent": cpu_percent,
            f"{prefix}memory_percent": memory.percent,
            f"{prefix}disk_read_bytes": disk_io.read_bytes,
            f"{prefix}disk_write_bytes": disk_io.write_bytes
        }
        
        if op_id not in self.system_metrics:
            self.system_metrics[op_id] = {}
            
        self.system_metrics[op_id].update(metrics)
    
    def collect_csi_metrics(self, config=None):
        """Collect CSI driver metrics
        
        Args:
            config: Configuration dictionary with metrics settings
        """
        if not config:
            return
        
        if not config.get('metrics_collection', {}).get('enabled', False):
            return
        
        ports = config.get('metrics_collection', {}).get('controller_ports', [8080, 8081])
        
        # Get EFS CSI controller pod
        try:
            kube_client = client.CoreV1Api()
            pods = kube_client.list_namespaced_pod(
                namespace="kube-system",
                label_selector="app=efs-csi-controller"
            )
            
            if not pods.items:
                self.logger.warning("No EFS CSI controller pods found")
                return
            
            controller_pod = pods.items[0]
            pod_name = controller_pod.metadata.name
            
            # Port-forward to the controller pod
            for port in ports:
                try:
                    # Use kubectl port-forward in a subprocess
                    import subprocess
                    import threading
                    import time
                    
                    # Start port-forwarding in a separate process
                    process = subprocess.Popen(
                        ["kubectl", "port-forward", pod_name, f"{port}:{port}", "-n", "kube-system"],
                        stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE
                    )
                    
                    # Give it time to establish the connection
                    time.sleep(2)
                    
                    # Collect metrics
                    try:
                        response = requests.get(f"http://localhost:{port}/metrics", timeout=5)
                        if response.status_code == 200:
                            self.csi_metrics[f"port_{port}"] = response.text
                    except requests.RequestException as e:
                        self.logger.warning(f"Failed to collect metrics from port {port}: {e}")
                    
                    # Terminate the port-forwarding process
                    process.terminate()
                    process.wait(timeout=5)
                    
                except Exception as e:
                    self.logger.warning(f"Error collecting metrics from port {port}: {e}")
        
        except Exception as e:
            self.logger.warning(f"Error collecting CSI metrics: {e}")
    
    def get_operation_metrics(self, op_id):
        """Get metrics for an operation
        
        Args:
            op_id: Operation ID or name
            
        Returns:
            Dictionary of metrics
        """
        if op_id not in self.operations:
            self.logger.warning(f"Operation {op_id} not found")
            return {}
            
        metrics = self.operations[op_id].copy()
        
        # Add system metrics if available
        if op_id in self.system_metrics:
            metrics["system"] = self.system_metrics[op_id]
            
        return metrics
    
    def track_controller_request(self, operation_type, start_time, success=True):
        """Track a controller request
        
        Args:
            operation_type: Type of operation (e.g., 'create_volume', 'delete_volume')
            start_time: Start time of the operation
            success: Whether the operation succeeded
        """
        duration = time.time() - start_time
        
        # Record request latency
        if operation_type not in self.controller_metrics["request_latency"]:
            self.controller_metrics["request_latency"][operation_type] = []
        self.controller_metrics["request_latency"][operation_type].append(duration)
        
        # Record operation count
        self.controller_metrics["operation_counts"][operation_type] += 1
        
        # Record success/failure
        status = "success" if success else "failure"
        self.controller_metrics["success_rates"][operation_type][status] += 1
    
    def track_volume_attachment(self, volume_id, start_time):
        """Track volume attachment time
        
        Args:
            volume_id: ID of the volume
            start_time: Start time of the attachment
        """
        duration = time.time() - start_time
        self.controller_metrics["volume_attach_timing"][volume_id] = duration
    
    def track_mount_operation(self, node_name, pod_name, start_time, success=True):
        """Track mount operation time
        
        Args:
            node_name: Name of the node
            pod_name: Name of the pod
            start_time: Start time of the mount operation
            success: Whether the operation succeeded
        """
        duration = time.time() - start_time
        
        if node_name not in self.node_metrics["mount_timing"]:
            self.node_metrics["mount_timing"][node_name] = {}
        
        self.node_metrics["mount_timing"][node_name][pod_name] = duration
        
        if not success:
            self.node_metrics["mount_errors"][node_name] += 1
    
    def track_node_resources(self, node_name, cpu_percent, memory_percent):
        """Track node resource utilization
        
        Args:
            node_name: Name of the node
            cpu_percent: CPU utilization percentage
            memory_percent: Memory utilization percentage
        """
        if node_name not in self.node_metrics["resource_utilization"]:
            self.node_metrics["resource_utilization"][node_name] = []
        
        self.node_metrics["resource_utilization"][node_name].append({
            "timestamp": time.time(),
            "cpu_percent": cpu_percent,
            "memory_percent": memory_percent
        })
    
    def track_access_point_creation(self, access_point_id, start_time):
        """Track access point creation time
        
        Args:
            access_point_id: ID of the access point
            start_time: Start time of the creation
        """
        duration = time.time() - start_time
        self.efs_metrics["access_point_timing"][access_point_id] = duration
    
    def track_mount_completion(self, pod_name, pvc_name, start_time):
        """Track mount completion time
        
        Args:
            pod_name: Name of the pod
            pvc_name: Name of the PVC
            start_time: Start time of the mount operation
        """
        duration = time.time() - start_time
        
        if pod_name not in self.efs_metrics["mount_completion_timing"]:
            self.efs_metrics["mount_completion_timing"][pod_name] = {}
        
        self.efs_metrics["mount_completion_timing"][pod_name][pvc_name] = duration
    
    def record_api_throttling(self, operation_type, error_message):
        """Record API throttling incident
        
        Args:
            operation_type: Type of operation that was throttled
            error_message: Error message from the API
        """
        self.efs_metrics["api_throttling_incidents"].append({
            "timestamp": time.time(),
            "operation_type": operation_type,
            "error_message": error_message
        })
    
    def collect_volume_events(self, namespace="default"):
        """Collect volume-related Kubernetes events
        
        Args:
            namespace: Kubernetes namespace to collect events from
        """
        try:
            kube_client = client.CoreV1Api()
            events = kube_client.list_namespaced_event(namespace=namespace)
            
            for event in events.items:
                if event.involved_object.kind in ["PersistentVolume", "PersistentVolumeClaim"]:
                    self.k8s_events["volume_events"].append({
                        "timestamp": time.time(),
                        "name": event.involved_object.name,
                        "kind": event.involved_object.kind,
                        "reason": event.reason,
                        "message": event.message,
                        "count": event.count
                    })
        except Exception as e:
            self.logger.warning(f"Error collecting volume events: {e}")
    
    def track_pv_pvc_binding(self, pvc_name, pv_name, bind_time):
        """Track PV-PVC binding time
        
        Args:
            pvc_name: Name of the PVC
            pv_name: Name of the PV
            bind_time: Time taken for binding in seconds
        """
        self.k8s_events["binding_times"][f"{pvc_name}-{pv_name}"] = bind_time
    
    def track_pod_startup_delay(self, pod_name, create_time, ready_time):
        """Track pod startup delay
        
        Args:
            pod_name: Name of the pod
            create_time: Time when the pod was created
            ready_time: Time when the pod became ready
        """
        delay = ready_time - create_time
        self.k8s_events["pod_startup_delays"][pod_name] = delay
        
    def parse_prometheus_metrics(self, metrics_text):
        """Parse Prometheus metrics from the CSI driver
        
        Args:
            metrics_text: Raw Prometheus metrics text
        
        Returns:
            Dictionary of parsed metrics
        """
        parsed_metrics = {}
        
        if not metrics_text:
            return parsed_metrics
        
        # Simple regex pattern to extract metrics
        pattern = r'^([a-zA-Z_:][a-zA-Z0-9_:]*)\s*({[^}]*})?\s*([0-9.eE+-]+)'
        
        for line in metrics_text.split('\n'):
            line = line.strip()
            if not line or line.startswith('#'):
                continue
                
            match = re.match(pattern, line)
            if match:
                metric_name = match.group(1)
                labels = match.group(2) or ""
                value = float(match.group(3))
                
                if metric_name not in parsed_metrics:
                    parsed_metrics[metric_name] = []
                
                parsed_metrics[metric_name].append({
                    "labels": labels,
                    "value": value
                })
        
        return parsed_metrics

    def get_all_metrics(self):
        """Get all collected metrics
        
        Returns:
            Dictionary of all metrics
        """
        return {
            "operations": self.operations,
            "system": self.system_metrics,
            "kubernetes": self.k8s_metrics,
            "csi": self.csi_metrics,
            "controller": self.controller_metrics,
            "node": self.node_metrics,
            "efs": self.efs_metrics,
            "k8s_events": self.k8s_events
        }
