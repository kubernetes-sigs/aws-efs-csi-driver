#!/usr/bin/env python3

import argparse
import logging
import os
import time
from datetime import datetime
from kubernetes import client, config
from kubernetes.client.rest import ApiException

"""
EFS CSI Driver Test Cleanup Script
This script deletes all test-related resources to ensure a clean environment
"""

# Configure logging
os.makedirs('logs', exist_ok=True)
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler(f'logs/cleanup_{datetime.now().strftime("%Y%m%d_%H%M%S")}.log'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)

def delete_resource(api_instance, resource_type, name, namespace, force=False):
    """Delete a specific Kubernetes resource with proper error handling"""
    try:
        logger.info(f"Deleting {resource_type}/{name} in namespace {namespace}")
        
        # Set deletion options
        body = client.V1DeleteOptions(
            grace_period_seconds=0 if force else None,
            propagation_policy="Background" if force else "Foreground"
        )
        
        if resource_type == "pod":
            api_instance.delete_namespaced_pod(name=name, namespace=namespace, body=body)
        elif resource_type == "pvc":
            api_instance.delete_namespaced_persistent_volume_claim(name=name, namespace=namespace, body=body)
        
        return True
    except ApiException as e:
        if e.status == 404:
            logger.warning(f"{resource_type}/{name} already deleted or not found")
            return True
        else:
            logger.error(f"Failed to delete {resource_type}/{name}: {e}")
            return False

def cleanup_test_resources(namespaces=None, pod_prefixes=None, pvc_prefixes=None, force=True, wait=True):
    """Clean up all test-related resources"""
    # Load kube config
    try:
        config.load_kube_config()
    except Exception as e:
        logger.error(f"Failed to load kubeconfig: {e}")
        return False
    
    core_v1 = client.CoreV1Api()
    
    # Default prefixes if none provided
    if not pod_prefixes:
        pod_prefixes = ["test-pod-", "efs-scale-test-", "efs-app", "efs-sanity-pod"]
    
    if not pvc_prefixes:
        pvc_prefixes = ["test-pvc-", "concurrent-pvc-", "many2one-", "one2one-", 
                        "chaos-pvc-", "chaos-ap-", "efs-volume-", "scale-test-pvc"]
    
    # Get list of namespaces to clean up
    try:
        if not namespaces:
            namespaces_list = core_v1.list_namespace()
            namespaces = [ns.metadata.name for ns in namespaces_list.items]
            # Filter only to likely test namespaces to avoid touching system namespaces
            namespaces = [ns for ns in namespaces if ns in 
                         ["default", "efs-stress-test", "efs-test", "test"]]
        
        logger.info(f"Cleaning up resources in namespaces: {namespaces}")
    except ApiException as e:
        logger.error(f"Failed to list namespaces: {e}")
        return False
    
    # Track deleted resources and failures
    deleted_resources = {
        "pods": [],
        "pvcs": []
    }
    failed_deletions = {
        "pods": [],
        "pvcs": []
    }
    
    # Delete pods that match the prefixes in each namespace
    for namespace in namespaces:
        try:
            # Get all pods in the namespace
            pods = core_v1.list_namespaced_pod(namespace=namespace)
            
            # Filter pods that match the prefixes
            for pod in pods.items:
                pod_name = pod.metadata.name
                if any(pod_name.startswith(prefix) for prefix in pod_prefixes):
                    success = delete_resource(core_v1, "pod", pod_name, namespace, force)
                    if success:
                        deleted_resources["pods"].append(f"{namespace}/{pod_name}")
                    else:
                        failed_deletions["pods"].append(f"{namespace}/{pod_name}")
        except ApiException as e:
            logger.error(f"Failed to list pods in namespace {namespace}: {e}")
    
    # Wait briefly for pods to start terminating
    if wait:
        logger.info("Waiting 5 seconds for pods to start terminating before deleting PVCs...")
        time.sleep(5)
    
    # Delete PVCs that match the prefixes in each namespace
    for namespace in namespaces:
        try:
            # Get all PVCs in the namespace
            pvcs = core_v1.list_namespaced_persistent_volume_claim(namespace=namespace)
            
            # Filter PVCs that match the prefixes
            for pvc in pvcs.items:
                pvc_name = pvc.metadata.name
                if any(pvc_name.startswith(prefix) for prefix in pvc_prefixes):
                    success = delete_resource(core_v1, "pvc", pvc_name, namespace, force)
                    if success:
                        deleted_resources["pvcs"].append(f"{namespace}/{pvc_name}")
                    else:
                        failed_deletions["pvcs"].append(f"{namespace}/{pvc_name}")
        except ApiException as e:
            logger.error(f"Failed to list PVCs in namespace {namespace}: {e}")
    
    # Print summary
    logger.info("Cleanup Summary:")
    logger.info(f"Deleted {len(deleted_resources['pods'])} pods and {len(deleted_resources['pvcs'])} PVCs")
    
    if failed_deletions["pods"] or failed_deletions["pvcs"]:
        logger.warning("Failed deletions:")
        for pod in failed_deletions["pods"]:
            logger.warning(f"  - Pod: {pod}")
        for pvc in failed_deletions["pvcs"]:
            logger.warning(f"  - PVC: {pvc}")
        return False
    else:
        logger.info("All resources deleted successfully")
        return True

def verify_resources_deleted(namespaces=None, pod_prefixes=None, pvc_prefixes=None, timeout=60):
    """Verify that resources have been completely deleted"""
    if not namespaces:
        namespaces = ["default", "efs-stress-test", "efs-test", "test"]
    
    if not pod_prefixes:
        pod_prefixes = ["test-pod-", "efs-scale-test-", "efs-app", "efs-sanity-pod"]
    
    if not pvc_prefixes:
        pvc_prefixes = ["test-pvc-", "concurrent-pvc-", "many2one-", "one2one-", 
                       "chaos-pvc-", "chaos-ap-", "efs-volume-", "scale-test-pvc"]
    
    logger.info(f"Verifying resource deletion for up to {timeout} seconds...")
    
    start_time = time.time()
    core_v1 = client.CoreV1Api()
    
    while time.time() - start_time < timeout:
        remaining_resources = []
        
        # Check each namespace for remaining resources
        for namespace in namespaces:
            try:
                # Check for remaining pods
                pods = core_v1.list_namespaced_pod(namespace=namespace)
                for pod in pods.items:
                    if any(pod.metadata.name.startswith(prefix) for prefix in pod_prefixes):
                        remaining_resources.append(f"pod/{namespace}/{pod.metadata.name}")
                
                # Check for remaining PVCs
                pvcs = core_v1.list_namespaced_persistent_volume_claim(namespace=namespace)
                for pvc in pvcs.items:
                    if any(pvc.metadata.name.startswith(prefix) for prefix in pvc_prefixes):
                        remaining_resources.append(f"pvc/{namespace}/{pvc.metadata.name}")
            
            except ApiException as e:
                logger.error(f"Error checking namespace {namespace}: {e}")
        
        if not remaining_resources:
            logger.info(f"All resources deleted successfully after {time.time() - start_time:.1f} seconds")
            return True
        
        logger.info(f"Still waiting on {len(remaining_resources)} resources to be deleted...")
        time.sleep(5)
    
    # If we get here, we timed out waiting for deletion
    logger.error(f"Timed out waiting for resource deletion. Remaining resources:")
    for resource in remaining_resources:
        logger.error(f"  - {resource}")
    
    return False

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Clean up EFS CSI Driver test resources")
    parser.add_argument("--namespaces", "-n", type=str, nargs="+",
                        help="Namespaces to clean up (default: default, efs-stress-test)")
    parser.add_argument("--force", "-f", action="store_true", default=True,
                        help="Force deletion with grace period 0 (default: True)")
    parser.add_argument("--verify", "-v", action="store_true", default=True,
                        help="Verify that all resources are deleted (default: True)")
    parser.add_argument("--verify-timeout", "-t", type=int, default=60,
                        help="Timeout in seconds for verification (default: 60)")
    
    args = parser.parse_args()
    
    # Start the cleanup process
    logger.info("Starting EFS CSI Driver test resource cleanup")
    success = cleanup_test_resources(
        namespaces=args.namespaces,
        force=args.force
    )
    
    # Verify deletion if requested
    if args.verify and success:
        verify_resources_deleted(
            namespaces=args.namespaces,
            timeout=args.verify_timeout
        )
    
    logger.info("Cleanup process completed")
