#!/usr/bin/env python3
import os
import sys
import yaml
import logging
import argparse
from datetime import datetime
from kubernetes import client, config
# Import test frameworks
from tests.orchestrator import EFSCSIOrchestrator
from utils.report_generator import ReportGenerator
from utils.metrics_collector import MetricsCollector
from utils.log_integration import collect_logs_on_test_failure
# Commented out to remove dependency on cluster setup
# from cluster_setup import ClusterSetup

def setup_logging(config):
    """Setup logging based on configuration
    
    Args:
        config: Configuration dictionary
    """
    log_config = config.get('logging', {})
    log_level = getattr(logging, log_config.get('level', 'INFO'))
    log_file = log_config.get('file', 'logs/efs_tests.log')
    
    # Create logs directory if it doesn't exist
    os.makedirs(os.path.dirname(log_file), exist_ok=True)
    
    logging.basicConfig(
        level=log_level,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
        handlers=[
            logging.FileHandler(log_file),
            logging.StreamHandler()
        ]
    )
    
    return logging.getLogger(__name__)

def parse_args():
    """Parse command line arguments
    
    Returns:
        Parsed arguments
    """
    parser = argparse.ArgumentParser(description='Run EFS CSI tests')
    parser.add_argument(
        '--config', 
        default='config/orchestrator_config.yaml',
        help='Path to configuration file'
    )
    parser.add_argument(
        '--test-suite', 
        choices=['orchestrator', 'chaos', 'all'],
        default='orchestrator',
        help='Test suite to run'
    )
    parser.add_argument(
        '--duration',
        type=int,
        help='Duration in seconds for test execution'
    )
    parser.add_argument(
        '--interval',
        type=int,
        default=None,
        help='Seconds to wait between operations (overrides config value)'
    )
    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Print what would be done without executing tests'
    )
    parser.add_argument(
        '--driver-pod-name',
        default=None,
        help='Name of the EFS CSI driver pod for log collection (optional)'
    )
    
    # Cluster setup options - kept for compatibility but functionality is disabled
    parser.add_argument(
        '--setup-cluster',
        action='store_true',
        help='Set up EKS cluster and EFS CSI driver before running tests (DISABLED)'
    )
    parser.add_argument(
        '--kubernetes-version',
        help='Kubernetes version to use for cluster (DISABLED)'
    )
    parser.add_argument(
        '--driver-version',
        help='EFS CSI Driver version to install (DISABLED)'
    )
    parser.add_argument(
        '--skip-cleanup',
        action='store_true',
        help='Skip cleanup of cluster resources after tests (DISABLED)'
    )
    
    return parser.parse_args()

def check_credentials():
    """Check if credentials are valid by making a simple API call"""
    try:
        # Attempt to get a list of namespaces - a simple, harmless API call
        api = client.CoreV1Api()
        api.list_namespace(_request_timeout=10)
        return True
    except Exception as e:
        error_str = str(e)
        if "401" in error_str or "Unauthorized" in error_str:
            return False
        # For other types of errors, we assume credentials are valid but other issues exist
        return True

def print_credential_renewal_instructions():
    """Print instructions for renewing AWS credentials"""
    print("\n" + "="*80)
    print(f"{'AWS CREDENTIALS EXPIRED OR INVALID':^80}")
    print("="*80)
    print("\nYour AWS credentials have expired or are invalid.")
    print("\nPlease check your AWS credentials and ensure they are properly configured.")
    print("\nAfter renewing your credentials, try running the tests again.")
    print("="*80 + "\n")

def main():
    """Main entry point"""
    args = parse_args()
    
    # Load configuration
    try:
        with open(args.config, 'r') as f:
            config = yaml.safe_load(f)
    except Exception as e:
        print(f"Error loading configuration: {e}")
        sys.exit(1)
    
    # Setup logging
    logger = setup_logging(config)
    logger.info(f"Starting EFS CSI tests with configuration from {args.config}")
    
    # Verify credentials before proceeding
    logger.info("Verifying AWS credentials")
    if not check_credentials():
        logger.error("AWS credentials are expired or invalid")
        print_credential_renewal_instructions()
        sys.exit(1)
    
    # Set up cluster if requested - DISABLED
    if args.setup_cluster:
        logger.warning("Cluster setup functionality is disabled - skipping setup")
        logger.warning("Please make sure you have a working cluster with EFS CSI driver installed")
        
        # The following code has been commented out
        """
        # Update config with command-line overrides
        if args.kubernetes_version:
            config['cluster']['kubernetes_version'] = args.kubernetes_version
            logger.info(f"Using Kubernetes version from command line: {args.kubernetes_version}")
            
        if args.driver_version:
            config['driver']['version'] = args.driver_version
            logger.info(f"Using EFS CSI Driver version from command line: {args.driver_version}")
        
        # Write updated config back to file
        with open(args.config, 'w') as f:
            yaml.dump(config, f)
            
        # Initialize and run cluster setup
        try:
            cluster_setup = ClusterSetup(config_file=args.config)
            setup_result = cluster_setup.setup()
            logger.info(f"Cluster setup completed: {setup_result}")
        except Exception as e:
            logger.error(f"Cluster setup failed: {e}")
            sys.exit(1)
        """
    
    # Initialize report generator
    report_dir = config.get('reporting', {}).get('output_dir', 'reports')
    report_generator = ReportGenerator(output_dir=report_dir)
    
    # Initialize metrics collector
    metrics_collector = MetricsCollector()
    
    # Run tests based on test suite
    results = {}
    
    try:
        if args.test_suite in ['orchestrator', 'all']:
            logger.info("Running orchestrator stress test suite")
            
            if args.dry_run:
                logger.info("DRY RUN MODE: Would run orchestrator with randomized operations")
                orchestrator_results = {
                    "status": "would_run",
                    "description": "Would run the orchestrator with randomized operations"
                }
            else:
                logger.info(f"Starting orchestrator for {args.duration} seconds")
                
                # Create orchestrator instance - use dedicated orchestrator config file if it exists,
                # otherwise fall back to the main config
                orchestrator_config = 'config/orchestrator_config.yaml'
                if not os.path.exists(orchestrator_config):
                    logger.warning(f"Orchestrator config file {orchestrator_config} not found, falling back to {args.config}")
                    orchestrator_config = args.config
                
                logger.info(f"Using orchestrator config: {orchestrator_config}")
                # Get driver pod name from config if not specified on command line
                driver_pod_name = args.driver_pod_name
                if driver_pod_name is None and 'driver' in config and 'pod_name' in config['driver']:
                    driver_pod_name = config['driver']['pod_name']
                    if driver_pod_name:
                        logger.info(f"Using driver pod name from config: {driver_pod_name}")
                
                # Pass the metrics collector and driver pod name to the orchestrator
                orchestrator = EFSCSIOrchestrator(
                    config_file=orchestrator_config, 
                    metrics_collector=metrics_collector,
                    driver_pod_name=driver_pod_name
                )
                
                # Override default test parameters if specified
                if args.duration:
                    orchestrator.test_duration = args.duration
                    logger.info(f"Test duration overridden to {args.duration} seconds")
                    
                if args.interval is not None:  # Only override if explicitly specified
                    orchestrator.operation_interval = args.interval
                    logger.info(f"Operation interval overridden to {orchestrator.operation_interval} seconds")
                else:
                    logger.info(f"Using operation interval from config: {orchestrator.operation_interval} seconds")
                
                # Run the orchestrator test
                orchestrator_results = orchestrator.run_test()
                
                # Generate one consolidated orchestrator report with all information
                orchestrator_report_dir = os.path.join(report_dir, 'orchestrator')
                os.makedirs(orchestrator_report_dir, exist_ok=True)

                timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
                test_name = f"efs_orchestrator_{timestamp}"
                
                # Create report path
                report_path = os.path.join(
                    orchestrator_report_dir, 
                    f"{test_name}.json"
                )
                
                # Add metadata, system info, and metrics to the results
                system_info = report_generator._collect_system_info()
                
                # Get all metrics from the metrics collector
                collected_metrics = metrics_collector.get_all_metrics()
                
                full_report = {
                    "test_name": test_name,
                    "test_type": "orchestrator",
                    "timestamp": timestamp,
                    "system_info": system_info,
                    "results": orchestrator_results,
                    "metrics": {
                        "file_performance": collected_metrics.get("file_performance", {})
                    }
                }
                
                logger.info("File performance metrics collected:")
                for volume, metrics in collected_metrics.get("file_performance", {}).get("by_volume", {}).items():
                    logger.info(f"  Volume: {volume}")
                    # Log read metrics if available
                    if metrics["iops"].get("read") is not None:
                        logger.info(f"    Read IOPS: {metrics['iops']['read']:.2f}")
                    if metrics["throughput"].get("read") is not None:
                        logger.info(f"    Read Throughput: {metrics['throughput']['read']:.2f} MB/s")
                        
                    # Log write metrics if available
                    if metrics["iops"].get("write") is not None:
                        logger.info(f"    Write IOPS: {metrics['iops']['write']:.2f}")
                    if metrics["throughput"].get("write") is not None:
                        logger.info(f"    Write Throughput: {metrics['throughput']['write']:.2f} MB/s")
                
                with open(report_path, 'w') as f:
                    import json
                    json.dump(full_report, f, indent=2)
                
                logger.info(f"Orchestrator report generated: {report_path}")
            
            results['orchestrator'] = orchestrator_results
        
    except Exception as e:
        logger.error(f"Error running tests: {e}", exc_info=True)
        
        # Collect logs on failure
        logger.info("Collecting logs due to test failure")
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        test_name = f"efs_orchestrator_failure_{timestamp}"
        
        # Get driver pod name from config if not specified on command line
        driver_pod_name = args.driver_pod_name
        if driver_pod_name is None and 'driver' in config and 'pod_name' in config['driver']:
            driver_pod_name = config['driver']['pod_name']
            if driver_pod_name:
                logger.info(f"Using driver pod name from config: {driver_pod_name}")
                
        logs_path = collect_logs_on_test_failure(test_name, metrics_collector, driver_pod_name=driver_pod_name)
        if logs_path:
            logger.info(f"Failure logs collected to: {logs_path}")
        else:
            logger.warning("Failed to collect logs")
        
        # Even if tests failed, try to clean up if requested - DISABLED
        if args.setup_cluster and not args.skip_cleanup:
            logger.warning("Cluster cleanup functionality is disabled")
            """
            try:
                logger.info("Attempting cleanup after test failure")
                cluster_setup = ClusterSetup(config_file=args.config)
                cluster_setup.cleanup()
                logger.info("Cleanup completed")
            except Exception as cleanup_error:
                logger.error(f"Cleanup after failure error: {cleanup_error}")
            """
        
        sys.exit(1)

if __name__ == "__main__":
    main()
