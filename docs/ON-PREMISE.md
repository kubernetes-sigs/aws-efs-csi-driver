## Notes for on-premise Kubernetes environment

### Decouple EC2 metadata service (IMDS)
Since on-premise Kubernetes environment cannot access Amazon EC2 metadata service and cannot get information about instanceID, region and availabilityZone, additional environment variables need to be set, otherwise it will throw "could not get metadata from AWS: EC2 instance metadata is not available" described in [issue 468](https://github.com/kubernetes-sigs/aws-efs-csi-driver/issues/468).

Environment variables need to be added for efs-plugin container in [controller-deployment.yaml](../deploy/kubernetes/base/controller-deployment.yaml), specify onPremise to let driver know it's a on-premise Kubernetes environment, and then follow the deployment guide in [README.md](./README.md). Examples are shown below (instanceID can be mocked):

```
...
        - name: onPremise
          value: "true"
        - name: instanceID
          value: i-0123456789012345
        - name: region
          value: us-east-1
        - name: availabilityZone
          value: us-east-1a
...
```
For IAM permission, you could set it using environment variables with [AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html) or mount secret to container for [configuration and credential file settings](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html).

### Configure region for efs-utils
Besides, you might encounter errors when mounting file system "Output: Error retrieving region. Please set the "region" parameter in the efs-utils configuration file", the binary entrypoint aws-efs-csi-driver would dynamically generate configuration file, but the region information need to specify in the conf file which will be override by aws-efs-csi-driver. Follow below procedure to fix this issue:
* Get the original efs-utils configuration file:
```
kubectl -n kube-system exec -it efs-csi-node-<id> -c efs-plugin cat /etc/amazon/efs/efs-utils.conf
```
* Configure region information `region = us-east-1` and add disable fetch ec2 metadata setting:
```
disable_fetch_ec2_metadata_token = true
```
* Create new configmap:
```
kubectl -n kube-system create configmap efs-utils-conf --from-file=./efs-utils.conf
```
* Edit efs-plugin in daemon set efs-csi-node:
```
kubectl -n kube-system edit daemonsets.apps efs-csi-node
```
Add the configurations below:
```
...
        lifecycle:
          postStart:
            exec:
              command: ["/bin/sh", "-c", "cp -f /tmp/efs-utils.conf /etc/amazon/efs/efs-utils.conf"]
...
        - mountPath: /tmp/efs-utils.conf 
          subPath: efs-utils.conf
          name: efs-utils-conf
...
        - configMap:
          name: efs-utils-conf
        name: efs-utils-conf
...
```

### DNS resolve issue
And if you still got errors 'Output: Failed to resolve "fs-01234567.efs.us-east-1.amazonaws.com" - check that your file system ID is correct, and ensure that the VPC has an EFS mount target for this file system ID." Follow below procedure to fix this issue:
* Configure IP address in /etc/hosts on each host, refer to [Walkthrough: Create and mount a file system on-premises with AWS Direct Connect and VPN](https://docs.aws.amazon.com/efs/latest/ug/efs-onpremises.html)
* Or install botocore on each host and set `fall_back_to_mount_target_ip_address_enabled = true` in efs-utils.conf, refer to [Using botocore to retrieve mount target ip address when dns name cannot be resolved](https://github.com/aws/efs-utils).
