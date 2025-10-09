#!/usr/bin/env python3

import os
import sys
import subprocess
import logging
import shlex
import shutil
import tarfile
from datetime import datetime

def execute_command(command, file, shell=False):
    """Execute a command and write output to file"""
    print(command + "\n", file=file, flush=True)
    if shell:
        subprocess.run(command, shell=True, text=True, stderr=subprocess.STDOUT, stdout=file)
    else:
        subprocess.run(shlex.split(command), text=True, stderr=subprocess.STDOUT, stdout=file)
    print("\n", file=file, flush=True)

def collect_driver_files_under_dir(driver_pod_name, dir_name, file):
    """Collect files under a directory in the container"""
    collect_driver_files_command = (
        f"kubectl exec {driver_pod_name} -n kube-system -c efs-plugin -- find {dir_name} "
        + r"-type f -exec ls {} \; -exec cat {} \;"
    )
    execute_command(command=collect_driver_files_command, file=file)

def collect_logs_for_test(test_name, driver_pod_name=None):
    """
    Use the log collector functionality to collect logs during test execution
    
    Args:
        test_name: Name of the test (for output directory)
        driver_pod_name: Name of the EFS CSI driver pod (if None, will attempt to find one)
        
    Returns:
        Path to the collected logs tarball
    """
    logger = logging.getLogger(__name__)
    logger.info(f"Collecting logs for test: {test_name}")
    
    # If driver pod name is not provided, try to find one
    if driver_pod_name is None:
        driver_pod_name = _find_driver_pod()
        if not driver_pod_name:
            logger.error("No EFS CSI driver pod found, cannot collect logs")
            return None
    
    # Create output directory for this test run
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    results_dir = f"logs/{test_name}_{timestamp}"
    os.makedirs(results_dir, exist_ok=True)
    
    # Create results subdirectory
    results_subdir = os.path.join(results_dir, "results")
    os.makedirs(results_subdir, exist_ok=True)
    
    try:
        # Save current directory and change to results directory
        original_dir = os.getcwd()
        os.chdir(results_dir)
        
        try:
            # Execute the log collection steps
            # Describe and get pod info
            with open(f"results/driver_info", "w") as f:
                execute_command(
                    command=f"kubectl describe pod {driver_pod_name} -n kube-system",
                    file=f
                )
                execute_command(
                    command=f"kubectl get pod {driver_pod_name} -n kube-system -o yaml",
                    file=f
                )
            
            # Get driver logs
            with open(f"results/driver_logs", "w") as f:
                execute_command(
                    command=f"kubectl logs {driver_pod_name} -n kube-system efs-plugin",
                    file=f
                )
            
            # Get EFS utils logs from the container
            with open(f"results/efs_utils_logs", "w") as f:
                collect_driver_files_under_dir(
                    driver_pod_name=driver_pod_name,
                    dir_name="/var/log/amazon/efs",
                    file=f
                )
            
            # Get EFS state directory contents
            with open(f"results/efs_utils_state_dir", "w") as f:
                collect_driver_files_under_dir(
                    driver_pod_name=driver_pod_name,
                    dir_name="/var/run/efs",
                    file=f
                )
            
            # Get mount information
            with open(f"results/mounts", "w") as f:
                execute_command(
                    command=f"kubectl exec {driver_pod_name} -n kube-system -c efs-plugin -- mount | grep nfs",
                    file=f,
                    shell=True
                )
            
            # Create tar file
            tarball_name = f"{test_name}_logs_{timestamp}.tgz"
            with tarfile.open(tarball_name, "w:gz") as tar:
                tar.add("results", arcname="results")
            
            logger.info(f"Log collection completed successfully: {os.path.join(results_dir, tarball_name)}")
            return os.path.join(results_dir, tarball_name)
            
        finally:
            # Change back to original directory
            os.chdir(original_dir)
    
    except Exception as e:
        logger.error(f"Error collecting logs: {e}", exc_info=True)
        return None

def _find_driver_pod():
    """Find an EFS CSI driver pod in the cluster"""
    try:
        # Use kubectl to find driver pods
        result = subprocess.run(
            "kubectl get pods -n kube-system -l app=efs-csi-controller -o jsonpath='{.items[0].metadata.name}'",
            shell=True,
            capture_output=True,
            text=True
        )
        
        if result.stdout:
            return result.stdout.strip()
            
        # Try to find node driver if controller not found
        result = subprocess.run(
            "kubectl get pods -n kube-system -l app=efs-csi-node -o jsonpath='{.items[0].metadata.name}'",
            shell=True,
            capture_output=True,
            text=True
        )
        
        if result.stdout:
            return result.stdout.strip()
            
        return None
    except Exception as e:
        logging.getLogger(__name__).error(f"Error finding driver pod: {e}")
        return None

def collect_resource_logs(resource_type, resource_name, namespace="default"):
    """
    Collect detailed logs and information about a specific Kubernetes resource
    
    Args:
        resource_type: Type of resource (pod, pvc, etc.)
        resource_name: Name of the resource
        namespace: Kubernetes namespace
        
    Returns:
        Path to the directory containing collected logs
    """
    logger = logging.getLogger(__name__)
    logger.info(f"Collecting logs for {resource_type}/{resource_name} in namespace {namespace}")
    
    # Create output directory for this resource
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    results_dir = f"logs/resource_{resource_type}_{resource_name}_{timestamp}"
    os.makedirs(results_dir, exist_ok=True)
    
    try:
        # Get basic info about the resource
        with open(os.path.join(results_dir, f"{resource_type}_description.txt"), "w") as f:
            execute_command(
                command=f"kubectl describe {resource_type} {resource_name} -n {namespace}",
                file=f
            )
            
        with open(os.path.join(results_dir, f"{resource_type}_yaml.yaml"), "w") as f:
            execute_command(
                command=f"kubectl get {resource_type} {resource_name} -n {namespace} -o yaml",
                file=f
            )
        
        # Get events related to this resource
        with open(os.path.join(results_dir, "events.txt"), "w") as f:
            execute_command(
                command=f"kubectl get events -n {namespace} --field-selector involvedObject.name={resource_name} --sort-by='.lastTimestamp'",
                file=f
            )
        
        # Resource-specific logging
        if resource_type == "pod":
            # Get container logs
            execute_command(
                command=f"mkdir -p {os.path.join(results_dir, 'container_logs')}",
                file=None,
                shell=True
            )
            # First get container names
            containers_result = subprocess.run(
                f"kubectl get pod {resource_name} -n {namespace} -o jsonpath='{{.spec.containers[*].name}}'",
                shell=True,
                capture_output=True,
                text=True
            )
            if containers_result.stdout:
                containers = containers_result.stdout.strip().split()
                for container in containers:
                    with open(os.path.join(results_dir, "container_logs", f"{container}.log"), "w") as f:
                        execute_command(
                            command=f"kubectl logs {resource_name} -n {namespace} -c {container}",
                            file=f
                        )
            
            # Get node info for this pod
            node_result = subprocess.run(
                f"kubectl get pod {resource_name} -n {namespace} -o jsonpath='{{.spec.nodeName}}'",
                shell=True,
                capture_output=True,
                text=True
            )
            if node_result.stdout:
                node_name = node_result.stdout.strip()
                with open(os.path.join(results_dir, "node_info.txt"), "w") as f:
                    execute_command(
                        command=f"kubectl describe node {node_name}",
                        file=f
                    )
            
            # Get volume information for this pod
            with open(os.path.join(results_dir, "volumes.txt"), "w") as f:
                execute_command(
                    command=f"kubectl get pod {resource_name} -n {namespace} -o jsonpath='{{.spec.volumes}}'",
                    file=f
                )
                
        elif resource_type == "pvc":
            # Get PV associated with this PVC
            pv_result = subprocess.run(
                f"kubectl get pvc {resource_name} -n {namespace} -o jsonpath='{{.spec.volumeName}}'",
                shell=True,
                capture_output=True,
                text=True
            )
            if pv_result.stdout:
                pv_name = pv_result.stdout.strip()
                if pv_name:
                    with open(os.path.join(results_dir, "pv_info.txt"), "w") as f:
                        execute_command(
                            command=f"kubectl describe pv {pv_name}",
                            file=f
                        )
                    with open(os.path.join(results_dir, "pv_yaml.yaml"), "w") as f:
                        execute_command(
                            command=f"kubectl get pv {pv_name} -o yaml",
                            file=f
                        )
            
            # Find pods using this PVC
            with open(os.path.join(results_dir, "using_pods.txt"), "w") as f:
                execute_command(
                    command=f"kubectl get pods -n {namespace} -o json | jq '.items[] | select(.spec.volumes[]?.persistentVolumeClaim?.claimName == \"{resource_name}\") | .metadata.name'",
                    file=f,
                    shell=True
                )
        
        logger.info(f"Resource logs collected successfully to: {results_dir}")
        return results_dir
    
    except Exception as e:
        logger.error(f"Error collecting resource logs: {e}", exc_info=True)
        return None

def collect_logs_on_test_failure(test_name, metrics_collector=None, driver_pod_name=None, failed_resources=None):
    """
    Collect logs when a test fails, and include metrics if available
    
    Args:
        test_name: Name of the test
        metrics_collector: Optional metrics collector instance
        driver_pod_name: Name of the EFS CSI driver pod
        failed_resources: Optional list of dicts with 'type', 'name', and 'namespace' keys
        
    Returns:
        Path to the collected logs tarball
    """
    logger = logging.getLogger(__name__)
    logger.info(f"Test '{test_name}' failed, collecting logs")
    
    # Create main directory for all failure logs
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    main_dir = f"logs/{test_name}_failure_{timestamp}"
    os.makedirs(main_dir, exist_ok=True)
    
    # Collect CSI driver logs using our own functions
    driver_logs_dir = collect_logs_for_test(f"{test_name}_driver", driver_pod_name)
    
    # Collect logs for each failed resource if provided
    if failed_resources:
        resources_dir = os.path.join(main_dir, "failed_resources")
        os.makedirs(resources_dir, exist_ok=True)
        
        for resource in failed_resources:
            resource_type = resource.get("type", "unknown")
            resource_name = resource.get("name", "unknown")
            namespace = resource.get("namespace", "default")
            
            resource_logs_dir = collect_resource_logs(
                resource_type=resource_type,
                resource_name=resource_name,
                namespace=namespace
            )
            
            # Copy resource logs to main directory
            if resource_logs_dir and os.path.exists(resource_logs_dir):
                logger.info(f"Adding {resource_type}/{resource_name} logs to failure archive")
                resource_target_dir = os.path.join(resources_dir, f"{resource_type}_{resource_name}")
                os.makedirs(resource_target_dir, exist_ok=True)
                
                # Copy all files from resource_logs_dir to resource_target_dir
                for item in os.listdir(resource_logs_dir):
                    source = os.path.join(resource_logs_dir, item)
                    target = os.path.join(resource_target_dir, item)
                    if os.path.isdir(source):
                        shutil.copytree(source, target, dirs_exist_ok=True)
                    else:
                        shutil.copy2(source, target)
    
    # If we have a metrics collector, save its data
    if metrics_collector:
        try:
            metrics_dir = os.path.join(main_dir, "metrics")
            os.makedirs(metrics_dir, exist_ok=True)
            
            metrics_file = os.path.join(metrics_dir, "test_metrics.json")
            with open(metrics_file, "w") as f:
                import json
                json.dump(metrics_collector.get_all_metrics(), f, indent=2)
                
            logger.info(f"Metrics saved to {metrics_file}")
        except Exception as e:
            logger.error(f"Error saving metrics: {e}")
    
    # Create tar file containing all logs
    tarball_path = f"{main_dir}.tgz"
    with tarfile.open(tarball_path, "w:gz") as tar:
        tar.add(main_dir, arcname=os.path.basename(main_dir))
    
    logger.info(f"Comprehensive failure logs collected to: {tarball_path}")
    return tarball_path

# Example of how to use this in tests
if __name__ == "__main__":
    # Set up basic logging
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )
    
    # Example usage
    tarball_path = collect_logs_for_test("example_test")
    print(f"Logs collected to: {tarball_path}")
# Enhanced log integration module
