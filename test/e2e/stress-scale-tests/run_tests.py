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
        default=300,
        help='Duration in seconds for test execution'
    )
    parser.add_argument(
        '--rate',
        type=int,
        default=5,
        help='Operations per second for tests'
    )
    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Print what would be done without executing tests'
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
    print("\nTo renew your credentials, run:")
    print("\n    ada credentials update --provider isengard --role=Admin --once --account YOUR_ACCOUNT_ID")
    print("\nOr for temporary credentials:")
    print("\n    aws sts get-session-token")
    print("\nAfter renewing credentials, try running the tests again.")
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
                orchestrator = EFSCSIOrchestrator(config_file=orchestrator_config)
                
                # Override default test parameters if specified
                if args.duration:
                    orchestrator.test_duration = args.duration
                    logger.info(f"Test duration overridden to {args.duration} seconds")
                    
                if args.rate:  # Use rate as operation interval (inverse relationship)
                    orchestrator.operation_interval = max(1, int(1 / args.rate))
                    logger.info(f"Operation interval overridden to {orchestrator.operation_interval} seconds (from rate {args.rate}/s)")
                
                # Run the orchestrator test
                orchestrator_results = orchestrator.run_test()
                
                # Generate orchestrator test specific report
                orchestrator_report_dir = os.path.join(report_dir, 'orchestrator')
                os.makedirs(orchestrator_report_dir, exist_ok=True)
                
                timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
                orchestrator_report_path = os.path.join(
                    orchestrator_report_dir, 
                    f"efs_orchestrator_{timestamp}.json"
                )
                
                with open(orchestrator_report_path, 'w') as f:
                    import json
                    json.dump(orchestrator_results, f, indent=2)
                    
                logger.info(f"Orchestrator report generated: {orchestrator_report_path}")
            
            results['orchestrator'] = orchestrator_results
        
    except Exception as e:
        logger.error(f"Error running tests: {e}", exc_info=True)
        
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
