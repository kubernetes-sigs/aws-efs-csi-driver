import json
import os
import datetime
import platform
import psutil
import yaml
from pathlib import Path
import socket
import subprocess

class ReportGenerator:
    """Generate detailed test reports in various formats"""
    
    def __init__(self, output_dir="reports"):
        """Initialize report generator
        
        Args:
            output_dir: Base directory to store reports
        """
        self.base_output_dir = output_dir
        Path(output_dir).mkdir(parents=True, exist_ok=True)
    
    def _get_output_dir(self, test_type):
        """Get the output directory for a specific test type
        
        Args:
            test_type: Type of test (e.g., 'stress', 'scalability')
            
        Returns:
            Path to the output directory
        """
        output_dir = os.path.join(self.base_output_dir, test_type)
        Path(output_dir).mkdir(parents=True, exist_ok=True)
        return output_dir
    
    def _determine_test_type(self, test_results):
        """Determine the test type from the results structure

        Args:
            test_results: Dictionary containing test results

        Returns:
            Test type string
        """
        # Check for orchestrator results (operations, scenarios, etc.)
        if "operations" in test_results and "scenarios" in test_results:
            return "orchestrator"
            
        # Default to orchestrator if nothing else matches
        return "orchestrator"
    
    def _collect_system_info(self):
        """Collect system information for the report
        
        Returns:
            Dictionary with system information
        """
        system_info = {
            "hostname": socket.gethostname(),
            "platform": platform.platform(),
            "python_version": platform.python_version(),
            "cpu_count": psutil.cpu_count(),
            "memory_total_gb": round(psutil.virtual_memory().total / (1024**3), 2),
            "timestamp": datetime.datetime.now().isoformat()
        }
        
        # Try to get Kubernetes cluster info
        try:
            kubectl_version = subprocess.check_output(["kubectl", "version", "--short"], 
                                                    stderr=subprocess.STDOUT).decode('utf-8')
            system_info["kubernetes_version"] = kubectl_version.strip()
        except (subprocess.SubprocessError, FileNotFoundError):
            system_info["kubernetes_version"] = "Unknown"
        
        # Try to get AWS region
        try:
            with open('config/test_config.yaml', 'r') as f:
                config = yaml.safe_load(f)
                system_info["aws_region"] = config.get('cluster', {}).get('region', 'Unknown')
                system_info["efs_filesystem_id"] = config.get('efs', {}).get('filesystem_id', 'Unknown')
        except Exception:
            system_info["aws_region"] = "Unknown"
            system_info["efs_filesystem_id"] = "Unknown"
        
        return system_info
    
    def generate_json_report(self, test_results, test_name):
        """Generate detailed JSON report
        
        Args:
            test_results: Dictionary containing test results
            test_name: Name of the test
            
        Returns:
            Path to the generated report
        """
        timestamp = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
        
        # Determine test type from results structure
        test_type = self._determine_test_type(test_results)
        
        output_dir = self._get_output_dir(test_type)
        filename = f"{test_name}_{timestamp}.json"
        filepath = os.path.join(output_dir, filename)
        
        # Add metadata and system info
        report = {
            "test_name": test_name,
            "test_type": test_type,
            "timestamp": timestamp,
            "system_info": self._collect_system_info(),
            "results": test_results
        }
        
        with open(filepath, 'w') as f:
            json.dump(report, f, indent=2)
            
        return filepath
    
    def generate_summary_report(self, test_results, test_name):
        """Generate a detailed human-readable summary report
        
        Args:
            test_results: Dictionary containing test results
            test_name: Name of the test
            
        Returns:
            Path to the generated report
        """
        timestamp = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
        
        # Determine test type from results structure
        test_type = self._determine_test_type(test_results)
        
        output_dir = self._get_output_dir(test_type)
        filename = f"{test_name}_{timestamp}_summary.txt"
        filepath = os.path.join(output_dir, filename)
        
        system_info = self._collect_system_info()
        
        with open(filepath, 'w') as f:
            # Write header
            f.write(f"{'='*80}\n")
            f.write(f"EFS CSI DRIVER TEST REPORT: {test_name.upper()}\n")
            f.write(f"{'='*80}\n\n")
            
            # Write system information
            f.write("SYSTEM INFORMATION\n")
            f.write(f"{'-'*80}\n")
            f.write(f"Hostname: {system_info['hostname']}\n")
            f.write(f"Platform: {system_info['platform']}\n")
            f.write(f"Python Version: {system_info['python_version']}\n")
            f.write(f"CPU Count: {system_info['cpu_count']}\n")
            f.write(f"Memory Total: {system_info['memory_total_gb']} GB\n")
            f.write(f"Kubernetes Version: {system_info['kubernetes_version']}\n")
            f.write(f"AWS Region: {system_info['aws_region']}\n")
            f.write(f"EFS Filesystem ID: {system_info['efs_filesystem_id']}\n")
            f.write(f"Test Timestamp: {system_info['timestamp']}\n\n")
            
            # Write test results
            f.write("TEST RESULTS\n")
            f.write(f"{'-'*80}\n")
            
            # Process different test types
            if test_type == "scalability":
                self._write_scalability_results(f, test_results)
            elif test_type == "stress":
                self._write_stress_results(f, test_results)
            elif test_type == "access_points":
                self._write_access_point_results(f, test_results)
            elif test_type == "statefulset":
                self._write_statefulset_results(f, test_results)
            else:
                self._write_generic_results(f, test_results)
            
            # Write footer
            f.write(f"\n{'='*80}\n")
            f.write(f"END OF REPORT: {datetime.datetime.now().isoformat()}\n")
            f.write(f"{'='*80}\n")
                
        return filepath
    
    def _write_scalability_results(self, file, results):
        """Write scalability test results to the report file"""
        if "pod_scaling" in results:
            file.write("\nPOD SCALING TEST RESULTS\n")
            file.write(f"{'-'*40}\n")
            
            pod_results = results["pod_scaling"]
            if isinstance(pod_results, dict):
                # Sort by scale (number of pods)
                for scale in sorted([int(k) for k in pod_results.keys()]):
                    data = pod_results[scale]
                    success = data.get('success', False)
                    duration = data.get('duration', 0)
                    pods_ready = data.get('pods_ready', 0)
                    
                    file.write(f"Scale: {scale} pods\n")
                    file.write(f"  Status: {'SUCCESS' if success else 'FAILED'}\n")
                    file.write(f"  Duration: {duration:.2f} seconds\n")
                    file.write(f"  Pods Ready: {pods_ready}\n")
                    if duration > 0:
                        file.write(f"  Scale Rate: {scale/duration:.2f} pods/second\n\n")
            else:
                file.write(f"Error: {pod_results}\n\n")
        
        if "volume_scaling" in results:
            file.write("\nVOLUME SCALING TEST RESULTS\n")
            file.write(f"{'-'*40}\n")
            
            volume_results = results["volume_scaling"]
            if isinstance(volume_results, dict):
                # Sort by scale (number of volumes)
                for scale in sorted([int(k) for k in volume_results.keys()]):
                    data = volume_results[scale]
                    success = data.get('success', False)
                    duration = data.get('duration', 0)
                    volumes_ready = data.get('volumes_ready', 0)
                    
                    file.write(f"Scale: {scale} volumes\n")
                    file.write(f"  Status: {'SUCCESS' if success else 'FAILED'}\n")
                    file.write(f"  Duration: {duration:.2f} seconds\n")
                    file.write(f"  Volumes Ready: {volumes_ready}\n")
                    if duration > 0:
                        file.write(f"  Scale Rate: {scale/duration:.2f} volumes/second\n\n")
            else:
                file.write(f"Error: {volume_results}\n\n")
    
    def _write_stress_results(self, file, results):
        """Write stress test results to the report file"""
        for test_name, result in results.items():
            file.write(f"\n{test_name.upper()} RESULTS\n")
            file.write(f"{'-'*40}\n")
            
            if isinstance(result, dict):
                if "sequential_write" in result:
                    file.write("\nSequential Write Test:\n")
                    seq_result = result["sequential_write"]
                    file.write(f"  Status: {seq_result.get('status', 'unknown')}\n")
                    file.write(f"  Duration: {seq_result.get('duration', 'N/A')} seconds\n")
                    
                    # Add detailed metrics if available
                    if "pod_metrics" in seq_result:
                        file.write("  Pod Metrics:\n")
                        for i, metric in enumerate(seq_result["pod_metrics"]):
                            file.write(f"    Sample {i+1}: Phase={metric.get('phase', 'unknown')}\n")
                
                if "random_write" in result:
                    file.write("\nRandom Write Test:\n")
                    rand_result = result["random_write"]
                    file.write(f"  Status: {rand_result.get('status', 'unknown')}\n")
                    file.write(f"  Duration: {rand_result.get('duration', 'N/A')} seconds\n")
                    
                    # Add detailed metrics if available
                    if "pod_metrics" in rand_result:
                        file.write("  Pod Metrics:\n")
                        for i, metric in enumerate(rand_result["pod_metrics"]):
                            file.write(f"    Sample {i+1}: Phase={metric.get('phase', 'unknown')}\n")
                
                if "mixed_io" in result:
                    file.write("\nMixed I/O Test:\n")
                    mixed_result = result["mixed_io"]
                    file.write(f"  Status: {mixed_result.get('status', 'unknown')}\n")
                    file.write(f"  Duration: {mixed_result.get('duration', 'N/A')} seconds\n")
                    
                    # Add detailed metrics if available
                    if "pod_metrics" in mixed_result:
                        file.write("  Pod Metrics:\n")
                        for i, metric in enumerate(mixed_result["pod_metrics"]):
                            file.write(f"    Sample {i+1}: Phase={metric.get('phase', 'unknown')}\n")
            else:
                file.write(f"Error: {result}\n")
    
    def _write_access_point_results(self, file, results):
        """Write access point test results to the report file"""
        if "access_point_scaling" in results:
            file.write("\nACCESS POINT SCALING TEST RESULTS\n")
            file.write(f"{'-'*40}\n")
            
            ap_results = results["access_point_scaling"]
            if isinstance(ap_results, dict):
                # Sort by scale (number of access points)
                for scale in sorted([int(k) for k in ap_results.keys()]):
                    data = ap_results[scale]
                    success = data.get('success', False)
                    duration = data.get('duration', 0)
                    aps_ready = data.get('access_points_ready', 0)
                    
                    file.write(f"Scale: {scale} access points\n")
                    file.write(f"  Status: {'SUCCESS' if success else 'FAILED'}\n")
                    file.write(f"  Duration: {duration:.2f} seconds\n")
                    file.write(f"  Access Points Ready: {aps_ready}\n")
                    if duration > 0:
                        file.write(f"  Scale Rate: {scale/duration:.2f} access points/second\n\n")
            else:
                file.write(f"Error: {ap_results}\n\n")
    
    def _write_statefulset_results(self, file, results):
        """Write StatefulSet test results to the report file"""
        if "statefulset_scaling" in results:
            file.write("\nSTATEFULSET SCALING TEST RESULTS\n")
            file.write(f"{'-'*40}\n")
            
            sts_result = results["statefulset_scaling"]
            if isinstance(sts_result, dict):
                replicas = sts_result.get('replicas', 0)
                success = sts_result.get('success', False)
                duration = sts_result.get('duration', 0)
                pods_ready = sts_result.get('pods_ready', 0)
                
                file.write(f"Replicas: {replicas}\n")
                file.write(f"Status: {'SUCCESS' if success else 'FAILED'}\n")
                file.write(f"Duration: {duration:.2f} seconds\n")
                file.write(f"Pods Ready: {pods_ready}\n")
                if duration > 0:
                    file.write(f"Scale Rate: {replicas/duration:.2f} pods/second\n\n")
            else:
                file.write(f"Error: {sts_result}\n\n")
    
    def _write_generic_results(self, file, results):
        """Write generic test results to the report file"""
        for test_name, result in results.items():
            file.write(f"\n{test_name.upper()}\n")
            file.write(f"{'-'*40}\n")
            
            if isinstance(result, dict):
                for key, value in result.items():
                    if isinstance(value, dict):
                        file.write(f"{key}:\n")
                        for sub_key, sub_value in value.items():
                            file.write(f"  {sub_key}: {sub_value}\n")
                    else:
                        file.write(f"{key}: {value}\n")
            else:
                file.write(f"{result}\n")
