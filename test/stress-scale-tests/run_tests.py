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
    # Driver pod name functionality commented out as not currently used
    # parser.add_argument(
    #     '--driver-pod-name',
    #     default=None,
    #     help='Name of the EFS CSI driver pod for log collection (optional)'
    # )
    
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

def load_config(config_path):
    """Load configuration from YAML file
    
    Args:
        config_path: Path to configuration file
        
    Returns:
        Loaded configuration as dictionary
    """
    try:
        with open(config_path, 'r') as f:
            config = yaml.safe_load(f)
        return config
    except Exception as e:
        print(f"Error loading configuration: {e}")
        sys.exit(1)

def get_driver_pod_name(args, config):
    """Get driver pod name from config
    
    Args:
        args: Parsed command line arguments
        config: Configuration dictionary
        
    Returns:
        Driver pod name or None
    """
    # Since driver-pod-name arg is commented out, we only check config
    driver_pod_name = None
    if 'driver' in config and 'pod_name' in config['driver']:
        driver_pod_name = config['driver']['pod_name']
    return driver_pod_name

def handle_setup_cluster(args, config, logger):
    """Handle cluster setup functionality
    
    Args:
        args: Command line arguments
        config: Configuration dictionary
        logger: Logger instance
    """
    if args.setup_cluster:
        logger.warning("Cluster setup functionality is disabled - skipping setup")
        logger.warning("Please make sure you have a working cluster with EFS CSI driver installed")

def initialize_components(config):
    """Initialize report generator and metrics collector
    
    Args:
        config: Configuration dictionary
        
    Returns:
        report_generator, metrics_collector, report_dir
    """
    report_dir = config.get('reporting', {}).get('output_dir', 'reports')
    report_generator = ReportGenerator(output_dir=report_dir)
    metrics_collector = MetricsCollector()
    return report_generator, metrics_collector, report_dir

def run_orchestrator_test(args, config, logger, metrics_collector, report_generator, report_dir):
    """Run the orchestrator test suite
    
    Args:
        args: Command line arguments
        config: Configuration dictionary
        logger: Logger instance
        metrics_collector: Instance of MetricsCollector
        report_generator: Instance of ReportGenerator
        report_dir: Path to report directory
    
    Returns:
        Orchestrator test results
    """
    results = {}
    
    if args.test_suite not in ['orchestrator', 'all']:
        return results

    logger.info("Running orchestrator stress test suite")
    
    if args.dry_run:
        logger.info("DRY RUN MODE: Would run orchestrator with randomized operations")
        return {
            'orchestrator': {
                "status": "would_run",
                "description": "Would run the orchestrator with randomized operations"
            }
        }
    
    # Set up the orchestrator
    orchestrator = setup_orchestrator(args, config, logger, metrics_collector)
    
    # Run the test
    logger.info(f"Starting orchestrator for {args.duration if args.duration else 'default'} seconds")
    test_results = orchestrator.run_test()
    
    # Generate and save the test report
    generate_test_report(
        test_results, 
        report_dir, 
        report_generator, 
        metrics_collector, 
        logger
    )
    
    return {'orchestrator': test_results}

def setup_orchestrator(args, config, logger, metrics_collector):
    """Set up the orchestrator for testing
    
    Args:
        args: Command line arguments
        config: Configuration dictionary
        logger: Logger instance
        metrics_collector: Instance of MetricsCollector
        
    Returns:
        Configured orchestrator instance
    """
    # Get orchestrator config file
    orchestrator_config = 'config/orchestrator_config.yaml'
    if not os.path.exists(orchestrator_config):
        logger.warning(f"Orchestrator config file {orchestrator_config} not found, falling back to {args.config}")
        orchestrator_config = args.config
    
    logger.info(f"Using orchestrator config: {orchestrator_config}")
    
    # Driver pod name functionality commented out as not currently needed
    # driver_pod_name = get_driver_pod_name(args, config)
    # if driver_pod_name:
    #     logger.info(f"Using driver pod name from config: {driver_pod_name}")
    
    # Create orchestrator (driver_pod_name parameter commented out)
    orchestrator = EFSCSIOrchestrator(
        config_file=orchestrator_config, 
        metrics_collector=metrics_collector,
        # driver_pod_name=driver_pod_name
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
        
    return orchestrator

def generate_test_report(test_results, report_dir, report_generator, metrics_collector, logger):
    """Generate a test report with metrics and results
    
    Args:
        test_results: Results from the test run
        report_dir: Directory to store reports
        report_generator: Report generator instance
        metrics_collector: Metrics collector instance
        logger: Logger instance
    """
    # Make report directory
    orchestrator_report_dir = os.path.join(report_dir, 'orchestrator')
    os.makedirs(orchestrator_report_dir, exist_ok=True)

    # Create timestamp and test name
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    test_name = f"efs_orchestrator_{timestamp}"
    
    # Create report path
    report_path = os.path.join(orchestrator_report_dir, f"{test_name}.json")
    
    # Add metadata, system info, and metrics to the results
    system_info = report_generator._collect_system_info()
    collected_metrics = metrics_collector.get_all_metrics()
    
    # Create full report
    full_report = {
        "test_name": test_name,
        "test_type": "orchestrator",
        "timestamp": timestamp,
        "system_info": system_info,
        "results": test_results,
        "metrics": {
            "file_performance": collected_metrics.get("file_performance", {})
        }
    }
    
    # Log metrics
    log_performance_metrics(collected_metrics, logger)
    
    # Write report to file
    with open(report_path, 'w') as f:
        import json
        json.dump(full_report, f, indent=2)
    
    logger.info(f"Orchestrator report generated: {report_path}")

def log_performance_metrics(collected_metrics, logger):
    """Log performance metrics
    
    Args:
        collected_metrics: Metrics data
        logger: Logger instance
    """
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

def handle_test_failure(e, args, config, metrics_collector, logger):
    """Handle test failure
    
    Args:
        e: Exception
        args: Command line arguments
        config: Configuration dictionary
        metrics_collector: Metrics collector instance
        logger: Logger instance
    """
    logger.error(f"Error running tests: {e}", exc_info=True)
    
    # Collect logs on failure
    logger.info("Collecting logs due to test failure")
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    test_name = f"efs_orchestrator_failure_{timestamp}"
    
    # Driver pod name functionality commented out
    # driver_pod_name = get_driver_pod_name(args, config)
    # if driver_pod_name:
    #     logger.info(f"Using driver pod name from config: {driver_pod_name}")
            
    logs_path = collect_logs_on_test_failure(test_name, metrics_collector)  # driver_pod_name parameter removed
    if logs_path:
        logger.info(f"Failure logs collected to: {logs_path}")
    else:
        logger.warning("Failed to collect logs")
    
    # Even if tests failed, try to clean up if requested - DISABLED
    if args.setup_cluster and not args.skip_cleanup:
        logger.warning("Cluster cleanup functionality is disabled")

def main():
    """Main entry point"""
    # Parse command line arguments
    args = parse_args()
    
    # Load configuration
    config = load_config(args.config)
    
    # Setup logging
    logger = setup_logging(config)
    logger.info(f"Starting EFS CSI tests with configuration from {args.config}")
    
    # Verify credentials before proceeding
    logger.info("Verifying AWS credentials")
    if not check_credentials():
        logger.error("AWS credentials are expired or invalid")
        print_credential_renewal_instructions()
        sys.exit(1)
    
    # Handle cluster setup if requested
    handle_setup_cluster(args, config, logger)
    
    # Initialize components
    report_generator, metrics_collector, report_dir = initialize_components(config)
    
    # Run tests
    try:
        results = run_orchestrator_test(
            args, config, logger, metrics_collector, report_generator, report_dir
        )
        return results
    except Exception as e:
        handle_test_failure(e, args, config, metrics_collector, logger)
        sys.exit(1)

if __name__ == "__main__":
    main()
