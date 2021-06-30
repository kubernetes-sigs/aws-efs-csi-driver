import subprocess
import shlex
import os
import shutil
import tarfile
import argparse
import sys

parser = argparse.ArgumentParser(description="Troubleshooting EFS CSI Driver")

parser.add_argument("--driver-pod-name", required=True, help="The EFS CSI driver pod name")

args = parser.parse_args(sys.argv[1:])

driver_pod_name = args.driver_pod_name

results_dir_path = 'results'

# Clean up existing results folder
shutil.rmtree(results_dir_path, ignore_errors=True)

os.makedirs(results_dir_path)

def execute(command, file, shell=False):
    print(command + "\n", file=file, flush=True)
    if shell:
        subprocess.run(command, shell=True, text=True, stderr=subprocess.STDOUT, stdout=f)
    else:
        subprocess.run(shlex.split(command), text=True, stderr=subprocess.STDOUT, stdout=f)
    print("\n", file=file, flush=True)


with open(results_dir_path + '/driver_info', 'w') as f:
    describe_driver_pod = f'kubectl describe po {driver_pod_name} -n kube-system'
    execute(command=describe_driver_pod, file=f)

    get_driver_pod = f'kubectl get po {driver_pod_name} -n kube-system -o yaml'
    execute(command=get_driver_pod, file=f)

with open(results_dir_path + '/driver_logs', 'w') as f:
    mounts = f'kubectl logs {driver_pod_name} -n kube-system efs-plugin'
    execute(command=mounts, file=f)


def collect_driver_files_under_dir(dir_name, file):
    collect_driver_files_under_dir = f'kubectl exec {driver_pod_name} -n kube-system -c efs-plugin -- find {dir_name} ' + \
                                     r'-type f -exec echo {} \; -exec cat {} \; -exec echo \;'
    execute(command=collect_driver_files_under_dir, file=file)


with open(results_dir_path + '/efs_utils_logs', 'w') as f:
    collect_driver_files_under_dir(dir_name='/var/log/amazon/efs', file=f)

with open(results_dir_path + '/efs_utils_state_dir', 'w') as f:
    collect_driver_files_under_dir(dir_name='/var/run/efs', file=f)

with open(results_dir_path + '/mounts', 'w') as f:
    mounts = f'kubectl exec {driver_pod_name} -n kube-system -c efs-plugin -- mount |grep nfs '
    execute(command=mounts, file=f, shell=True)

with tarfile.open("results.tgz", "w:gz") as tar:
    tar.add(results_dir_path, arcname=os.path.basename(results_dir_path))
