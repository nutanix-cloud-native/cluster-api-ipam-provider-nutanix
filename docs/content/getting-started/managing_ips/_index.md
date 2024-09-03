+++
title = "Managing IPs"
icon = "fa-solid fa-seedling"
weight = 2
+++

In order to manage IPs via CAIPAMX, a `NutanixIPPool` has to be created via the Kubernetes API. The `NutanixIPPool`
specifies which Nutanix cluster and subnet to manage IPs in, and which credentials to use to perform IP operations.

## Create Nutanix credentials secret

```shell
$ export NUTANIX_USER=...
$ export NUTANIX_PASSWORD=...

$ cat <<EOF | kubectl apply --server-side -f -
apiVersion: v1
kind: Secret
metadata:
  name: pc-creds-for-ipam
stringData:
  credentials: |
    [
      {
        "type": "basic_auth",
        "data": {
          "prismCentral":{
            "username": "${NUTANIX_USER}",
            "password": "${NUTANIX_PASSWORD}"
          }
        }
      }
    ]
EOF
```

## Create the IP pool

```shell
$ export NUTANIX_ENDPOINT=https://<host>:9440
$ export NUTANIX_SUBNET=...

$ cat <<EOF | kubectl apply --server-side -f -
apiVersion: ipam.cluster.x-k8s.io/v1alpha1
kind: NutanixIPPool
metadata:
  name: nutanixippool-sample
spec:
  prismCentral:
    address: ${NUTANIX_ENDPOINT}
    port: 9440
    credentialSecretRef:
      name: pc-creds-for-ipam
  subnet: ${NUTANIX_SUBNET}
EOF
```

## Create the IP address claim

```shell
$ cat <<EOF | kubectl apply --server-side -f -
apiVersion: ipam.cluster.x-k8s.io/v1beta1
kind: IPAddressClaim
metadata:
  name: my-ip
spec:
  poolRef:
    apiGroup: ipam.cluster.x-k8s.io
    kind: NutanixIPPool
    name: nutanixippool-sample
EOF
```

## Check the IP address has been reserved

As this is asynchronous you may have to wait for a short period until the IP address is reserved
and the Kubernetes `IPAddress` exists.

```shell
$ kubectl get ipaddress my-ip
NAME    ADDRESS        POOL NAME              POOL KIND       AGE
my-ip   10.40.142.50   nutanixippool-sample   NutanixIPPool   3s
```

## Delete the IP address claim

```shell
$ kubectl delete ipaddressclaim my-ip
ipaddressclaim.ipam.cluster.x-k8s.io "my-ip" deleted
```

Again, this operation is asynchronous so the deletion of the associated `IPAddress` will take a short time, but once
the IP has been unreserved, both the IP address claim and the IP address resouces will have been deleted:

```shell
$ kubectl get ipaddressclaim,ipaddress my-ip
Error from server (NotFound): ipaddressclaims.ipam.cluster.x-k8s.io "my-ip" not found
Error from server (NotFound): ipaddresses.ipam.cluster.x-k8s.io "my-ip" not found
```
