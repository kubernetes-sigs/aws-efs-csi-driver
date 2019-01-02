[![Build Status](https://travis-ci.org/aws/aws-efs-csi-driver.svg?branch=master)](https://travis-ci.org/aws/aws-efs-csi-driver)

**WARNING**: This driver is in ALPHA currently. This means that there may potentially be backwards compatible breaking changes moving forward. Do NOT use this driver in a production environment in its current state.

**DISCLAIMER**: This is not an officially supported Amazon product

## AWS EFS CSI Driver
###

The [Amazon Elastic File System](https://aws.amazon.com/efs/) Container Storage Interface (CSI) Driver implements [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) interface for container orchestrators to manage lifecycle of Amazon EFS volumes.

This driver is in alpha stage. Basic volume operations that are functional include NodePublishVolume/NodeUnpublishVolume.

### CSI Specification Compability Matrix
| AWS EFS CSI Driver \ CSI Version       | v0.3.0| v1.0.0 |
|----------------------------------------|-------|--------|
| master branch                          | yes   | no     |

### Kubernetes Version Compability Matrix
| AWS EFS CSI Driver \ Kubernetes Version| v1.12 | v1.13 |
|----------------------------------------|-------|-------|
| master branch                          | yes   | yes   |

## Features
Currently only static provisioning is supported. This means a AWS EFS filesystem needs to be created manually on AWS first. After that it could be mounted inside container as a volume using AWS EFS CSI Driver.

# Kubernetes Example
This example demos how to make a EFS filesystem mounted inside container using the driver. Before this, get yourself familiar with setting up kubernetes on AWS and [creating EFS filesystem](https://docs.aws.amazon.com/efs/latest/ug/getting-started.html). And when creating EFS filesystem, make sure it is accessible from kuberenetes cluster. This can be achieved by creating EFS filesystem inside the same VPC as kubernetes cluster or using VPC peering.

Once kubernetes cluster and EFS filesystem is created, modify secret manifest file [secret.yaml](../deploy/kubernetes/secret.yaml). 

Then create the secret object:
```
kubectl apply -f deploy/kubernetes/secret.yaml 
```

Deploy AWS EFS CSI driver:

```
kubectl apply -f https://raw.githubusercontent.com/aws/aws-efs-csi-driver/master/deploy/kubernetes/attacher.yaml 
kubectl apply -f https://raw.githubusercontent.com/aws/aws-efs-csi-driver/master/deploy/kubernetes/node.yaml
```

Edit the [persistence volume manifest file](../deploy/kubernetes/sample_app/pv.yaml):
```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: efs-pv
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Recycle
  storageClassName: efs-sc
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId] 
```
Replace `VolumeHandle` with `FileSystemId` of the EFS filesystem that needs to be mounted. You can find it using AWS CLI:

```
aws efs describe-file-systems 
```

Then create PV, persistence volume claim (PVC) and storage class:
```
kubectl apply -f deploy/kubernetes/sample_app/storageclass.yaml
kubectl apply -f deploy/kubernetes/sample_app/pv.yaml
kubectl apply -f deploy/kubernetes/sample_app/claim.yaml
kubectl apply -f deploy/kubernetes/sample_app/pod.yaml
```

After the objects are created, verify that pod name app is running:

```
kubectl get pods
```

Also you can verify that data is written onto EFS filesystem:

```
kubectl exec -ti app -- tail -f /data/out.txt
```

## Development
Please go through [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and [General CSI driver development guideline](https://kubernetes-csi.github.io/docs/Development.html) to get some basic understanding of CSI driver before you start.

### Requirements
* Golang 1.11.2+

### Testing
To execute all unit tests, run: `make test`

## License
This library is licensed under the Apache 2.0 License. 
