## Use in Stateful Set
This example shows how to consume EFS filesystem from StatefulSets using the driver. Before the example, refer to [StatefulSets](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) for what it is.

## Deploy the example

```sh
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-efs-csi-driver/master/examples/kubernetes/statefulset/specs/example.yaml
```

## Check the StatefulSets Application
Check StatefulSets is deployed successfully:
```sh
$ kubectl get sts
NAME          READY   AGE
efs-app-sts   3/3     70m
``` 

Check the pods are running:
```sh
$ kubectl get po
NAME            READY   STATUS    RESTARTS   AGE
efs-app-sts-0   1/1     Running   0          71m
efs-app-sts-1   1/1     Running   0          71m
efs-app-sts-2   1/1     Running   0          71m
```

Check data are written onto EFS filesystem:
```sh
$ kubectl exec -ti efs-app-sts-0 -- tail -f /efs-data/out.txt
Mon May 6 00:50:15 UTC 2019
Mon May 6 00:50:18 UTC 2019
Mon May 6 00:50:19 UTC 2019
Mon May 6 00:50:20 UTC 2019
Mon May 6 00:50:23 UTC 2019
Mon May 6 00:50:24 UTC 2019
Mon May 6 00:50:25 UTC 2019
Mon May 6 00:50:28 UTC 2019
Mon May 6 00:50:29 UTC 2019
Mon May 6 00:50:30 UTC 2019
```

