# Troubleshooting

#### Note
By default, logs are published to the node in which the EFS mount occurs. 
This node can be found by examining the output of `kubectl describe pod efs-app`, where `efs-app` is the pod which utilizes the EFS PVC. For the most accurate logs, please substitute the pod name corresponding to the aforementioned node in the below steps for `<driver_pod_name>`.

### Log collector script

The log collector script will collect  
- output of `kubectl describe pod <driver_pod_name> -n kube-system`
- output of `kubectl get pod <driver_pod_name> -o yaml -n kube-system`
- efs-utils logs from `/var/log/amazon/efs`
- efs-utils state files from `/var/run/efs`
- nfs mounts on filesystem

You can run the log collector as follows:  
```
python3 log_collector.py --driver-pod-name <driver_pod_name>
# for example
python3 log_collector.py --driver-pod-name efs-csi-node-7g8k2
```

However, before running the log collector, you may want to enable debug logs for both the csi driver and [efs-utils](https://github.com/aws/efs-utils), 
which is the mounting utility used by the driver.

After enabling debug logging using the following sections and before collecting logs,
make sure to attempt the mount again, so that new failure logs are acquired.

### Enable debug logs option (Increase verbosity level of CSI driver to the highest level and enable debug in efs-utils)
Modify the deployment manifests to enable debug logs using one of the following methods
depending on your installation method:

**Kustomize/kubectl installation Example:**  
```sh
sed -i 's/debug-logs=false/debug-logs=true/g' deploy/kubernetes/base/node-daemonset.yaml deploy/kubernetes/base/controller-deployment.yaml
```
You will need to redeploy the driver after running this command.

**Helm installation Example:**
```sh
helm upgrade --install aws-efs-csi-driver --namespace kube-system --set debugLogs=true
```

### Advanced debugging (optional)
Capture traffic from the pod where failure is occurring:  
`tcpdump -W 30 -C 1000 -s 2000 -w nfs_pcap_$(date +%FT%T).pcap -i any -z gzip -Z root 'port 2049 or (src 127.0.0.1 and dst 127.0.0.1)'`

Capture operating system log data from the pod where failure is occurring:  
`cat /var/log/message > kernel_debug.log`

### Posting to this repository's issues
We encourage users to post errors that you may run into to this repository's issues.
Please include the appropriate logs from the above sources.
