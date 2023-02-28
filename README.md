# Get started

## Difference with [multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane) in open-cluster-management

This reposotiy is an extension to [open-cluster-management-io/multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane) with the following enhancements:
1. Added managed cluster info addon
2. Added policy addon
3. Added managed service account addon

In line with [open-cluster-management-io/multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane):
1. Using the same API Server, it starts core kubernetes and enables open-cluster-management registration controllers
2. Both embedded etcd and external etcd are supported
3. Test scenarios in [open-cluster-management-io/multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane) are all covered in the current repository

## Install multicluster-controlplane

### Option 1: Start multicluster-controlplane with embedded etcd on Openshift Cluster
#### Build image

```bash
$ export IMAGE_NAME=<customized image. default is quay.io/stolostron/multicluster-controlplane:latest>
$ make build-image push-image
```

#### Install 
Set environment variables firstly and then deploy controlplane.
* `HUB_NAME` (optional) is the namespace where the controlplane is deployed in. The default is `multicluster-controlplane`.
```bash
$ export HUB_NAME=<hub name>
$ make deploy
```

### Option 2: Start multicluster-controlplane with external etcd on Openshift Cluster 

#### Install etcd
Set environmrnt variables and deploy etcd.
* `ETCD_NS` (optional) is the namespace where the etcd is deployed in. The default is `multicluster-controlplane-etcd`.

For example:
```bash
$ export ETCD_NS=<etcd namespace>
$ make deploy-etcd
```

#### Build image
```bash
$ export IMAGE_NAME=<customized image. default is quay.io/stolostron/multicluster-controlplane:latest>
$ make build-image push-image
```

#### Install controlplane
Set environment variables and deploy controlplane.
* `HUB_NAME` (optional) is the namespace where the controlplane is deployed in. The default is `multicluster-controlplane`.

For example: 
```bash
$ export HUB_NAME=<hub name>
$ make deploy-with-external-etcd
```

### Option 3: Start multicluster-controlplane as a local binary

```bash
$ make all
```

### Option 4: Start multicluster-controlplane with embedded etcd on KinD cluster

```bash
$ make setup-e2e
```

## Access the controlplane and join cluster

The kubeconfig file of the controlplane is in the dir `hack/deploy/cert-${HUB_NAME}/kubeconfig`.

You can use clusteradm to access and join a cluster.
```bash
$ clusteradm --kubeconfig=<kubeconfig file> get token --use-bootstrap-token
$ clusteradm join --hub-token <hub token> --hub-apiserver <hub apiserver> --cluster-name <cluster_name>
$ clusteradm --kubeconfig=<kubeconfig file> accept --clusters <cluster_name>
```

> **Warning**
> clusteradm version should be v0.4.1 or later


## Undeploy the controlplane
```bash
$ make destroy
```