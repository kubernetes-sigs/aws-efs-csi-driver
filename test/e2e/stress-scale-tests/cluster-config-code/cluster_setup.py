#!/usr/bin/env python3
"""
EFS CSI Driver Cluster Setup Script

This script automates the provisioning of an EKS cluster and installation of the EFS CSI Driver:
1. Creates an EKS cluster with the specified Kubernetes version
2. Creates an EFS filesystem in the same VPC (or uses an existing one)
3. Installs the EFS CSI Driver with the specified version
4. Sets up the storage class and other necessary resources

This can be used standalone or imported by the test orchestrator.
"""

import os
import sys
import time
import json
import yaml
import logging
import subprocess
import argparse
from pathlib import Path
from datetime import datetime

# Set up logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler("logs/cluster_setup.log"),
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger("cluster_setup")

class ClusterSetup:
    """Class to automate EKS cluster creation and EFS CSI driver installation"""
    
    def __init__(self, config_file='config/orchestrator_config.yaml'):
        """Initialize setup parameters from configuration file"""
        # Load configuration
        with open(config_file, 'r') as f:
            self.config = yaml.safe_load(f)
        
        # Extract cluster configuration
        cluster_config = self.config.get('cluster', {})
        self.create_cluster = cluster_config.get('create', True)
        self.kubernetes_version = cluster_config.get('kubernetes_version', '1.26')
        self.region = cluster_config.get('region', 'us-west-2')
        self.node_type = cluster_config.get('node_type', 't3.large')
        self.node_count = cluster_config.get('node_count', 3)
        self.cluster_name = cluster_config.get('name', f"efs-csi-test-{int(time.time())}")
        
        # Extract driver configuration
        driver_config = self.config.get('driver', {})
        self.csi_version = driver_config.get('version', '2.4.8')
        self.install_method = driver_config.get('install_method', 'helm')
        self.create_filesystem = driver_config.get('create_filesystem', True)
        self.filesystem_id = driver_config.get('filesystem_id', None)
        
        # Track resources
        self.cluster_config_path = None
        self.existing_cluster = not self.create_cluster
        
    def setup(self):
        """Run the complete setup process"""
        # Create directories for logs and configs
        os.makedirs("logs", exist_ok=True)
        
        if self.create_cluster:
            self.create_cluster_config()
            self.provision_cluster()
        else:
            logger.info("Using existing cluster as specified in config")
            self.verify_cluster_connection()
        
        if self.create_filesystem:
            self.create_efs_filesystem()
        else:
            if not self.filesystem_id:
                logger.error("No filesystem ID provided but create_filesystem=false")
                raise ValueError("When create_filesystem is false, you must specify a filesystem_id")
            logger.info(f"Using existing filesystem: {self.filesystem_id}")
        
        self.install_efs_csi_driver()
        self.create_storage_class()
        
        logger.info("Cluster setup completed successfully!")
        return {
            "cluster_name": self.cluster_name,
            "region": self.region,
            "kubernetes_version": self.kubernetes_version,
            "filesystem_id": self.filesystem_id,
            "csi_version": self.csi_version
        }
        
    def create_cluster_config(self):
        """Create eksctl cluster configuration"""
        config_dir = "cluster-config"
        os.makedirs(config_dir, exist_ok=True)
        
        # Generate cluster configuration for eksctl
        cluster_config = {
            "apiVersion": "eksctl.io/v1alpha5",
            "kind": "ClusterConfig",
            "metadata": {
                "name": self.cluster_name,
                "region": self.region,
                "version": self.kubernetes_version
            },
            "iam": {
                "withOIDC": True,
                "serviceAccounts": [
                    {
                        "metadata": {
                            "name": "efs-csi-controller-sa",
                            "namespace": "kube-system"
                        },
                        "attachPolicyARNs": [
                            "arn:aws:iam::aws:policy/service-role/AmazonEFSCSIDriverPolicy"
                        ]
                    }
                ]
            },
            "nodeGroups": [
                {
                    "name": "ng-1",
                    "instanceType": self.node_type,
                    "desiredCapacity": self.node_count,
                    "minSize": max(1, self.node_count - 1),
                    "maxSize": self.node_count + 2,
                    "volumeSize": 80,
                    "labels": {
                        "role": "general",
                        "efs-issue": "true"
                    },
                    "tags": {
                        "nodegroup-role": "general"
                    },
                    "iam": {
                        "attachPolicy": {
                            "Version": "2012-10-17",
                            "Statement": [
                                {
                                    "Effect": "Allow",
                                    "Action": [
                                        "elasticfilesystem:DescribeAccessPoints",
                                        "elasticfilesystem:DescribeFileSystems",
                                        "elasticfilesystem:DescribeMountTargets",
                                        "ec2:DescribeAvailabilityZones"
                                    ],
                                    "Resource": "*"
                                },
                                {
                                    "Effect": "Allow",
                                    "Action": [
                                        "elasticfilesystem:CreateAccessPoint"
                                    ],
                                    "Resource": "*",
                                    "Condition": {
                                        "StringLike": {
                                            "aws:RequestTag/kubernetes.io/cluster/*": "owned"
                                        }
                                    }
                                },
                                {
                                    "Effect": "Allow",
                                    "Action": [
                                        "elasticfilesystem:DeleteAccessPoint"
                                    ],
                                    "Resource": "*",
                                    "Condition": {
                                        "StringEquals": {
                                            "aws:ResourceTag/kubernetes.io/cluster/*": "owned"
                                        }
                                    }
                                }
                            ]
                        }
                    }
                }
            ]
        }
        
        # Write config to file
        config_path = os.path.join(config_dir, f"{self.cluster_name}.yaml")
        with open(config_path, 'w') as f:
            yaml.dump(cluster_config, f)
            
        logger.info(f"Created cluster config at {config_path}")
        self.cluster_config_path = config_path
        
        return config_path
    
    def provision_cluster(self):
        """Create EKS cluster using eksctl"""
        if not self.cluster_config_path:
            self.create_cluster_config()
        
        # Verify AWS credentials before attempting to create cluster
        logger.info("Verifying AWS credentials...")
        aws_check = self._run_command(["aws", "sts", "get-caller-identity"])
        
        if aws_check['returncode'] != 0:
            logger.error("AWS credentials verification failed. Please check your AWS configuration.")
            logger.error(f"Error: {aws_check['stderr']}")
            sys.exit(1)
        else:
            ident = json.loads(aws_check['stdout'])
            logger.info(f"Using AWS account: {ident.get('Account', 'unknown')} with user: {ident.get('UserId', 'unknown')}")
        
        # Verify eksctl installation
        eksctl_check = self._run_command(["eksctl", "version"])
        if eksctl_check['returncode'] != 0:
            logger.error("eksctl not found or not working properly. Please install eksctl.")
            logger.error(f"Error: {eksctl_check['stderr']}")
            sys.exit(1)
        else:
            logger.info(f"eksctl version: {eksctl_check['stdout'].strip()}")
        
        logger.info(f"Creating EKS cluster {self.cluster_name} in {self.region}...")
        logger.info("This may take 15-20 minutes to complete...")
        
        # Use verbose flag for more detailed output
        result = self._run_command(
            ["eksctl", "create", "cluster", "-f", self.cluster_config_path, "--verbose", "4"]
        )
        
        if result['returncode'] != 0:
            logger.error("Failed to create EKS cluster")
            logger.error(f"Error details: {result['stderr']}")
            logger.error("Common causes of failure:")
            logger.error("  1. Insufficient IAM permissions")
            logger.error("  2. Service quotas reached (check your AWS console)")
            logger.error("  3. Network connectivity issues")
            logger.error("  4. Region availability zone limitations")
            
            # Save the detailed output to a log file for debugging
            error_log = f"logs/eks-cluster-creation-error-{self.cluster_name}.log"
            with open(error_log, 'w') as f:
                f.write(f"STDOUT:\n{result['stdout']}\n\nSTDERR:\n{result['stderr']}")
            logger.error(f"Detailed error log saved to: {error_log}")
            
            sys.exit(1)
            
        logger.info("EKS cluster created successfully")
        
        # Update kubeconfig
        self._run_command([
            "aws", "eks", "update-kubeconfig",
            "--name", self.cluster_name,
            "--region", self.region
        ])
        
        # Verify cluster is accessible
        self.verify_cluster_connection()
        
    def verify_cluster_connection(self):
        """Verify that we can connect to the Kubernetes cluster"""
        nodes_check = self._run_command(["kubectl", "get", "nodes"])
        if nodes_check['returncode'] == 0:
            logger.info("Cluster verified - nodes accessible:")
            logger.info(nodes_check['stdout'])
        else:
            logger.error("Could not verify cluster nodes. There might be connectivity issues.")
            logger.error(f"Error: {nodes_check['stderr']}")
            sys.exit(1)
    
    def create_efs_filesystem(self):
        """Create EFS filesystem in the same VPC as the cluster"""
        logger.info("Creating EFS filesystem...")
        
        # Get VPC ID from cluster
        logger.info(f"Getting VPC ID for cluster {self.cluster_name} in region {self.region}")
        result = self._run_command([
            "aws", "eks", "describe-cluster",
            "--name", self.cluster_name,
            "--region", self.region,
            "--query", "cluster.resourcesVpcConfig.vpcId",
            "--output", "text"
        ])
        
        if result['returncode'] != 0:
            logger.error("Failed to get VPC ID")
            sys.exit(1)
            
        vpc_id = result['stdout'].strip()
        logger.info(f"Cluster VPC: {vpc_id}")
        
        # Create security group for EFS
        sg_result = self._run_command([
            "aws", "ec2", "create-security-group",
            "--group-name", f"efs-test-sg-{self.cluster_name}",
            "--description", "EFS security group for CSI driver testing",
            "--vpc-id", vpc_id,
            "--output", "json"
        ])
        
        if sg_result['returncode'] != 0:
            logger.error("Failed to create security group")
            sys.exit(1)
            
        sg_data = json.loads(sg_result['stdout'])
        sg_id = sg_data['GroupId']
        
        # Add inbound rule for NFS
        self._run_command([
            "aws", "ec2", "authorize-security-group-ingress",
            "--group-id", sg_id,
            "--protocol", "tcp",
            "--port", "2049",
            "--cidr", "0.0.0.0/0"
        ])
        
        # Create EFS filesystem
        efs_result = self._run_command([
            "aws", "efs", "create-file-system",
            "--performance-mode", "generalPurpose",
            "--throughput-mode", "bursting",
            "--tags", f"Key=Name,Value=efs-csi-test-{self.cluster_name}",
            "--region", self.region,
            "--output", "json"
        ])
        
        if efs_result['returncode'] != 0:
            logger.error("Failed to create EFS filesystem")
            sys.exit(1)
            
        efs_data = json.loads(efs_result['stdout'])
        filesystem_id = efs_data['FileSystemId']
        self.filesystem_id = filesystem_id
        logger.info(f"Created EFS filesystem: {filesystem_id}")
        
        # Wait for filesystem creation to complete
        logger.info("Waiting for EFS filesystem to be available...")
        time.sleep(10)  # Initial wait
        
        max_retries = 12
        retry_count = 0
        while retry_count < max_retries:
            status_result = self._run_command([
                "aws", "efs", "describe-file-systems",
                "--file-system-id", filesystem_id,
                "--query", "FileSystems[0].LifeCycleState",
                "--output", "text"
            ])
            
            status = status_result['stdout'].strip()
            logger.info(f"EFS status: {status}")
            
            if status == "available":
                break
                
            retry_count += 1
            time.sleep(10)
        
        if retry_count >= max_retries:
            logger.error("EFS filesystem failed to become available in time")
            sys.exit(1)
        
        # Get subnet IDs
        subnet_result = self._run_command([
            "aws", "ec2", "describe-subnets",
            "--filters", f"Name=vpc-id,Values={vpc_id}",
            "--query", "Subnets[*].SubnetId",
            "--output", "json"
        ])
        
        subnets = json.loads(subnet_result['stdout'])
        
        # Create mount targets in each subnet
        for subnet_id in subnets:
            logger.info(f"Creating mount target in subnet {subnet_id}")
            self._run_command([
                "aws", "efs", "create-mount-target",
                "--file-system-id", filesystem_id,
                "--subnet-id", subnet_id,
                "--security-groups", sg_id
            ])
        
        # Wait for mount targets to be available
        logger.info("Waiting for mount targets to be available...")
        time.sleep(20)  # Initial wait
        
        max_retries = 12
        retry_count = 0
        while retry_count < max_retries:
            targets_result = self._run_command([
                "aws", "efs", "describe-mount-targets",
                "--file-system-id", filesystem_id,
                "--output", "json"
            ])
            
            targets = json.loads(targets_result['stdout'])['MountTargets']
            
            # Check if all targets are available
            all_available = True
            for target in targets:
                if target['LifeCycleState'] != 'available':
                    all_available = False
                    break
            
            if all_available:
                logger.info(f"All {len(targets)} mount targets are available")
                break
                
            retry_count += 1
            logger.info(f"Waiting for mount targets... (retry {retry_count}/{max_retries})")
            time.sleep(10)
        
        if retry_count >= max_retries:
            logger.warning("Some mount targets may not be available yet, but continuing...")
        
        # Update config file with filesystem ID
        self.update_config_with_filesystem()
        return filesystem_id
    
    def update_config_with_filesystem(self):
        """Update the configuration file with filesystem ID"""
        if not self.filesystem_id:
            logger.warning("No filesystem ID to update in config")
            return
            
        # Update the original config file
        with open('config/orchestrator_config.yaml', 'r') as f:
            config_data = yaml.safe_load(f)
        
        # Update the driver section
        if 'driver' in config_data:
            config_data['driver']['filesystem_id'] = self.filesystem_id
        
        # Update the storage_class section
        if 'storage_class' in config_data and 'parameters' in config_data['storage_class']:
            config_data['storage_class']['parameters']['fileSystemId'] = self.filesystem_id
        
        # Write updated config
        with open('config/orchestrator_config.yaml', 'w') as f:
            yaml.dump(config_data, f)
        
        logger.info(f"Updated config file with filesystem ID: {self.filesystem_id}")
    
    def install_efs_csi_driver(self):
        """Install EFS CSI driver on the cluster"""
        logger.info(f"Installing EFS CSI Driver {self.csi_version}...")
        
        if self.install_method == "helm":
            self._install_via_helm()
        elif self.install_method == "yaml":
            self._install_via_yaml()
        elif self.install_method == "custom":
            self._install_custom()
        else:
            logger.error(f"Unknown installation method: {self.install_method}")
            sys.exit(1)
        
        logger.info("Verifying EFS CSI Driver installation...")
        self._verify_driver_installation()
    
    def _install_via_helm(self):
        """Install the EFS CSI Driver using Helm"""
        # Install driver using Helm
        self._run_command([
            "helm", "repo", "add", "aws-efs-csi-driver", 
            "https://kubernetes-sigs.github.io/aws-efs-csi-driver/"
        ])
        
        self._run_command(["helm", "repo", "update"])
        
        result = self._run_command([
            "helm", "upgrade", "--install", "aws-efs-csi-driver",
            "aws-efs-csi-driver/aws-efs-csi-driver",
            "--namespace", "kube-system",
            "--version", self.csi_version,
            "--set", "controller.serviceAccount.create=false",
            "--set", "controller.serviceAccount.name=efs-csi-controller-sa"
        ])
        
        if result['returncode'] != 0:
            logger.error("Failed to install EFS CSI driver via Helm")
            sys.exit(1)
            
        logger.info("EFS CSI driver installed successfully via Helm")
    
    def _install_via_yaml(self):
        """Install the EFS CSI Driver using kubectl and YAML manifests"""
        # Download the driver YAML for the specified version
        yaml_url = f"https://raw.githubusercontent.com/kubernetes-sigs/aws-efs-csi-driver/v{self.csi_version}/deploy/kubernetes/overlays/stable/ecr/manifest.yaml"
        
        # Download the manifest
        result = self._run_command([
            "curl", "-sSL", yaml_url, "-o", "efs-csi-driver.yaml"
        ])
        
        if result['returncode'] != 0:
            logger.error(f"Failed to download YAML manifest from {yaml_url}")
            sys.exit(1)
        
        # Apply the manifest
        apply_result = self._run_command([
            "kubectl", "apply", "-f", "efs-csi-driver.yaml"
        ])
        
        if apply_result['returncode'] != 0:
            logger.error("Failed to apply EFS CSI driver YAML")
            sys.exit(1)
            
        logger.info("EFS CSI driver installed successfully via YAML")
    
    def _install_custom(self):
        """Install the EFS CSI Driver using custom method specified in config"""
        logger.info("Using custom installation method - this must be implemented by user")
        # This is a placeholder for custom installation logic
        # Users would implement this in their own subclass or modify this method
        
    def _verify_driver_installation(self):
        """Verify that the EFS CSI Driver is installed and running"""
        pod_check = self._run_command([
            "kubectl", "get", "pods", "-n", "kube-system", 
            "-l", "app=efs-csi-controller", 
            "--no-headers"
        ])
        
        if pod_check['returncode'] != 0 or not pod_check['stdout'].strip():
            logger.error("Could not find EFS CSI controller pods")
            sys.exit(1)
        
        # Count running pods
        running_pods = 0
        for line in pod_check['stdout'].strip().split('\n'):
            if line and "Running" in line:
                running_pods += 1
        
        if running_pods == 0:
            logger.error("No EFS CSI controller pods are running")
            sys.exit(1)
        
        logger.info(f"Found {running_pods} running EFS CSI controller pods")
        
        # Check CSI driver registration
        csi_check = self._run_command([
            "kubectl", "get", "csidrivers.storage.k8s.io", 
            "efs.csi.aws.com", "--no-headers"
        ])
        
        if csi_check['returncode'] != 0:
            logger.warning("CSI driver not registered as a custom resource")
        else:
            logger.info("CSI driver registered successfully")
    
    def create_storage_class(self):
        """Create the storage class for the EFS CSI Driver"""
        if not self.filesystem_id:
            logger.error("Cannot create storage class without filesystem ID")
            sys.exit(1)
            
        logger.info(f"Creating StorageClass with filesystem ID: {self.filesystem_id}")
        
        # Get storage class configuration from config
        sc_config = self.config.get('storage_class', {})
        sc_name = sc_config.get('name', 'efs-sc')
        sc_parameters = sc_config.get('parameters', {})
        
        # Ensure filesystem ID is set
        sc_parameters['fileSystemId'] = self.filesystem_id
        
        storage_class = {
            "apiVersion": "storage.k8s.io/v1",
            "kind": "StorageClass",
            "metadata": {"name": sc_name},
            "provisioner": "efs.csi.aws.com",
            "parameters": sc_parameters
        }
        
        # Add mount options if specified
        mount_options = sc_config.get('mount_options')
        if mount_options:
            storage_class["mountOptions"] = mount_options
            
        # Add reclaim policy if specified
        reclaim_policy = sc_config.get('reclaim_policy')
        if reclaim_policy:
            storage_class["reclaimPolicy"] = reclaim_policy
            
        # Add volume binding mode if specified
        volume_binding_mode = sc_config.get('volume_binding_mode')
        if volume_binding_mode:
            storage_class["volumeBindingMode"] = volume_binding_mode
        
        # Write storage class to file
        sc_path = "storage-class.yaml"
        with open(sc_path, 'w') as f:
            yaml.dump(storage_class, f)
        
        # Apply storage class
        result = self._run_command(["kubectl", "apply", "-f", sc_path])
        
        if result['returncode'] != 0:
            logger.error("Failed to create storage class")
            sys.exit(1)
            
        logger.info(f"Storage class '{sc_name}' created successfully")
        
        # Verify storage class
        verify_result = self._run_command([
            "kubectl", "get", "storageclass", sc_name
        ])
        
        if verify_result['returncode'] == 0:
            logger.info(f"Verified storage class '{sc_name}' exists")
        else:
            logger.warning(f"Could not verify storage class '{sc_name}'")
    
    def cleanup(self, delete_cluster=True):
        """Clean up resources - delete EFS filesystem and optionally the EKS cluster"""
        logger.info("Starting cleanup process...")
        
        if self.filesystem_id and self.create_filesystem:
            logger.info(f"Deleting EFS filesystem {self.filesystem_id}...")
            
            # First delete mount targets
            try:
                targets_result = self._run_command([
                    "aws", "efs", "describe-mount-targets",
                    "--file-system-id", self.filesystem_id,
                    "--output", "json"
                ])
                
                if targets_result['returncode'] == 0:
                    targets = json.loads(targets_result['stdout']).get('MountTargets', [])
                    
                    for target in targets:
                        target_id = target['MountTargetId']
                        logger.info(f"Deleting mount target {target_id}")
                        self._run_command([
                            "aws", "efs", "delete-mount-target",
                            "--mount-target-id", target_id
                        ])
                    
                    # Wait for mount targets to be deleted
                    logger.info("Waiting for mount targets to be deleted...")
                    time.sleep(30)
            except Exception as e:
                logger.warning(f"Error while deleting mount targets: {e}")
            
            # Now delete the filesystem
            try:
                self._run_command([
                    "aws", "efs", "delete-file-system",
                    "--file-system-id", self.filesystem_id
                ])
                logger.info(f"EFS filesystem {self.filesystem_id} deletion initiated")
            except Exception as e:
                logger.warning(f"Error while deleting filesystem: {e}")
        
        if delete_cluster and self.cluster_name and self.create_cluster:
            logger.info(f"Deleting EKS cluster {self.cluster_name}...")
            
            try:
                self._run_command([
                    "eksctl", "delete", "cluster",
                    "--name", self.cluster_name,
                    "--region", self.region
                ])
                logger.info(f"EKS cluster {self.cluster_name} deletion initiated")
            except Exception as e:
                logger.warning(f"Error while deleting cluster: {e}")
                
        logger.info("Cleanup process completed")
    
    def _run_command(self, cmd):
        """Run a command and return stdout, stderr, and return code"""
        logger.info(f"Running: {' '.join(cmd)}")
        
        process = subprocess.Popen(
            cmd, 
            stdout=subprocess.PIPE, 
            stderr=subprocess.PIPE,
            universal_newlines=True
        )
        
        stdout, stderr = process.communicate()
        
        if process.returncode != 0:
            logger.warning(f"Command returned non-zero exit code: {process.returncode}")
            logger.warning(f"stderr: {stderr}")
        
        return {
            "stdout": stdout,
            "stderr": stderr,
            "returncode": process.returncode
        }

def main():
    """Main function when run as a script"""
    parser = argparse.ArgumentParser(description='EFS CSI Driver Cluster Setup')
    
    parser.add_argument(
        '--config',
        default='config/orchestrator_config.yaml',
        help='Path to configuration file'
    )
    
    parser.add_argument(
        '--no-cleanup',
        action='store_true',
        help='Do not clean up resources on failure'
    )
    
    parser.add_argument(
        '--verify-only',
        action='store_true',
        help='Only verify existing cluster and exit'
    )
    
    parser.add_argument(
        '--debug',
        action='store_true',
        help='Enable debug logging'
    )
    
    args = parser.parse_args()
    
    # Set debug logging if requested
    if args.debug:
        logger.setLevel(logging.DEBUG)
        for handler in logger.handlers:
            handler.setLevel(logging.DEBUG)
        
    try:
        # Initialize setup
        setup = ClusterSetup(config_file=args.config)
        
        if args.verify_only:
            setup.verify_cluster_connection()
            logger.info("Cluster verification completed successfully")
            return 0
            
        # Run setup
        result = setup.setup()
        logger.info(f"Setup completed successfully: {result}")
        return 0
        
    except Exception as e:
        logger.exception(f"Error during setup: {e}")
        
        if not args.no_cleanup:
            try:
                logger.info("Attempting cleanup after failure...")
                setup.cleanup()
            except Exception as cleanup_error:
                logger.error(f"Cleanup error: {cleanup_error}")
        
        return 1

if __name__ == "__main__":
    sys.exit(main())
